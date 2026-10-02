package main

import (
	"testing"
	"time"

	domainsignal "pano_chart/backend/domain/signal"
)

func TestOutcomeInSnapshot(t *testing.T) {
	asOf := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if outcomeInSnapshot(nil, asOf) {
		t.Fatal("nil outcome")
	}
	before := &domainsignal.Outcome{ResolvedAt: asOf.Add(-time.Hour)}
	if !outcomeInSnapshot(before, asOf) {
		t.Fatal("resolved before asOf should be in snapshot")
	}
	at := &domainsignal.Outcome{ResolvedAt: asOf}
	if !outcomeInSnapshot(at, asOf) {
		t.Fatal("resolved_at == asOf should be in snapshot")
	}
	after := &domainsignal.Outcome{ResolvedAt: asOf.Add(time.Second)}
	if outcomeInSnapshot(after, asOf) {
		t.Fatal("resolved after asOf must be excluded")
	}
}
