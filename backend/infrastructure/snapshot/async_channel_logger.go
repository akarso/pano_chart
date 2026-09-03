package snapshot

import (
	"fmt"
	"sync"
	"time"

	"pano_chart/backend/domain"
)

// AsyncChannelLogger buffers snapshots on a channel and flushes them
// in batches via a dedicated goroutine. This prevents scoring slowdown
// from synchronous IO.
type AsyncChannelLogger struct {
	ch      chan domain.EvaluationSnapshot
	sink    func([]domain.EvaluationSnapshot) // batch writer injected at construction
	done    chan struct{}
	once    sync.Once
	batchSz int
	timeout time.Duration
}

// NewAsyncChannelLogger creates a buffered async logger.
//
// Parameters:
//   - bufferSize: channel capacity (e.g. 1000)
//   - batchSize: max snapshots per flush
//   - flushInterval: max wait before flushing a partial batch
//   - sink: function that persists a batch of snapshots
func NewAsyncChannelLogger(bufferSize, batchSize int, flushInterval time.Duration, sink func([]domain.EvaluationSnapshot)) *AsyncChannelLogger {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	if batchSize <= 0 {
		batchSize = 50
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	l := &AsyncChannelLogger{
		ch:      make(chan domain.EvaluationSnapshot, bufferSize),
		sink:    sink,
		done:    make(chan struct{}),
		batchSz: batchSize,
		timeout: flushInterval,
	}
	go l.drain()
	return l
}

// Log enqueues a snapshot for async persistence.
// Returns an error only if the buffer is full (back-pressure).
func (l *AsyncChannelLogger) Log(snap domain.EvaluationSnapshot) error {
	select {
	case l.ch <- snap:
		return nil
	default:
		return fmt.Errorf("snapshot buffer full, dropping snapshot for %s", snap.Symbol)
	}
}

// Stop gracefully shuts down the drain goroutine and flushes remaining items.
func (l *AsyncChannelLogger) Stop() {
	l.once.Do(func() {
		close(l.ch)
		<-l.done
	})
}

// drain runs in a dedicated goroutine, batching and flushing snapshots.
func (l *AsyncChannelLogger) drain() {
	defer close(l.done)

	batch := make([]domain.EvaluationSnapshot, 0, l.batchSz)
	ticker := time.NewTicker(l.timeout)
	defer ticker.Stop()

	for {
		select {
		case snap, ok := <-l.ch:
			if !ok {
				// Channel closed — flush remaining.
				if len(batch) > 0 {
					l.sink(batch)
				}
				return
			}
			batch = append(batch, snap)
			if len(batch) >= l.batchSz {
				l.sink(batch)
				batch = make([]domain.EvaluationSnapshot, 0, l.batchSz)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.sink(batch)
				batch = make([]domain.EvaluationSnapshot, 0, l.batchSz)
			}
		}
	}
}
