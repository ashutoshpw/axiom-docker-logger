package driver

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/axiomhq/axiom-go/axiom"
	"github.com/axiomhq/axiom-go/axiom/ingest"
	"github.com/containerd/fifo"
	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	protoio "github.com/gogo/protobuf/io"
	log "github.com/sirupsen/logrus"
)

const (
	// Buffer config
	defaultBatchSize     = 100
	defaultFlushInterval = 1 * time.Second

	// Retry config
	maxRetries     = 3
	initialBackoff = 1 * time.Second
	backoffFactor  = 2
)

// containerLogger holds the state for a single container's log stream.
type containerLogger struct {
	file   string
	info   logger.Info
	stream io.ReadCloser
	cancel context.CancelFunc
}

// Driver implements the Docker logging driver interface.
type Driver struct {
	mu      sync.Mutex
	logs    map[string]*containerLogger
	client  *axiom.Client
	dataset string
}

// New creates a new Driver with the given Axiom client and default dataset.
func New(client *axiom.Client, dataset string) *Driver {
	return &Driver{
		logs:    make(map[string]*containerLogger),
		client:  client,
		dataset: dataset,
	}
}

// StartLogging is called by Docker when a container starts.
// file is the path to the FIFO pipe where Docker streams protobuf-encoded log entries.
func (d *Driver) StartLogging(file string, info logger.Info) error {
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		return fmt.Errorf("logger already exists for %s", file)
	}
	d.mu.Unlock()

	log.WithFields(log.Fields{
		"container_id": info.ContainerID[:12],
		"file":         file,
	}).Info("Starting logging")

	// Open the FIFO pipe. Docker writes protobuf log entries here.
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := fifo.OpenFifo(ctx, file, syscall.O_RDONLY, 0)
	if err != nil {
		cancel()
		return fmt.Errorf("opening fifo: %w", err)
	}

	cl := &containerLogger{
		file:   file,
		info:   info,
		stream: stream,
		cancel: cancel,
	}

	d.mu.Lock()
	d.logs[file] = cl
	d.mu.Unlock()

	// Determine which Axiom dataset to use.
	// Per-container override via --log-opt axiom-dataset=... takes precedence.
	dataset := d.dataset
	if ds, ok := info.Config["axiom-dataset"]; ok && ds != "" {
		dataset = ds
	}

	go d.consumeLogs(ctx, cl, dataset)

	return nil
}

// StopLogging is called by Docker when a container stops.
func (d *Driver) StopLogging(file string) error {
	d.mu.Lock()
	cl, ok := d.logs[file]
	if !ok {
		d.mu.Unlock()
		return nil
	}
	delete(d.logs, file)
	d.mu.Unlock()

	log.WithField("file", file).Info("Stopping logging")

	// Signal the consumer goroutine to stop.
	cl.cancel()
	// Close the FIFO stream.
	cl.stream.Close()

	return nil
}

// consumeLogs reads protobuf log entries from the FIFO, batches them,
// and sends them to Axiom with retry on failure.
func (d *Driver) consumeLogs(ctx context.Context, cl *containerLogger, dataset string) {
	dec := protoio.NewUint32DelimitedReader(cl.stream, binary.BigEndian, 1e6)
	defer dec.Close()

	var (
		buf    logdriver.LogEntry
		batch  []axiom.Event
		ticker = time.NewTicker(defaultFlushInterval)
	)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Copy the batch so we can reset it immediately and keep reading.
		toSend := batch
		batch = nil

		if err := d.ingestWithRetry(ctx, dataset, toSend); err != nil {
			log.WithError(err).WithField("count", len(toSend)).
				Error("Failed to ingest events after retries, events dropped")
		}
	}

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit.
			flush()
			return
		case <-ticker.C:
			flush()
		default:
			if err := dec.ReadMsg(&buf); err != nil {
				if err == io.EOF || ctx.Err() != nil {
					flush()
					return
				}
				// Reset the decoder on transient read errors and continue.
				log.WithError(err).Warn("Error reading log entry, resetting decoder")
				dec = protoio.NewUint32DelimitedReader(cl.stream, binary.BigEndian, 1e6)
				continue
			}

			event := d.toAxiomEvent(&buf, cl.info)
			batch = append(batch, event)
			buf.Reset()

			if len(batch) >= defaultBatchSize {
				flush()
			}
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

// ingestWithRetry attempts to send events to Axiom, retrying with exponential
// backoff on failure. Returns nil on success, or the last error after all
// retries are exhausted.
func (d *Driver) ingestWithRetry(ctx context.Context, dataset string, events []axiom.Event) error {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.WithFields(log.Fields{
				"attempt": attempt,
				"backoff": backoff,
				"count":   len(events),
			}).Warn("Retrying Axiom ingest")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= backoffFactor
		}

		_, err := d.client.IngestEvents(ctx, dataset, events)
		if err == nil {
			if attempt > 0 {
				log.WithField("attempt", attempt).Info("Axiom ingest succeeded after retry")
			}
			return nil
		}
		lastErr = err
		log.WithError(err).WithFields(log.Fields{
			"attempt": attempt,
			"count":   len(events),
		}).Warn("Axiom ingest failed")
	}

	return fmt.Errorf("ingest failed after %d retries: %w", maxRetries, lastErr)
}
