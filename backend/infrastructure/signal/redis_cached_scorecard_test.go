package signal

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	appsignal "pano_chart/backend/application/signal"
)

type memRedis struct {
	mu   sync.Mutex
	data map[string]string
	ttl  map[string]time.Duration
	gets int
	sets int
}

func (m *memRedis) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	v, ok := m.data[key]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (m *memRedis) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sets++
	if m.data == nil {
		m.data = map[string]string{}
	}
	if m.ttl == nil {
		m.ttl = map[string]time.Duration{}
	}
	m.data[key] = value
	m.ttl[key] = ttl
	return nil
}

type failRedis struct{}

func (failRedis) Get(context.Context, string) (string, error) {
	return "", errors.New("redis down")
}
func (failRedis) Set(context.Context, string, string, time.Duration) error {
	return nil
}

type countingAPI struct {
	mu      sync.Mutex
	gets    int
	sums    int
	card    appsignal.Scorecard
	sum     appsignal.SummaryResult
	block   chan struct{}
	entered chan struct{} // closed once when Get first enters
}

func (c *countingAPI) Get(ctx context.Context, _, _, _, _ string) (appsignal.Scorecard, error) {
	c.mu.Lock()
	c.gets++
	if c.entered != nil {
		select {
		case <-c.entered:
		default:
			close(c.entered)
		}
	}
	c.mu.Unlock()
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
			return appsignal.Scorecard{}, ctx.Err()
		}
	}
	return c.card, nil
}
func (c *countingAPI) Summary(ctx context.Context, _, _ string) (appsignal.SummaryResult, error) {
	c.mu.Lock()
	c.sums++
	c.mu.Unlock()
	return c.sum, nil
}

func waitGets(t *testing.T, next *countingAPI, min int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		next.mu.Lock()
		n := next.gets
		next.mu.Unlock()
		if n >= min {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("flight never reached gets=%d (have %d)", min, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWrapRedisGetErr_logsOnlyWhenRequestContextLive(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	live := context.Background()
	if err := wrapRedisGetErr(live, "dial", context.DeadlineExceeded); err == nil {
		t.Fatal("expected wrap")
	}
	if !bytes.Contains(buf.Bytes(), []byte("dial")) {
		t.Fatalf("dial timeout on a live request must be logged, got %q", buf.String())
	}

	buf.Reset()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wrapRedisGetErr(canceled, "gone", context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("wrap=%v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("canceled request must not log, got %q", buf.String())
	}

	buf.Reset()
	expired, stop := context.WithTimeout(context.Background(), time.Nanosecond)
	defer stop()
	time.Sleep(time.Millisecond)
	if err := wrapRedisGetErr(expired, "late", context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wrap=%v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expired request must not log, got %q", buf.String())
	}
}

func TestRedisCachedScorecard_flightRecheckLogsAgainstRequest(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	release := make(chan struct{})
	redis := &recheckRedis{release: release, second: make(chan struct{})}
	next := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1}}
	cache := NewRedisCachedScorecard(next, redis, "scorecards")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := cache.Get(ctx, "badge", "x", "1h", "30d")
		errCh <- err
	}()
	select {
	case <-redis.second:
	case <-time.After(2 * time.Second):
		t.Fatal("flight re-check never started")
	}
	cancel()
	close(release)
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Get did not return")
	}
	if buf.Len() != 0 {
		t.Fatalf("canceled request must not log the flight re-check, got %q", buf.String())
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 0 {
		t.Fatalf("redis error must not scan, gets=%d", gets)
	}

	buf.Reset()
	redis2 := &recheckRedis{release: make(chan struct{})}
	close(redis2.release)
	cache2 := NewRedisCachedScorecard(next, redis2, "scorecards")
	if _, err := cache2.Get(context.Background(), "badge", "x", "1h", "7d"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("scorecards/get/")) {
		t.Fatalf("live request must log the flight re-check, got %q", buf.String())
	}
}

// recheckRedis misses the first GET, then returns DeadlineExceeded on the flight re-check.
type recheckRedis struct {
	mu      sync.Mutex
	n       int
	second  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *recheckRedis) Get(ctx context.Context, _ string) (string, error) {
	r.mu.Lock()
	r.n++
	n := r.n
	r.mu.Unlock()
	if n == 1 {
		return "", redis.Nil
	}
	r.once.Do(func() {
		if r.second != nil {
			close(r.second)
		}
	})
	select {
	case <-r.release:
		return "", context.DeadlineExceeded
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func (r *recheckRedis) Set(context.Context, string, string, time.Duration) error { return nil }

func TestScorecardCacheKey_noColonCollision(t *testing.T) {
	k1 := scorecardCacheKey("scorecards", "get", "regime", "regime:trend", "1h", "30d")
	k2 := scorecardCacheKey("scorecards", "get", "regime:regime", "trend", "1h", "30d")
	if k1 == k2 {
		t.Fatalf("collided: %s", k1)
	}
}

func TestRedisCachedScorecard_stableSinceBucketHits(t *testing.T) {
	redisMem := &memRedis{}
	next := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "trend_up", Total: 1, Hits: 1, HitRate: 1}}
	cache := NewRedisCachedScorecard(next, redisMem, "scorecards")

	_, err := cache.Get(context.Background(), "Badge", "trend_up", "1H", "30d")
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.Get(context.Background(), "badge", "trend_up", "1h", "30d")
	if err != nil {
		t.Fatal(err)
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 1 {
		t.Fatalf("expected 1 compute for Badge/badge + 1H/1h + since=30d, got %d", gets)
	}
	wantKey := scorecardCacheKey("scorecards", "get", "badge", "trend_up", "1h", "30d")
	if _, ok := redisMem.data[wantKey]; !ok {
		t.Fatalf("missing key %q in %v", wantKey, redisMem.data)
	}
}

func TestRedisCachedScorecard_absoluteSinceDistinctKeys(t *testing.T) {
	redisMem := &memRedis{}
	next := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1}}
	cache := NewRedisCachedScorecard(next, redisMem, "scorecards")

	if _, err := cache.Get(context.Background(), "badge", "x", "1h", "2026-09-21T00:01:00Z"); err != nil {
		t.Fatal(err)
	}
	next.card = appsignal.Scorecard{Kind: "badge", Label: "x", Total: 0}
	if _, err := cache.Get(context.Background(), "badge", "x", "1h", "2026-09-21T00:09:00Z"); err != nil {
		t.Fatal(err)
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 2 {
		t.Fatalf("absolute windows must not share flight/key, gets=%d", gets)
	}
	if len(redisMem.data) != 2 {
		t.Fatalf("keys=%v", redisMem.data)
	}
}

func TestRedisCachedScorecard_callerCancelSurfaces(t *testing.T) {
	block := make(chan struct{})
	next := &countingAPI{
		card:  appsignal.Scorecard{Kind: "badge", Label: "x", Total: 3},
		block: block,
	}
	cache := NewRedisCachedScorecard(next, nil, "scorecards")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := cache.Get(ctx, "badge", "x", "1h", "30d")
		errCh <- err
	}()
	waitGets(t, next, 1)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel")
	}
	close(block)
}

