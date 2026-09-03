package snapshot_test

import (
	"sync"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/infrastructure/snapshot"
)

func TestAsyncChannelLogger_FlushesOnBatchSize(t *testing.T) {
	var mu sync.Mutex
	var received []domain.EvaluationSnapshot

	sink := func(batch []domain.EvaluationSnapshot) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, batch...)
	}

	// batchSize=5, flushInterval=10s (won't trigger in this test)
	logger := snapshot.NewAsyncChannelLogger(100, 5, 10*time.Second, sink)

	for i := 0; i < 5; i++ {
		err := logger.Log(domain.EvaluationSnapshot{Symbol: "SYM"})
		if err != nil {
			t.Fatalf("Log returned error: %v", err)
		}
	}

	// Allow drain goroutine to process
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 5 {
		t.Errorf("expected 5 flushed items, got %d", count)
	}

	logger.Stop()
}

func TestAsyncChannelLogger_FlushesOnTimer(t *testing.T) {
	var mu sync.Mutex
	var received []domain.EvaluationSnapshot

	sink := func(batch []domain.EvaluationSnapshot) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, batch...)
	}

	// batchSize=100 (won't trigger), flushInterval=50ms
	logger := snapshot.NewAsyncChannelLogger(100, 100, 50*time.Millisecond, sink)

	for i := 0; i < 3; i++ {
		_ = logger.Log(domain.EvaluationSnapshot{Symbol: "SYM"})
	}

	// Wait for timer flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 flushed items via timer, got %d", count)
	}

	logger.Stop()
}

func TestAsyncChannelLogger_FlushesOnStop(t *testing.T) {
	var mu sync.Mutex
	var received []domain.EvaluationSnapshot

	sink := func(batch []domain.EvaluationSnapshot) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, batch...)
	}

	// Large batch and interval so neither triggers naturally
	logger := snapshot.NewAsyncChannelLogger(100, 1000, 10*time.Second, sink)

	for i := 0; i < 7; i++ {
		_ = logger.Log(domain.EvaluationSnapshot{Symbol: "SYM"})
	}

	logger.Stop()

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 7 {
		t.Errorf("expected 7 flushed on stop, got %d", count)
	}
}

func TestAsyncChannelLogger_DropsWhenFull(t *testing.T) {
	// Sink that blocks forever (simulating slow consumer)
	sink := func(batch []domain.EvaluationSnapshot) {
		select {} // block forever
	}

	// Buffer of 2, batch of 100 (never flushes by batch)
	logger := snapshot.NewAsyncChannelLogger(2, 100, 10*time.Second, sink)

	// Fill the buffer
	_ = logger.Log(domain.EvaluationSnapshot{Symbol: "A"})
	_ = logger.Log(domain.EvaluationSnapshot{Symbol: "B"})

	// This should return an error (buffer full)
	err := logger.Log(domain.EvaluationSnapshot{Symbol: "C"})
	if err == nil {
		t.Error("expected error when buffer is full, got nil")
	}
}

func TestAsyncChannelLogger_StopIsIdempotent(t *testing.T) {
	sink := func(batch []domain.EvaluationSnapshot) {}
	logger := snapshot.NewAsyncChannelLogger(10, 5, 50*time.Millisecond, sink)

	// Should not panic
	logger.Stop()
	logger.Stop()
	logger.Stop()
}
