package evaluation_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appeval "pano_chart/backend/application/evaluation"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type fakeRankings struct {
	mu        sync.Mutex
	calls     []string
	byTF      map[string][]usecases.RankedResult
	err       error
	delay     time.Duration
	blockCh   chan struct{} // if set, wait until closed (or ctx done)
	enteredCh chan struct{} // closed once when Execute begins (before block/delay)
	entered   sync.Once
}

func (f *fakeRankings) Execute(ctx context.Context, req usecases.GetRankingsRequest) (usecases.RankingsResult, error) {
	if f.enteredCh != nil {
		f.entered.Do(func() { close(f.enteredCh) })
	}
	if f.blockCh != nil {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return usecases.RankingsResult{}, ctx.Err()
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return usecases.RankingsResult{}, ctx.Err()
		}
	}
	tf := req.Timeframe.String()
	f.mu.Lock()
	f.calls = append(f.calls, tf)
	f.mu.Unlock()
	if f.err != nil {
		return usecases.RankingsResult{}, f.err
	}
	rows := f.byTF[tf]
	return usecases.RankingsResult{Results: rows, Sort: req.Sort}, nil
}

func (f *fakeRankings) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeStore struct {
	mu     sync.Mutex
	puts   []putCall
	byTF   map[string]storeEntry
	err    error
	getErr error
}

type putCall struct {
	tf string
	n  int
	at time.Time
}

type storeEntry struct {
	evals []domain.EvaluationSnapshot
	at    time.Time
}

func (f *fakeStore) Put(_ context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.puts = append(f.puts, putCall{tf: tf, n: len(evals), at: computedAt})
	if f.byTF == nil {
		f.byTF = make(map[string]storeEntry)
	}
	cp := append([]domain.EvaluationSnapshot(nil), evals...)
	f.byTF[tf] = storeEntry{evals: cp, at: computedAt}
	return nil
}

func (f *fakeStore) Get(_ context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, time.Time{}, f.getErr
	}
	e, ok := f.byTF[tf]
	if !ok {
		return nil, time.Time{}, ports.ErrEvaluationNotFound
	}
	return e.evals, e.at, nil
}

func (f *fakeStore) GetSymbol(context.Context, string, string) (domain.EvaluationSnapshot, time.Time, error) {
	return domain.EvaluationSnapshot{}, time.Time{}, nil
}

func (f *fakeStore) putLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

// recordingLock is a real in-memory NX lock with optional short TTL expiry.
type recordingLock struct {
	mu      sync.Mutex
	holders map[string]string
	expiry  map[string]time.Time
	now     func() time.Time
	acquire int
	release int
}

func newRecordingLock() *recordingLock {
	return &recordingLock{
		holders: make(map[string]string),
		expiry:  make(map[string]time.Time),
		now:     time.Now,
	}
}

func (l *recordingLock) TryAcquire(_ context.Context, key string, ttl time.Duration, holder string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.acquire++
	now := l.now()
	if exp, ok := l.expiry[key]; ok && now.Before(exp) {
		return false, nil
	}
	l.holders[key] = holder
	l.expiry[key] = now.Add(ttl)
	return true, nil
}

