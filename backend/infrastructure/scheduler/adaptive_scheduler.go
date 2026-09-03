package scheduler

import (
	"sync"
	"time"
)

type Priority int

const (
	PriorityTop20 Priority = iota
	PriorityTop100
	PriorityRest
)

var priorityIntervals = map[Priority]time.Duration{
	PriorityTop20:  30 * time.Second,
	PriorityTop100: 60 * time.Second,
	PriorityRest:   120 * time.Second,
}

type SymbolTimeframe struct {
	Symbol    string
	Timeframe string
}

type AdaptiveScheduler struct {
	mu        sync.Mutex
	priority  map[SymbolTimeframe]Priority
	lastFetch map[SymbolTimeframe]time.Time
}

func NewAdaptiveScheduler() *AdaptiveScheduler {
	return &AdaptiveScheduler{
		priority:  make(map[SymbolTimeframe]Priority),
		lastFetch: make(map[SymbolTimeframe]time.Time),
	}
}

func (a *AdaptiveScheduler) SetPriority(st SymbolTimeframe, p Priority) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.priority[st] = p
}

func (a *AdaptiveScheduler) MarkRefreshed(st SymbolTimeframe, t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastFetch[st] = t
}

func (a *AdaptiveScheduler) Due(now time.Time) []SymbolTimeframe {
	a.mu.Lock()
	defer a.mu.Unlock()
	var due []SymbolTimeframe
	for st, p := range a.priority {
		last, ok := a.lastFetch[st]
		interval := priorityIntervals[p]
		if !ok || now.Sub(last) >= interval {
			due = append(due, st)
		}
	}
	return due
}