func TestRedisCachedScorecard_siblingSurvivesLeaderCancel(t *testing.T) {
	block := make(chan struct{})
	next := &countingAPI{
		card:  appsignal.Scorecard{Kind: "badge", Label: "x", Total: 7, Hits: 4},
		block: block,
	}
	joined := make(chan struct{}, 2)
	cache := NewRedisCachedScorecard(next, nil, "scorecards")
	cache.onFlightJoin = func() { joined <- struct{}{} }

	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := cache.Get(leaderCtx, "badge", "x", "1h", "30d")
		leaderErr <- err
	}()
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatal("leader never joined flight")
	}
	waitGets(t, next, 1)

	waiterDone := make(chan struct{})
	var waiterCard appsignal.Scorecard
	var waiterErr error
	go func() {
		defer close(waiterDone)
		waiterCard, waiterErr = cache.Get(context.Background(), "badge", "x", "1h", "30d")
	}()
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never joined flight")
	}

	leaderCancel()
	select {
	case err := <-leaderErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not return on cancel")
	}
	select {
	case <-waiterDone:
		t.Fatal("waiter must still be in the flight after the leader returns")
	default:
	}

	close(block)
	select {
	case <-waiterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not return")
	}
	if waiterErr != nil {
		t.Fatalf("waiter err=%v (must not inherit leader cancel)", waiterErr)
	}
	if waiterCard.Total != 7 {
		t.Fatalf("waiter card=%+v", waiterCard)
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 1 {
		t.Fatalf("shared flight must compute once, gets=%d", gets)
	}
}

func TestRedisCachedScorecard_abandonReleasesComputeSlot(t *testing.T) {
	block := make(chan struct{})
	next := &countingAPI{
		card:    appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1},
		block:   block,
		entered: make(chan struct{}),
	}
	cache := NewRedisCachedScorecard(next, nil, "scorecards")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := cache.Get(ctx, "badge", "x", "1h", "30d")
		errCh <- err
	}()
	select {
	case <-next.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("compute never entered")
	}
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not return")
	}

	// Abandoned flight must release the semaphore so another key can compute.
	next2 := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "y", Total: 2}}
	cache.next = next2
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := cache.Get(context.Background(), "badge", "y", "1h", "7d")
		if err != nil {
			t.Errorf("second key: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("abandoned flight held compute slot")
	}
	close(block)
}

func TestRedisCachedScorecard_redisErrorDoesNotScanSQLite(t *testing.T) {
	next := &countingAPI{
		card: appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1},
		sum:  appsignal.SummaryResult{Timeframe: "1h"},
	}
	cache := NewRedisCachedScorecard(next, failRedis{}, "scorecards")
	if _, err := cache.Get(context.Background(), "badge", "x", "1h", "30d"); err == nil {
		t.Fatal("expected redis error on Get")
	}
	if _, err := cache.Summary(context.Background(), "1h", "30d"); err == nil {
		t.Fatal("expected redis error on Summary")
	}
	next.mu.Lock()
	gets, sums := next.gets, next.sums
	next.mu.Unlock()
	if gets != 0 || sums != 0 {
		t.Fatalf("redis failure must not fall through to SQLite, gets=%d sums=%d", gets, sums)
	}
}