func (l *recordingLock) Release(ctx context.Context, key, holder string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("release with cancelled ctx: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.release++
	if l.holders[key] == holder {
		delete(l.holders, key)
		delete(l.expiry, key)
	}
	return nil
}

func (l *recordingLock) held(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.expiry[key]
	return ok && l.now().Before(exp)
}

type gateLock struct {
	ok  bool
	err error
}

func (f *gateLock) TryAcquire(context.Context, string, time.Duration, string) (bool, error) {
	return f.ok, f.err
}

func (f *gateLock) Release(context.Context, string, string) error { return nil }

func sampleRanked(symbol string) usecases.RankedResult {
	return usecases.RankedResult{
		Symbol: domain.NewSymbolUnsafe(symbol),
		Scores: map[string]float64{
			"Trend Predictability": 0.7,
			"Sideways Consistency": 0.2,
			"Compression":          0.1,
			"Breakout Up":          0.05,
			"Breakout Down":        0.02,
		},
		Volume:    1e6,
		Sparkline: []float64{100, 101, 102, 103},
	}
}

func TestRefresher_TickOneTimeframePerCall(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
		"1h":  {sampleRanked("BTCUSDT"), sampleRanked("ETHUSDT")},
		"4h":  {sampleRanked("BTCUSDT")},
		"1d":  {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{}
	r := appeval.NewRefresher(rank, store, appeval.DefaultTimeframes)

	fixed := time.Unix(1_700_000_000, 0).UTC()
	r.SetNow(func() time.Time { return fixed })

	r.Tick(context.Background())
	if store.putLen() != 1 || store.puts[0].tf != "15m" {
		t.Fatalf("first tick: %+v", store.puts)
	}
	r.Tick(context.Background())
	if store.putLen() != 2 || store.puts[1].tf != "1h" || store.puts[1].n != 2 {
		t.Fatalf("second tick: %+v", store.puts)
	}
	r.Tick(context.Background())
	r.Tick(context.Background())
	if store.putLen() != 4 {
		t.Fatalf("after four ticks: %d", store.putLen())
	}
	r.Tick(context.Background())
	if store.putLen() != 4 {
		t.Fatalf("fifth tick should be no-op, puts=%d", store.putLen())
	}
}

func TestRefresher_LeaderRefreshesAgainAfterRelease(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{}
	lock := newRecordingLock()
	r := appeval.NewRefresher(rank, store, []string{"15m"})
	r.SetLock(lock, "leader")

	t0 := time.Unix(1_700_000_000, 0).UTC()
	now := t0
	lock.now = func() time.Time { return now }
	r.SetNow(func() time.Time { return now })

	r.Tick(context.Background())
	if store.putLen() != 1 {
		t.Fatalf("first put: %d", store.putLen())
	}
	if lock.held("eval:refresh:15m") {
		t.Fatal("lock must be released after successful Put")
	}
	if lock.release < 1 {
		t.Fatal("expected Release call")
	}

	// Advance exactly one refresh interval — lock key is gone, leader must Put again.
	now = t0.Add(appeval.RefreshInterval("15m"))
	r.Tick(context.Background())
	if store.putLen() != 2 {
		t.Fatalf("leader must refresh on schedule after release, puts=%d", store.putLen())
	}
}

func TestRefresher_LockHeldPreventsSecondInstance(t *testing.T) {
	block := make(chan struct{})
	entered := make(chan struct{})
	rank := &fakeRankings{
		byTF:      map[string][]usecases.RankedResult{"15m": {sampleRanked("BTCUSDT")}},
		blockCh:   block,
		enteredCh: entered,
	}
	store := &fakeStore{}
	lock := newRecordingLock()

	t0 := time.Unix(1_700_000_000, 0).UTC()
	now := t0
	lock.now = func() time.Time { return now }

	r1 := appeval.NewRefresher(rank, store, []string{"15m"})
	r1.SetLock(lock, "a")
	r1.SetNow(func() time.Time { return now })

	done := make(chan struct{})
	go func() {
		r1.Tick(context.Background())
		close(done)
	}()
	<-entered // r1 holds lock, blocked in Execute

	r2 := appeval.NewRefresher(rank, store, []string{"15m"})
	r2.SetLock(lock, "b")
	r2.SetNow(func() time.Time { return now })
	r2.Tick(context.Background())

	if store.putLen() != 0 {
		t.Fatal("no Put while first Execute in flight")
	}

	close(block)
	<-done
	if store.putLen() != 1 {
		t.Fatalf("r1 should Put once, got %d", store.putLen())
	}
	if rank.callCount() != 1 {
		t.Fatalf("only r1 may Execute, got %d calls", rank.callCount())
	}
}

func TestRefresher_DefaultLockTTLFloor(t *testing.T) {
	got := appeval.DefaultLockTTL("15m")
	if got != 30*time.Minute {
		t.Fatalf("DefaultLockTTL(15m): want 30m floor, got %v", got)
	}
	// 1d interval is 6h → TTL is interval+padding, above the floor.
	want1d := appeval.RefreshInterval("1d") + 30*time.Second
	if got := appeval.DefaultLockTTL("1d"); got != want1d {
		t.Fatalf("DefaultLockTTL(1d): want %v, got %v", want1d, got)
	}
}

func TestRefreshEnabledFromEnv(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", true},
		{"1", true},
		{"true", true},
		{"0", false},
		{"false", false},
		{"off", false},
		{"no", false},
		{" OFF ", false},
	}
	for _, tc := range cases {
		if got := appeval.RefreshEnabledFromEnv(tc.v); got != tc.want {
			t.Errorf("RefreshEnabledFromEnv(%q)=%v want %v", tc.v, got, tc.want)
		}
	}
}

func TestRefresher_StoreFreshSkipsRescore(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{}
	lock := newRecordingLock()

	t0 := time.Unix(1_700_000_000, 0).UTC()
	now := t0
	lock.now = func() time.Time { return now }

	leader := appeval.NewRefresher(rank, store, []string{"15m"})
	leader.SetLock(lock, "leader")
	leader.SetNow(func() time.Time { return now })
	leader.Tick(context.Background())
	if store.putLen() != 1 || rank.callCount() != 1 {
		t.Fatalf("leader: puts=%d calls=%d", store.putLen(), rank.callCount())
	}

	// Peer with empty lastPut acquires lock moments later; store is still fresh.
	peer := appeval.NewRefresher(rank, store, []string{"15m"})
	peer.SetLock(lock, "peer")
	peer.SetNow(func() time.Time { return now.Add(time.Second) })
	peer.Tick(context.Background())

	if rank.callCount() != 1 {
		t.Fatalf("peer must not re-score while store fresh, calls=%d", rank.callCount())
	}
	if store.putLen() != 1 {
		t.Fatalf("peer must not Put again, puts=%d", store.putLen())
	}
}

func TestRefresher_EmptyRankingsDoesNotWipeStore(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{}
	r := appeval.NewRefresher(rank, store, []string{"15m"})

	t0 := time.Unix(1_700_000_000, 0).UTC()
	now := t0
	r.SetNow(func() time.Time { return now })
	r.Tick(context.Background())
	if store.putLen() != 1 {
		t.Fatal("seed put required")
	}

	// Interval elapsed; rankings now empty (transient failure).
	now = t0.Add(appeval.RefreshInterval("15m"))
	rank.byTF["15m"] = nil
	r.Tick(context.Background())

	if store.putLen() != 1 {
		t.Fatalf("empty rankings must not Put, puts=%d", store.putLen())
	}
	got, _, err := store.Get(context.Background(), "15m")
	if err != nil || len(got) != 1 {
		t.Fatalf("last good evals must remain: n=%d err=%v", len(got), err)
	}
	if r.PutCounts()["15m"] != 1 {
		t.Fatal("empty result must not count as successful Put")
	}
}

func TestRefresher_LastPutUsesCompletionTime(t *testing.T) {
	rank := &fakeRankings{
		byTF:  map[string][]usecases.RankedResult{"15m": {sampleRanked("BTCUSDT")}},
		delay: 5 * time.Millisecond,
	}
	store := &fakeStore{}
	r := appeval.NewRefresher(rank, store, []string{"15m"})

	t0 := time.Unix(1_700_000_000, 0).UTC()
	var calls int
	r.SetNow(func() time.Time {
		calls++
		// First call: Tick tryClaim; later calls during/after refresh advance.
		return t0.Add(time.Duration(calls) * time.Millisecond)
	})
	r.Tick(context.Background())
	if store.putLen() != 1 {
		t.Fatal("expected put")
	}
	putAt := store.puts[0].at
	if !putAt.After(t0) {
		t.Errorf("Put computedAt should be completion wall time > tick start, got %v", putAt)
	}

	// Immediately after completion, TF must not be due even if tick-start+interval
	// would have passed relative to an early lastPut stamp.
	r.SetNow(func() time.Time { return putAt.Add(appeval.RefreshInterval("15m") - time.Second) })
	r.Tick(context.Background())
	if store.putLen() != 1 {
		t.Fatalf("must respect completion-based lastPut, puts=%d", store.putLen())
	}
}

func TestRefresher_FailedPutDoesNotAdvanceLastPut(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{err: errors.New("put failed")}
	r := appeval.NewRefresher(rank, store, []string{"15m"})

	t0 := time.Unix(1_700_000_000, 0).UTC()
	now := t0
	r.SetNow(func() time.Time { return now })

	r.Tick(context.Background())
	if r.PutCounts()["15m"] != 0 {
		t.Fatal("failed Put must not increment putCount")
	}
	now = t0.Add(time.Second)
	before := rank.callCount()
	r.Tick(context.Background())
	if rank.callCount() != before {
		t.Fatal("backoff should block retry")
	}
	now = t0.Add(15 * time.Second)
	r.Tick(context.Background())
	if rank.callCount() != before+1 {
		t.Fatalf("after backoff want +1 call, got %d", rank.callCount()-before)
	}
}

func TestRefresher_LockSkipDoesNotAdvanceLastPut(t *testing.T) {
	rank := &fakeRankings{byTF: map[string][]usecases.RankedResult{
		"15m": {sampleRanked("BTCUSDT")},
	}}
	store := &fakeStore{}
	r := appeval.NewRefresher(rank, store, []string{"15m"})
	r.SetLock(&gateLock{ok: false}, "a")

	t0 := time.Unix(1_700_000_000, 0).UTC()
	r.SetNow(func() time.Time { return t0 })
	r.Tick(context.Background())
	if store.putLen() != 0 || r.PutCounts()["15m"] != 0 {
		t.Fatal("lock skip must not Put or advance lastPut")
	}
	r.SetLock(&gateLock{ok: true}, "a")
	r.Tick(context.Background())
	if store.putLen() != 1 {
		t.Fatalf("want Put after lock acquired, got %d", store.putLen())
	}
}

func TestRefresher_ConcurrentTickSameTF(t *testing.T) {
	rank := &fakeRankings{
		byTF:  map[string][]usecases.RankedResult{"15m": {sampleRanked("BTCUSDT")}},
		delay: 50 * time.Millisecond,
	}
	store := &fakeStore{}
	r := appeval.NewRefresher(rank, store, []string{"15m"})
	r.SetNow(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Tick(context.Background())
		}()
	}
	wg.Wait()
	if store.putLen() != 1 || rank.callCount() != 1 {
		t.Fatalf("concurrent Ticks must run once: puts=%d calls=%d", store.putLen(), rank.callCount())
	}
}

