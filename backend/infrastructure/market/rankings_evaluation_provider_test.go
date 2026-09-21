package market

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type stubRankings struct {
	mu         sync.Mutex
	calls      int
	err        error
	rows       []usecases.RankedResult
	blockCh    chan struct{}
	entered    chan struct{}
	finished   chan struct{}
	once       sync.Once
	finishOnce sync.Once
}

func (s *stubRankings) Execute(ctx context.Context, _ usecases.GetRankingsRequest) ([]usecases.RankedResult, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	defer s.finishOnce.Do(func() {
		if s.finished != nil {
			close(s.finished)
		}
	})
	if s.entered != nil {
		s.once.Do(func() { close(s.entered) })
	}
	if s.blockCh != nil {
		select {
		case <-s.blockCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func (s *stubRankings) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type stubEvalStore struct {
	evals  []domain.EvaluationSnapshot
	at     time.Time
	getErr error
}

func (s *stubEvalStore) Put(context.Context, string, []domain.EvaluationSnapshot, time.Time) error {
	return nil
}

func (s *stubEvalStore) Get(_ context.Context, _ string) ([]domain.EvaluationSnapshot, time.Time, error) {
	if s.getErr != nil {
		return nil, time.Time{}, s.getErr
	}
	return s.evals, s.at, nil
}

func (s *stubEvalStore) GetSymbol(context.Context, string, string) (domain.EvaluationSnapshot, time.Time, error) {
	return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
}

func freshSnap(sym string) domain.EvaluationSnapshot {
	return domain.EvaluationSnapshot{
		Symbol:      sym,
		TrendScore:  0.9,
		AlgoVersion: domain.AlgoVersion,
	}
}

func TestRankingsEvaluationProvider_FreshStoreHitSkipsRankings(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{freshSnap("BTCUSDT")},
		at:    now.Add(-time.Minute),
	}
	rankings := &stubRankings{rows: []usecases.RankedResult{{}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if len(got) != 1 || got[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected store snapshot, got %#v", got)
	}
	if rankings.callCount() != 0 {
		t.Fatalf("expected zero rankings calls on fresh hit, got %d", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_ExactStaleBoundaryIsFresh(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	staleAfter := domain.EvaluationStaleAfter(domain.Timeframe1h)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{freshSnap("BTCUSDT")},
		at:    now.Add(-staleAfter), // age == StaleAfter → still fresh (>)
	}
	rankings := &stubRankings{}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	if _, err := p.GetLatestEvaluations(context.Background(), "1h"); err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 0 {
		t.Fatalf("age==StaleAfter must be fresh, got %d rankings calls", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_StaleStoreFallsBack(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	staleAfter := domain.EvaluationStaleAfter(domain.Timeframe1h)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{freshSnap("OLD")},
		at:    now.Add(-(staleAfter + time.Second)),
	}
	sym, _ := domain.NewSymbol("ETHUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{
		Symbol: sym,
		Scores: map[string]float64{"Trend Predictability": 0.7},
	}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 {
		t.Fatalf("expected rankings fallback, got %d calls", rankings.callCount())
	}
	if len(got) != 1 || got[0].Symbol != "ETHUSDT" {
		t.Fatalf("expected rankings-derived snapshot, got %#v", got)
	}
}

func TestRankingsEvaluationProvider_EmptyFreshIsMiss(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{},
		at:    now.Add(-time.Second),
	}
	sym, _ := domain.NewSymbol("BTCUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 {
		t.Fatalf("empty array must miss → rankings, got %d calls", rankings.callCount())
	}
	if len(got) != 1 {
		t.Fatalf("expected rankings result, got %d", len(got))
	}
}

func TestRankingsEvaluationProvider_AlgoVersionMismatchFallsBack(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: "old-algo",
		}},
		at: now.Add(-time.Second),
	}
	sym, _ := domain.NewSymbol("ETHUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 || got[0].Symbol != "ETHUSDT" {
		t.Fatalf("algo mismatch must fall back, calls=%d got=%#v", rankings.callCount(), got)
	}
}

func TestRankingsEvaluationProvider_EmptyAlgoVersionFallsBack(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: "",
		}},
		at: now.Add(-time.Second),
	}
	sym, _ := domain.NewSymbol("ETHUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 || got[0].Symbol != "ETHUSDT" {
		t.Fatalf("empty AlgoVersion must fall back, calls=%d got=%#v", rankings.callCount(), got)
	}
}

func TestRankingsEvaluationProvider_MissFallsBack(t *testing.T) {
	store := &stubEvalStore{getErr: ports.ErrEvaluationNotFound}
	sym, _ := domain.NewSymbol("BTCUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	got, err := p.GetLatestEvaluations(context.Background(), "15m")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 {
		t.Fatalf("expected rankings on miss, got %d", rankings.callCount())
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 eval, got %d", len(got))
	}
}

func TestRankingsEvaluationProvider_TransportErrorFallsBack(t *testing.T) {
	boom := errors.New("redis down")
	store := &stubEvalStore{getErr: boom}
	sym, _ := domain.NewSymbol("BTCUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("transport must fail open to rankings: %v", err)
	}
	if rankings.callCount() != 1 {
		t.Fatalf("expected rankings fallback on transport error, got %d calls", rankings.callCount())
	}
	if len(got) != 1 || got[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected rankings-derived snapshot, got %#v", got)
	}
}

func TestRankingsEvaluationProvider_ContextCanceledDoesNotFallback(t *testing.T) {
	store := &stubEvalStore{getErr: context.Canceled}
	rankings := &stubRankings{}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	_, err := p.GetLatestEvaluations(context.Background(), "1h")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if rankings.callCount() != 0 {
		t.Fatalf("cancel must not start rankings, got %d calls", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_DeadlineExceededDoesNotFallback(t *testing.T) {
	store := &stubEvalStore{getErr: context.DeadlineExceeded}
	rankings := &stubRankings{}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	_, err := p.GetLatestEvaluations(context.Background(), "1h")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if rankings.callCount() != 0 {
		t.Fatalf("deadline must not start rankings, got %d calls", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_FutureAtIsStale(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &stubEvalStore{
		evals: []domain.EvaluationSnapshot{freshSnap("BTCUSDT")},
		at:    now.Add(time.Hour), // clock skew / malformed future
	}
	sym, _ := domain.NewSymbol("ETHUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)
	p.SetNow(func() time.Time { return now })

	got, err := p.GetLatestEvaluations(context.Background(), "1h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 || got[0].Symbol != "ETHUSDT" {
		t.Fatalf("future at must fall back, calls=%d got=%#v", rankings.callCount(), got)
	}
}

func TestRankingsEvaluationProvider_NilStoreComputes(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	rankings := &stubRankings{rows: []usecases.RankedResult{{Symbol: sym}}}
	p := NewRankingsEvaluationProvider(rankings)

	got, err := p.GetLatestEvaluations(context.Background(), "4h")
	if err != nil {
		t.Fatalf("GetLatestEvaluations: %v", err)
	}
	if rankings.callCount() != 1 || len(got) != 1 {
		t.Fatalf("expected compute path, calls=%d got=%d", rankings.callCount(), len(got))
	}
}

func TestRankingsEvaluationProvider_SingleflightCoalescesMiss(t *testing.T) {
	store := &stubEvalStore{getErr: ports.ErrEvaluationNotFound}
	sym, _ := domain.NewSymbol("BTCUSDT")
	entered := make(chan struct{})
	block := make(chan struct{})
	rankings := &stubRankings{
		rows:    []usecases.RankedResult{{Symbol: sym}},
		blockCh: block,
		entered: entered,
	}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	const n = 8
	var joined atomic.Int32
	p.fallbackEnter = func() { joined.Add(1) }

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := p.GetLatestEvaluations(context.Background(), "1h")
			errs <- err
		}()
	}
	<-entered
	// Wait until every caller has entered computeFromRankings (and thus
	// joined the in-flight DoChan) before releasing Execute — no fixed sleep.
	deadline := time.Now().Add(2 * time.Second)
	for joined.Load() < int32(n) {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for siblings to join flight: joined=%d want=%d", joined.Load(), n)
		}
		runtime.Gosched()
	}
	close(block)

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("GetLatestEvaluations: %v", err)
		}
	}
	if rankings.callCount() != 1 {
		t.Fatalf("expected singleflight to coalesce to 1 Execute, got %d", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_CancelledCallerReturnsWhileFlightContinues(t *testing.T) {
	store := &stubEvalStore{getErr: ports.ErrEvaluationNotFound}
	sym, _ := domain.NewSymbol("BTCUSDT")
	entered := make(chan struct{})
	block := make(chan struct{})
	finished := make(chan struct{})
	rankings := &stubRankings{
		rows:     []usecases.RankedResult{{Symbol: sym}},
		blockCh:  block,
		entered:  entered,
		finished: finished,
	}
	p := NewRankingsEvaluationProvider(rankings)
	p.SetStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := p.GetLatestEvaluations(ctx, "1h")
		errCh <- err
	}()
	<-entered
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled caller blocked until rankings finished")
	}
	close(block) // allow shared flight to finish under WithoutCancel
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shared flight did not complete after cancel")
	}
	if rankings.callCount() != 1 {
		t.Fatalf("shared flight must run once, callCount=%d", rankings.callCount())
	}
}

func TestRankingsEvaluationProvider_CloneDeepCopiesSparkline(t *testing.T) {
	src := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", Sparkline: []float64{1, 2, 3}, AlgoVersion: domain.AlgoVersion,
	}}
	got := cloneEvaluationSnapshots(src)
	if len(got) != 1 || got[0].Sparkline[0] != 1 {
		t.Fatalf("clone: %#v", got)
	}
	got[0].Sparkline[0] = 99
	if src[0].Sparkline[0] != 1 {
		t.Fatal("Sparkline backing array must not be shared")
	}
}

func TestRankingsEvaluationProvider_ZeroValueSafe(t *testing.T) {
	var p *RankingsEvaluationProvider
	if _, err := p.GetLatestEvaluations(context.Background(), "1h"); err == nil {
		t.Fatal("expected error for nil provider")
	}
	p = &RankingsEvaluationProvider{}
	if _, err := p.GetLatestEvaluations(context.Background(), "1h"); err == nil {
		t.Fatal("expected error for zero-value provider")
	}
}
