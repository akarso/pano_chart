package mtf_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appmtf "pano_chart/backend/application/mtf"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// fakeEvalStore serves canned snapshots keyed by "tf:symbol". Put/Get are
// unused by mtf.Service and are stubbed only to satisfy ports.EvaluationStore.
type fakeEvalStore struct {
	byKey map[string]storedSnap
	err   error // when set, every GetSymbol call returns this error
}

type storedSnap struct {
	snap domain.EvaluationSnapshot
	at   time.Time
}

func newFakeEvalStore() *fakeEvalStore {
	return &fakeEvalStore{byKey: map[string]storedSnap{}}
}

func (f *fakeEvalStore) put(tf, symbol string, snap domain.EvaluationSnapshot, at time.Time) {
	f.byKey[tf+":"+symbol] = storedSnap{snap: snap, at: at}
}

func (f *fakeEvalStore) Put(context.Context, string, []domain.EvaluationSnapshot, time.Time) error {
	return nil
}

func (f *fakeEvalStore) Get(context.Context, string) ([]domain.EvaluationSnapshot, time.Time, error) {
	return nil, time.Time{}, nil
}

func (f *fakeEvalStore) GetSymbol(_ context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error) {
	if f.err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, f.err
	}
	e, ok := f.byKey[tf+":"+symbol]
	if !ok {
		return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
	}
	return e.snap, e.at, nil
}

var _ ports.EvaluationStore = (*fakeEvalStore)(nil)

func trendSnapshot() domain.EvaluationSnapshot {
	return domain.EvaluationSnapshot{
		TrendScore:       0.9,
		SidewaysScore:    0.05,
		CompressionScore: 0.03,
		BreakoutUpScore:  0.02,
		Bias:             "up",
		AlgoVersion:      domain.AlgoVersion,
	}
}

func sidewaysSnapshot() domain.EvaluationSnapshot {
	return domain.EvaluationSnapshot{
		TrendScore:       0.05,
		SidewaysScore:    0.9,
		CompressionScore: 0.03,
		BreakoutUpScore:  0.02,
		Bias:             "neutral",
		AlgoVersion:      domain.AlgoVersion,
	}
}

// putAll writes snap under every timeframe in tfs, all fresh as of now.
func putAll(store *fakeEvalStore, symbol string, tfs []string, snap domain.EvaluationSnapshot) {
	now := time.Now()
	for _, tf := range tfs {
		store.put(tf, symbol, snap, now)
	}
}

func TestCalculate_FourFramesAllTrend_AlignmentOne(t *testing.T) {
	store := newFakeEvalStore()
	putAll(store, "BTCUSDT", []string{"15m", "1h", "4h", "1d"}, trendSnapshot())

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	if len(stack.Frames) != 4 {
		t.Fatalf("expected 4 frames, got %d: %+v", len(stack.Frames), stack.Frames)
	}
	if stack.Alignment != 1.0 {
		t.Fatalf("expected alignment 1.0, got %g", stack.Alignment)
	}
	if stack.AlignedState != mkt.StateTrend {
		t.Fatalf("expected alignedState trend, got %s", stack.AlignedState)
	}
	for _, f := range stack.Frames {
		if f.Dominant != mkt.StateTrend {
			t.Errorf("frame %s: expected dominant trend, got %s", f.Timeframe, f.Dominant)
		}
		if f.Bias != "up" {
			t.Errorf("frame %s: expected bias up, got %s", f.Timeframe, f.Bias)
		}
	}
}

func TestCalculate_TwoTrendTwoSideways_HalfAlignment_Indecisive(t *testing.T) {
	store := newFakeEvalStore()
	now := time.Now()
	store.put("15m", "ETHUSDT", trendSnapshot(), now)
	store.put("1h", "ETHUSDT", trendSnapshot(), now)
	store.put("4h", "ETHUSDT", sidewaysSnapshot(), now)
	store.put("1d", "ETHUSDT", sidewaysSnapshot(), now)

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "ETHUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	if len(stack.Frames) != 4 {
		t.Fatalf("expected 4 frames, got %d", len(stack.Frames))
	}
	if stack.Alignment != 0.5 {
		t.Fatalf("expected alignment 0.5, got %g", stack.Alignment)
	}
	if stack.AlignedState != mkt.StateIndecisive {
		t.Fatalf("expected alignedState indecisive, got %s", stack.AlignedState)
	}
}

