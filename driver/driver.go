package driver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/axiomhq/axiom-go/axiom"
	"github.com/axiomhq/axiom-go/axiom/ingest"
	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	protoio "github.com/gogo/protobuf/io"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = time.Second
	shutdownTimeout      = 30 * time.Second
	ingestTimeout        = 10 * time.Second
	maxRetries           = 3
	initialBackoff       = time.Second
)

type containerLogger struct {
	info     logger.Info
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	cancel   context.CancelFunc
}

type Driver struct {
	mu      sync.Mutex
	logs    map[string]*containerLogger
	client  *axiom.Client
	dataset string
}

func New(client *axiom.Client, dataset string) *Driver {
	return &Driver{logs: make(map[string]*containerLogger), client: client, dataset: dataset}
}

func (d *Driver) StartLogging(file string, info logger.Info) error {
	if _, ok := info.Config["axiom-token"]; ok {
		return errors.New("axiom-token is unsupported; configure AXIOM_TOKEN on the Docker plugin")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cl := &containerLogger{info: info, stop: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	// Reserve the path before opening; no mutex is held over filesystem or HTTP IO.
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		cancel()
		return fmt.Errorf("logger already exists for %s", file)
	}
	d.logs[file] = cl
	d.mu.Unlock()
	fd, err := unix.Open(file, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		cancel()
		close(cl.done)
		d.mu.Lock()
		if d.logs[file] == cl {
			delete(d.logs, file)
		}
		d.mu.Unlock()
		return fmt.Errorf("opening log fifo: %w", err)
	}
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFIFO {
		unix.Close(fd)
		cancel()
		close(cl.done)
		d.mu.Lock()
		if d.logs[file] == cl {
			delete(d.logs, file)
		}
		d.mu.Unlock()
		return errors.New("log stream must be a FIFO")
	}
	dataset := d.dataset
	if ds := info.Config["axiom-dataset"]; ds != "" {
		dataset = ds
	}
	go d.consumeLogs(ctx, cl, fd, dataset)
	return nil
}

func (d *Driver) StopLogging(file string) error {
	d.mu.Lock()
	cl := d.logs[file]
	d.mu.Unlock()
	if cl == nil {
		return nil
	}
	cl.stopOnce.Do(func() {
		close(cl.stop)
		go func() {
			timer := time.NewTimer(shutdownTimeout)
			defer timer.Stop()
			select {
			case <-cl.done:
			case <-timer.C:
				cl.cancel()
			}
		}()
	})
	<-cl.done
	d.mu.Lock()
	if d.logs[file] == cl {
		delete(d.logs, file)
	}
	d.mu.Unlock()
	// Acknowledge lifecycle completion even on delivery failure, allowing Docker to
	// close/remove its writer. Delivery failures are reported by the consumer.
	return nil
}

// fifoReader uses nonblocking reads so StopLogging can drain available bytes
// while Docker still holds its writer open. Only the reader owns and closes fd.
type fifoReader struct {
	fd   int
	ctx  context.Context
	stop <-chan struct{}
}

func (r *fifoReader) Read(p []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		n, err := unix.Read(r.fd, p)
		if n > 0 {
			return n, nil
		}
		if err == nil {
			return 0, io.EOF
		}
		if err == unix.EINTR {
			continue
		}
		if err != unix.EAGAIN {
			return 0, err
		}
		select {
		case <-r.stop:
			return 0, io.EOF
		default:
		}
		_, err = unix.Poll([]unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}, 100)
		if err != nil && err != unix.EINTR {
			return 0, err
		}
	}
}
func (r *fifoReader) Close() error { return unix.Close(r.fd) }

