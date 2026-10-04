package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/axiomhq/axiom-go/axiom"
	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	"github.com/klauspost/compress/zstd"
)

func receiver(t *testing.T, handler http.HandlerFunc) *axiom.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := axiom.NewClient(axiom.SetToken("xaat-test"), axiom.SetNoRetry(), axiom.SetURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func stream(t *testing.T, d *Driver) (*os.File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	// Docker opens its writer before StartLogging and retains it until StopLogging returns.
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.StartLogging(path, logger.Info{ContainerID: strings.Repeat("a", 64), ContainerName: "example", Config: map[string]string{"axiom-dataset": "override"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.StopLogging(path); f.Close() })
	return f, path
}

func TestDelivery(t *testing.T) {
	for _, mode := range []string{"idle", "stop", "batch"} {
		t.Run(mode, func(t *testing.T) {
			got := make(chan []axiom.Event, 10)
			c := receiver(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/datasets/override/ingest" {
					t.Errorf("wrong dataset: %s", r.URL.Path)
				}
				zr, err := zstd.NewReader(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer zr.Close()
				var events []axiom.Event
				dec := json.NewDecoder(zr)
				for dec.More() {
					var e axiom.Event
					if err := dec.Decode(&e); err != nil {
						t.Error(err)
						break
					}
					events = append(events, e)
				}
				json.NewEncoder(w).Encode(map[string]int{"ingested": len(events), "failed": 0})
				got <- events
			})
			d := New(c, "default")
			f, path := stream(t, d)
			count := 1
			if mode == "batch" {
				count = 100
			}
			enc := logdriver.NewLogEntryEncoder(f)
			for i := 0; i < count; i++ {
				if err := enc.Encode(&logdriver.LogEntry{Line: []byte(fmt.Sprintf("marker-%d", i)), Source: "stdout", TimeNano: time.Now().UnixNano()}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "stop" {
				time.Sleep(50 * time.Millisecond)
				if err := d.StopLogging(path); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case events := <-got:
				if len(events) != count || events[0]["message"] != "marker-0" || events[0]["source"] != "stdout" {
					t.Fatalf("unexpected delivery: %v", events)
				}
			case <-time.After(2500 * time.Millisecond):
				t.Fatal("log delivery timed out")
			}
		})
	}
}

func TestRejectedEventsAreNotRetried(t *testing.T) {
	var calls atomic.Int32
	c := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ingested":1,"failed":1,"failures":[{"error":"invalid timestamp"}]}`)
	})
	err := New(c, "logs").ingestWithRetry(context.Background(), "logs", []axiom.Event{{"message": "accepted"}, {"message": "rejected"}})
	if err == nil {
		t.Fatal("partial rejection reported as success")
	}
	if calls.Load() != 1 {
		t.Fatalf("partial batch replayed %d times", calls.Load())
	}
}

func TestRequestFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		wantCalls int
		wantError bool
	}{
		{"transient", 503, 2, false}, {"unauthorized", 401, 1, true}, {"rate-limit", 429, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			c := receiver(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, `{"message":"failure"}`)
					return
				}
				fmt.Fprint(w, `{"ingested":1,"failed":0}`)
			})
			err := New(c, "logs").ingestWithRetry(context.Background(), "logs", []axiom.Event{{"message": "retry"}})
			if (err != nil) != tc.wantError || int(calls.Load()) != tc.wantCalls {
				t.Fatalf("error=%v requests=%d", err, calls.Load())
			}
		})
	}
}

func TestConcurrentStopDrainsFIFO(t *testing.T) {
	var count atomic.Int32
	c := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		zr, err := zstd.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		defer zr.Close()
		dec := json.NewDecoder(zr)
		n := 0
		for dec.More() {
			var e axiom.Event
			if err := dec.Decode(&e); err != nil {
				t.Error(err)
				break
			}
			n++
		}
		count.Add(int32(n))
		fmt.Fprintf(w, `{"ingested":%d,"failed":0}`, n)
	})
	d := New(c, "logs")
	f, path := stream(t, d)
	enc := logdriver.NewLogEntryEncoder(f)
	for i := 0; i < 75; i++ {
		if err := enc.Encode(&logdriver.LogEntry{Line: []byte("drain"), TimeNano: time.Now().UnixNano()}); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { done <- d.StopLogging(path) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 75 {
		t.Fatalf("delivered %d of 75 pending records", count.Load())
	}
}

func TestShutdownBoundsStalledIngestion(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	c := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	d := New(c, "logs")
	f, path := stream(t, d)
	if err := logdriver.NewLogEntryEncoder(f).Encode(&logdriver.LogEntry{Line: []byte("stall"), TimeNano: time.Now().UnixNano()}); err != nil {
		t.Fatal(err)
	}
	<-started
	start := time.Now()
	if err := d.StopLogging(path); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 32*time.Second {
		t.Fatalf("shutdown exceeded deadline: %v", elapsed)
	}
}
