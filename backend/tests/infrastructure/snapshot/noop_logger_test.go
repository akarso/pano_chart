package snapshot_test

import (
	"testing"

	"pano_chart/backend/domain"
	"pano_chart/backend/infrastructure/snapshot"
)

func TestNoopLogger_LogReturnsNil(t *testing.T) {
	logger := &snapshot.NoopLogger{}
	err := logger.Log(domain.EvaluationSnapshot{
		Symbol:    "BTCUSDT",
		Timeframe: "1h",
	})
	if err != nil {
		t.Errorf("NoopLogger.Log should return nil, got %v", err)
	}
}

func TestNoopLogger_LogMultipleDoesNotPanic(t *testing.T) {
	logger := &snapshot.NoopLogger{}
	for i := 0; i < 1000; i++ {
		_ = logger.Log(domain.EvaluationSnapshot{Symbol: "TEST"})
	}
}