func (d *Driver) consumeLogs(ctx context.Context, cl *containerLogger, fd int, dataset string) {
	defer close(cl.done)
	defer cl.cancel()
	events := make(chan axiom.Event, defaultBatchSize)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer close(events)
		reader := &fifoReader{fd: fd, ctx: ctx, stop: cl.stop}
		dec := protoio.NewUint32DelimitedReader(reader, binary.BigEndian, 1e6)
		defer dec.Close()
		for {
			var entry logdriver.LogEntry
			if err := dec.ReadMsg(&entry); err != nil {
				if err != io.EOF {
					log.WithField("container_id", cl.info.ContainerID).Warn("Log stream ended with an incomplete record or read error")
				}
				return
			}
			event := d.toAxiomEvent(&entry, cl.info)
			select {
			case events <- event:
			case <-ctx.Done():
				log.Warn("Shutdown deadline discarded a decoded log record")
				return
			}
		}
	}()
	ticker := time.NewTicker(defaultFlushInterval)
	defer ticker.Stop()
	batch := make([]axiom.Event, 0, defaultBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := d.ingestWithRetry(ctx, dataset, batch); err != nil {
			log.WithFields(log.Fields{"container_id": cl.info.ContainerID, "dataset": dataset, "count": len(batch), "reason": err.Error()}).Error("Log batch was not fully delivered")
		}
		batch = make([]axiom.Event, 0, defaultBatchSize)
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				flush()
				<-readDone
				return
			}
			batch = append(batch, event)
			if len(batch) == defaultBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			<-readDone
			log.WithFields(log.Fields{"container_id": cl.info.ContainerID, "undelivered_events": len(batch) + len(events)}).Error("Shutdown deadline exceeded; remaining FIFO bytes may also be lost")
			return
		}
	}
}

// toAxiomEvent converts a Docker protobuf log entry to an Axiom event.
func (d *Driver) toAxiomEvent(entry *logdriver.LogEntry, info logger.Info) axiom.Event {
	event := axiom.Event{
		ingest.TimestampField: time.Unix(0, entry.TimeNano),
		"message":             string(entry.Line),
		"source":              entry.Source,
		"partial":             entry.Partial,
		"container_id":        info.ContainerID,
		"container_name":      info.ContainerName,
		"container_image":     info.ContainerImageName,
		"daemon_name":         info.DaemonName,
	}

	// Add container labels.
	if len(info.ContainerLabels) > 0 {
		labels := make(map[string]string, len(info.ContainerLabels))
		for k, v := range info.ContainerLabels {
			labels[k] = v
		}
		event["labels"] = labels
	}

	// Add partial log metadata if present.
	if entry.PartialLogMetadata != nil {
		event["partial_id"] = entry.PartialLogMetadata.Id
		event["partial_ordinal"] = entry.PartialLogMetadata.Ordinal
		event["partial_last"] = entry.PartialLogMetadata.Last
	}

	return event
}

// Retry only request failures whose result may recover. Never replay partial
// ingestion: the status identifies counts, not the original rejected records.
func (d *Driver) ingestWithRetry(ctx context.Context, dataset string, events []axiom.Event) error {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			timer := time.NewTimer(initialBackoff * time.Duration(1<<(attempt-1)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		requestCtx, cancel := context.WithTimeout(ctx, ingestTimeout)
		status, err := d.client.IngestEvents(requestCtx, dataset, events)
		cancel()
		if err == nil {
			if status == nil {
				return errors.New("ingestion response omitted status")
			}
			if status.Failed > 0 || status.Ingested != uint64(len(events)) {
				return fmt.Errorf("Axiom accepted %d of %d events; rejected %d (batch will not be replayed)", status.Ingested, len(events), status.Failed)
			}
			return nil
		}
		var httpErr axiom.HTTPError
		var limitErr axiom.LimitError
		var networkErr net.Error
		retryable := errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkErr)
		if errors.As(err, &limitErr) {
			httpErr = limitErr.HTTPError
		}
		if errors.As(err, &httpErr) || errors.As(err, &limitErr) {
			retryable = httpErr.Status == 429 || httpErr.Status >= 500
		}
		if !retryable || attempt == maxRetries {
			// SDK/server errors may include request URLs or event contents; expose only
			// classifications and status codes in plugin diagnostics.
			if errors.As(err, &httpErr) || errors.As(err, &limitErr) {
				return fmt.Errorf("Axiom HTTP %d after %d attempt(s)", httpErr.Status, attempt+1)
			}
			return fmt.Errorf("Axiom request failed after %d attempt(s)", attempt+1)
		}
	}
	return errors.New("Axiom retry limit reached")
}