func TestRefresher_ContextCancelDuringExecute(t *testing.T) {
	block := make(chan struct{})
	entered := make(chan struct{})
	rank := &fakeRankings{
		byTF:      map[string][]usecases.RankedResult{"15m": {sampleRanked("BTCUSDT")}},
		blockCh:   block,
		enteredCh: entered,
	}
	store := &fakeStore{}
	lock := newRecordingLock()
	r := appeval.NewRefresher(rank, store, []string{"15m"})
	r.SetLock(lock, "a")
	r.SetNow(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Tick(ctx)
		close(done)
	}()

	<-entered // Execute has started and is blocked
	cancel()
	close(block)
	<-done

	if store.putLen() != 0 {
		t.Fatalf("cancel during Execute must not Put, got %d", store.putLen())
	}
	if lock.release < 1 {
		t.Fatal("Release must be attempted after cancel (with uncancellable ctx)")
	}
	if lock.held("eval:refresh:15m") {
		t.Fatal("lock must be released after cancel/failure path — Release must not use cancelled ctx")
	}
}

func TestRefreshInterval(t *testing.T) {
	cases := []struct {
		tf   string
		want time.Duration
	}{
		{"15m", 15 * time.Minute / 4},
		{"1h", 15 * time.Minute},
		{"4h", time.Hour},
		{"1d", 6 * time.Hour},
		{"1m", 30 * time.Second},
	}
	for _, tc := range cases {
		if got := appeval.RefreshInterval(tc.tf); got != tc.want {
			t.Errorf("%s: want %v, got %v", tc.tf, tc.want, got)
		}
	}
}

func TestSnapshotsFromRankings_EnrichesAndDedupes(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	results := []usecases.RankedResult{
		sampleRanked("BTCUSDT"),
		{
			Symbol:    domain.NewSymbolUnsafe("BTCUSDT"),
			Scores:    map[string]float64{"Trend Predictability": 0.99, "Sideways Consistency": 0.1},
			Sparkline: []float64{1, 2, 3},
		},
	}
	snaps := appeval.SnapshotsFromRankings(results, "1h", at)
	if len(snaps) != 1 || snaps[0].TrendScore != 0.99 {
		t.Fatalf("dedupe: %+v", snaps)
	}
}
