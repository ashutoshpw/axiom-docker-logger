// Command smokereceiver validates real Docker plugin delivery without Axiom credentials.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

func main() {
	ready := flag.String("ready", "", "file receiving the listener URL")
	flag.Parse()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	var mu sync.Mutex
	seen := map[string]int{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/datasets/smoke/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer xaat-smoke" {
			http.Error(w, "invalid request", 400)
			return
		}
		zr, err := zstd.NewReader(r.Body)
		if err != nil {
			http.Error(w, "invalid compression", 400)
			return
		}
		defer zr.Close()
		dec := json.NewDecoder(zr)
		count := 0
		for {
			var event map[string]any
			err := dec.Decode(&event)
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "invalid event", 400)
				return
			}
			message, _ := event["message"].(string)
			source, _ := event["source"].(string)
			if event["container_id"] == "" || event["container_name"] == "" || event["_time"] == nil {
				http.Error(w, "missing metadata", 400)
				return
			}
			mu.Lock()
			seen[source+":"+strings.TrimSpace(message)]++
			mu.Unlock()
			count++
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ingested":%d,"failed":0}`, count)
	})
	mux.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		for _, key := range r.URL.Query()["key"] {
			if seen[key] != 1 {
				http.Error(w, "marker missing or duplicated: "+key, 409)
				return
			}
		}
		w.WriteHeader(204)
	})
	if err := os.WriteFile(*ready, []byte("http://"+listener.Addr().String()), 0600); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.Serve(listener, mux))
}