func TestRedisCachedScorecard_getCancelPreservesContext(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	redis := &blockOnGetRedis{started: started, once: &once}
	next := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1}}
	cache := NewRedisCachedScorecard(next, redis, "scorecards")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := cache.Get(ctx, "badge", "x", "1h", "30d")
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("redis Get never blocked")
	}
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
		if !errors.Is(err, errScorecardCache) {
			t.Fatalf("want errScorecardCache, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Get did not return after cancel")
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 0 {
		t.Fatalf("canceled redis Get must not scan, gets=%d", gets)
	}
}

type blockOnGetRedis struct {
	started chan struct{}
	once    *sync.Once
}

func (b *blockOnGetRedis) Get(ctx context.Context, _ string) (string, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return "", ctx.Err()
}
func (b *blockOnGetRedis) Set(context.Context, string, string, time.Duration) error {
	return nil
}

func TestRedisCachedScorecard_missIsNilNotHardError(t *testing.T) {
	redisMem := &memRedis{}
	next := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "x", Total: 1}}
	cache := NewRedisCachedScorecard(next, redisMem, "scorecards")
	_, err := cache.Get(context.Background(), "badge", "x", "1h", "30d")
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.Get(context.Background(), "badge", "x", "1h", "30d")
	if err != nil {
		t.Fatal(err)
	}
	next.mu.Lock()
	gets := next.gets
	next.mu.Unlock()
	if gets != 1 {
		t.Fatalf("cache miss handling broken, gets=%d", gets)
	}
}

func TestRedisCachedScorecard_panicDoesNotStealOtherPermit(t *testing.T) {
	aGo := make(chan struct{})
	bGo := make(chan struct{})
	aEntered := make(chan struct{})
	bEntered := make(chan struct{})
	cEntered := make(chan struct{})
	var onceA, onceB, onceC sync.Once
	api := &labelGateAPI{
		get: func(label string) (appsignal.Scorecard, error) {
			switch label {
			case "a":
				onceA.Do(func() { close(aEntered) })
				<-aGo
				panic("boom")
			case "b":
				onceB.Do(func() { close(bEntered) })
				<-bGo
				return appsignal.Scorecard{Kind: "badge", Label: "b", Total: 2}, nil
			default:
				onceC.Do(func() { close(cEntered) })
				return appsignal.Scorecard{Kind: "badge", Label: "c", Total: 3}, nil
			}
		},
	}
	cache := NewRedisCachedScorecard(api, nil, "scorecards")
	waited := make(chan struct{}, 4)
	cache.onComputeWait = func() { waited <- struct{}{} }

	aErr := make(chan error, 1)
	go func() {
		_, err := cache.Get(context.Background(), "badge", "a", "1h", "30d")
		aErr <- err
	}()
	select {
	case <-aEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("A never entered")
	}

	go func() {
		_, _ = cache.Get(context.Background(), "badge", "b", "1h", "7d")
	}()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("B never blocked on the semaphore")
	}

	close(aGo)
	select {
	case <-aErr:
	case <-time.After(2 * time.Second):
		t.Fatal("A did not finish panicking")
	}
	select {
	case <-bEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("B did not take the slot after A panicked")
	}

	go func() {
		_, _ = cache.Get(context.Background(), "badge", "c", "1h", "1d")
	}()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("C never blocked on the semaphore")
	}
	select {
	case <-cEntered:
		t.Fatal("C entered while B still holds the compute slot")
	case <-time.After(50 * time.Millisecond):
	}
	close(bGo)
	select {
	case <-cEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("C did not run after B released the slot")
	}
}

type labelGateAPI struct {
	get func(label string) (appsignal.Scorecard, error)
}

func (a *labelGateAPI) Get(_ context.Context, _, label, _, _ string) (appsignal.Scorecard, error) {
	return a.get(label)
}
func (a *labelGateAPI) Summary(context.Context, string, string) (appsignal.SummaryResult, error) {
	return appsignal.SummaryResult{}, nil
}

func TestRedisCachedScorecard_panicBecomesError(t *testing.T) {
	next := &panicAPI{}
	cache := NewRedisCachedScorecard(next, nil, "scorecards")
	_, err := cache.Get(context.Background(), "badge", "x", "1h", "30d")
	if err == nil {
		t.Fatal("expected panic turned into error")
	}

	// Compute slot must be free for a subsequent key.
	okAPI := &countingAPI{card: appsignal.Scorecard{Kind: "badge", Label: "y", Total: 2}}
	cache.next = okAPI
	card, err := cache.Get(context.Background(), "badge", "y", "1h", "7d")
	if err != nil {
		t.Fatal(err)
	}
	if card.Total != 2 {
		t.Fatalf("%+v", card)
	}
}

type panicAPI struct{}

func (panicAPI) Get(context.Context, string, string, string, string) (appsignal.Scorecard, error) {
	panic("boom")
}
func (panicAPI) Summary(context.Context, string, string) (appsignal.SummaryResult, error) {
	panic("boom")
}