func TestCalculate_MissingFrame_Skipped(t *testing.T) {
	store := newFakeEvalStore()
	now := time.Now()
	// Only 3 of 4 timeframes populated — "1d" is a miss.
	store.put("15m", "SOLUSDT", trendSnapshot(), now)
	store.put("1h", "SOLUSDT", trendSnapshot(), now)
	store.put("4h", "SOLUSDT", trendSnapshot(), now)

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "SOLUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	if len(stack.Frames) != 3 {
		t.Fatalf("expected 3 frames (1d missing), got %d", len(stack.Frames))
	}
	if stack.Alignment != 1.0 {
		t.Fatalf("expected alignment 1.0 over the 3 present frames, got %g", stack.Alignment)
	}
	if stack.AlignedState != mkt.StateTrend {
		t.Fatalf("expected alignedState trend, got %s", stack.AlignedState)
	}
}

func TestCalculate_StaleFrame_Skipped(t *testing.T) {
	store := newFakeEvalStore()
	now := time.Now()
	store.put("15m", "ADAUSDT", trendSnapshot(), now)
	store.put("1h", "ADAUSDT", trendSnapshot(), now)
	store.put("4h", "ADAUSDT", trendSnapshot(), now)
	// "1d"'s staleAfter is 12h; 48h ago is well past it on every timeframe.
	store.put("1d", "ADAUSDT", trendSnapshot(), now.Add(-48*time.Hour))

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "ADAUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	if len(stack.Frames) != 3 {
		t.Fatalf("expected 3 frames (1d stale), got %d", len(stack.Frames))
	}
	for _, f := range stack.Frames {
		if f.Timeframe == "1d" {
			t.Fatalf("stale 1d frame must not appear")
		}
	}
}

func TestCalculate_AlgoVersionMismatch_Skipped(t *testing.T) {
	store := newFakeEvalStore()
	now := time.Now()
	store.put("15m", "XRPUSDT", trendSnapshot(), now)
	store.put("1h", "XRPUSDT", trendSnapshot(), now)
	store.put("4h", "XRPUSDT", trendSnapshot(), now)
	stale := trendSnapshot()
	stale.AlgoVersion = "v0.0.0-old"
	store.put("1d", "XRPUSDT", stale, now)

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "XRPUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if len(stack.Frames) != 3 {
		t.Fatalf("expected 3 frames (1d algo mismatch), got %d", len(stack.Frames))
	}
}

func TestCalculate_NoFramesAvailable_ZeroAlignmentIndecisive(t *testing.T) {
	store := newFakeEvalStore() // nothing stored for this symbol

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "NEWUSDT")
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if len(stack.Frames) != 0 {
		t.Fatalf("expected 0 frames, got %d", len(stack.Frames))
	}
	if stack.Alignment != 0 {
		t.Fatalf("expected alignment 0, got %g", stack.Alignment)
	}
	if stack.AlignedState != mkt.StateIndecisive {
		t.Fatalf("expected alignedState indecisive, got %s", stack.AlignedState)
	}
}

func TestCalculate_StoreCancellation_PropagatesError(t *testing.T) {
	store := newFakeEvalStore()
	store.err = context.Canceled

	svc := appmtf.NewService(store)
	_, err := svc.Calculate(context.Background(), "BTCUSDT")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCalculate_StoreTransportError_SkipsFrameNotAbort(t *testing.T) {
	store := newFakeEvalStore()
	store.err = errors.New("redis: connection refused")

	svc := appmtf.NewService(store)
	stack, err := svc.Calculate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("expected no error (frames just skipped), got %v", err)
	}
	if len(stack.Frames) != 0 {
		t.Fatalf("expected 0 frames, got %d", len(stack.Frames))
	}
	if stack.AlignedState != mkt.StateIndecisive {
		t.Fatalf("expected alignedState indecisive, got %s", stack.AlignedState)
	}
}
