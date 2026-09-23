package scoring_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"pano_chart/backend/domain"
	infrascoring "pano_chart/backend/infrastructure/scoring"
)

type fakeCalc struct {
	name  string
	score float64
	err   error
}

func (f *fakeCalc) Name() string { return f.name }

func (f *fakeCalc) Score(_ domain.CandleSeries) (float64, error) {
	return f.score, f.err
}

func TestLoggingScoreCalculator_DelegatesNameAndScore(t *testing.T) {
	inner := &fakeCalc{name: "Sideways Consistency", score: 0.42}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1.0)

	if calc.Name() != "Sideways Consistency" {
		t.Errorf("expected Name() to delegate, got %q", calc.Name())
	}

	score, err := calc.Score(domain.CandleSeries{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0.42 {
		t.Errorf("expected Score() to delegate the inner value, got %v", score)
	}
}

func TestLoggingScoreCalculator_PropagatesInnerError(t *testing.T) {
	innerErr := errors.New("boom")
	inner := &fakeCalc{name: "X", err: innerErr}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1.0)

	_, err := calc.Score(domain.CandleSeries{})
	if !errors.Is(err, innerErr) {
		t.Errorf("expected the inner error to propagate unchanged, got %v", err)
	}
}

func TestLoggingScoreCalculator_ZeroSampleRateNeverPanics(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 0)

	for i := 0; i < 50; i++ {
		if _, err := calc.Score(domain.CandleSeries{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

type sampleCall struct {
	calculator string
	symbol     string
	tf         string
	score      float64
}

type recordingSink struct {
	mu    sync.Mutex
	calls []sampleCall
	err   error
}

func (s *recordingSink) Record(_ context.Context, calculator, symbol, tf string, score float64, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, sampleCall{calculator, symbol, tf, score})
	return s.err
}

func (s *recordingSink) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestLoggingScoreCalculator_SetSinkRecordsInsteadOfLogging(t *testing.T) {
	logs := captureLog(t)
	inner := &fakeCalc{name: "Sideways Consistency", score: 0.42}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1)
	sink := &recordingSink{}
	calc.SetSink(sink)

	series, err := domain.NewCandleSeries(domain.NewSymbolUnsafe("btcusdt"), domain.NewTimeframeUnsafe("1h"), nil)
	if err != nil {
		t.Fatal(err)
	}
	score, err := calc.Score(series)
	if err != nil {
		t.Fatal(err)
	}
	if score != 0.42 {
		t.Fatalf("score=%v", score)
	}
	if sink.len() != 1 {
		t.Fatalf("calls=%d", sink.len())
	}
	got := sink.calls[0]
	if got.calculator != "Sideways Consistency" || got.symbol != "BTCUSDT" || got.tf != "1h" || got.score != 0.42 {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(logs.String(), "score=") {
		t.Fatalf("logged score line: %s", logs.String())
	}
}

func TestLoggingScoreCalculator_ZeroRateDoesNotRecord(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 0)
	sink := &recordingSink{}
	calc.SetSink(sink)
	if _, err := calc.Score(domain.CandleSeries{}); err != nil {
		t.Fatal(err)
	}
	if sink.len() != 0 {
		t.Fatalf("calls=%d", sink.len())
	}
}

func TestLoggingScoreCalculator_InnerErrorSkipsSink(t *testing.T) {
	innerErr := errors.New("boom")
	inner := &fakeCalc{name: "X", err: innerErr}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1)
	sink := &recordingSink{}
	calc.SetSink(sink)
	_, err := calc.Score(domain.CandleSeries{})
	if !errors.Is(err, innerErr) {
		t.Fatalf("err=%v", err)
	}
	if sink.len() != 0 {
		t.Fatalf("calls=%d", sink.len())
	}
}

func TestLoggingScoreCalculator_NilSinkRestoresLogging(t *testing.T) {
	logs := captureLog(t)
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1)
	calc.SetSink(&recordingSink{})
	calc.SetSink(nil)
	if _, err := calc.Score(domain.CandleSeries{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "score=") {
		t.Fatalf("log=%q", logs.String())
	}
}

func TestLoggingScoreCalculator_SetSinkConcurrentWithScore(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1)
	sink := &recordingSink{}
	series, err := domain.NewCandleSeries(domain.NewSymbolUnsafe("btcusdt"), domain.NewTimeframeUnsafe("1h"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			calc.SetSink(sink)
			calc.SetSink(nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if _, err := calc.Score(series); err != nil {
				t.Errorf("score: %v", err)
			}
		}
	}()
	wg.Wait()
}

func TestLoggingScoreCalculator_SinkErrorDoesNotFailScore(t *testing.T) {
	logs := captureLog(t)
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1)
	calc.SetSink(&recordingSink{err: errors.New("disk")})
	score, err := calc.Score(domain.CandleSeries{})
	if err != nil || score != 0.5 {
		t.Fatalf("score=%v err=%v", score, err)
	}
	if !strings.Contains(logs.String(), "score sample") {
		t.Fatalf("log=%q", logs.String())
	}
}

func TestSampleRateFromEnv(t *testing.T) {
	if got := infrascoring.SampleRateFromEnv(""); got != 0.1 {
		t.Fatalf("empty=%v", got)
	}
	if got := infrascoring.SampleRateFromEnv("nope"); got != 0.1 {
		t.Fatalf("invalid=%v", got)
	}
	if got := infrascoring.SampleRateFromEnv(" 0.25 "); got != 0.25 {
		t.Fatalf("parsed=%v", got)
	}
	for _, raw := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf"} {
		if got := infrascoring.SampleRateFromEnv(raw); got != 0.1 {
			t.Fatalf("%s=%v", raw, got)
		}
	}
}

func TestNewLoggingScoreCalculator_NaNDoesNotSample(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, math.NaN())
	sink := &recordingSink{}
	calc.SetSink(sink)
	if _, err := calc.Score(domain.CandleSeries{}); err != nil {
		t.Fatal(err)
	}
	if sink.len() != 0 {
		t.Fatalf("calls=%d", sink.len())
	}
}

func TestNewLoggingScoreCalculator_ClampsSampleRate(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}

	// Out-of-range sample rates must not panic or misbehave — just clamp.
	tooHigh := infrascoring.NewLoggingScoreCalculator(inner, 5.0)
	tooLow := infrascoring.NewLoggingScoreCalculator(inner, -5.0)

	if _, err := tooHigh.Score(domain.CandleSeries{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := tooLow.Score(domain.CandleSeries{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
