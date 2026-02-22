package scheduler

import (
	"testing"
	"time"
)

func TestAdaptiveScheduler_StaggeredIntervals(t *testing.T) {
	sched := NewAdaptiveScheduler()
	// Top 20: every 30s, Top 100: every 60s, Rest: every 120s
	sched.SetPriority(SymbolTimeframe{"BTCUSDT", "5m"}, PriorityTop20)
	sched.SetPriority(SymbolTimeframe{"ETHUSDT", "5m"}, PriorityTop100)
	sched.SetPriority(SymbolTimeframe{"DOGEUSDT", "5m"}, PriorityRest)

	now := time.Now()
	// All should be due at start
	due := sched.Due(now)
	if len(due) != 3 {
		t.Errorf("expected all 3 due at start, got %d", len(due))
	}

	// Mark as refreshed
	for _, st := range due {
		sched.MarkRefreshed(st, now)
	}

	// After 31s, only Top 20 should be due
	due = sched.Due(now.Add(31 * time.Second))
	if len(due) != 1 || due[0].Symbol != "BTCUSDT" {
		t.Errorf("expected only BTCUSDT due after 31s, got %+v", due)
	}

	// After 61s, Top 20 and Top 100 should be due
	due = sched.Due(now.Add(61 * time.Second))
	if len(due) != 2 {
		t.Errorf("expected 2 due after 61s, got %+v", due)
	}
}
