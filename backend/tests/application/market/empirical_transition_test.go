package market_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/transition"
	mkt "pano_chart/backend/domain/market"
)

func TestBuildMatrix_CompressionToExpansion(t *testing.T) {
	periods := make([]mkt.RegimePeriod, 0, 80)
	for i := 0; i < 40; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: int64Ptr(int64(i))},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: int64Ptr(int64(i))},
		)
	}
	m := transition.BuildMatrix(periods)
	look, ok := m.Lookup(mkt.RegimeCompression, 10)
	if !ok {
		t.Fatal("lookup failed")
	}
	if look.SampleSize != 40 || look.Pooled {
		t.Fatalf("sample=%d pooled=%v", look.SampleSize, look.Pooled)
	}
	if look.Probabilities.Expansion < 0.9 {
		t.Fatalf("P(expansion|compression)=%f want ≥ 0.9", look.Probabilities.Expansion)
	}
}

func TestBuildMatrix_EmptyBucketDoesNotUseLaplacePrior(t *testing.T) {
	// All transitions at duration 100 (mature vs median 100). Young must not
	// return the uniform Laplace prior as if it were evidence.
	end := int64(1)
	periods := make([]mkt.RegimePeriod, 0, 80)
	for i := 0; i < 40; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 100, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 100, EndTimestamp: &end},
		)
	}
	m := transition.BuildMatrix(periods)

	young, ok := m.Lookup(mkt.RegimeCompression, 10) // < 25% of median 100
	if !ok {
		t.Fatal("expected pooled fallback when young bucket empty")
	}
	if !young.Pooled {
		t.Fatal("young bucket empty must use pooled row, not empty-bucket Laplace")
	}
	if young.Probabilities.Expansion < 0.9 {
		t.Fatalf("pooled P(expansion)=%f want ≥ 0.9 (not 0.25 prior)", young.Probabilities.Expansion)
	}
	if young.SampleSize != 40 {
		t.Fatalf("pooled sampleSize=%d want 40", young.SampleSize)
	}

	mature, ok := m.Lookup(mkt.RegimeCompression, 100)
	if !ok || mature.Pooled || mature.SampleSize != 40 {
		t.Fatalf("mature lookup=%+v ok=%v", mature, ok)
	}
}

func TestBuildMatrix_SilentIndecisiveDoNotSelfLoop(t *testing.T) {
	end := int64(1)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeSilent, DurationCandles: 5, EndTimestamp: &end},
		{Regime: mkt.RegimeIndecisive, DurationCandles: 7, EndTimestamp: &end},
		{Regime: mkt.RegimeTrend, DurationCandles: 10, EndTimestamp: &end},
	}
	m := transition.BuildMatrix(periods)
	look, ok := m.Lookup(mkt.RegimeSideways, 12)
	if !ok {
		t.Fatal("expected sideways→trend after merge")
	}
	if look.Probabilities.Sideways >= look.Probabilities.Trend {
		t.Fatalf("expected trend to dominate after merge, got %+v", look.Probabilities)
	}
}

func TestBuildMatrix_UnknownRegimeBreaksSegment(t *testing.T) {
	end := int64(1)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeTrend, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.Regime("bogus"), DurationCandles: 5, EndTimestamp: &end},
		{Regime: mkt.RegimeTrend, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeSideways, DurationCandles: 10, EndTimestamp: &end},
	}
	m := transition.BuildMatrix(periods)
	look, ok := m.Lookup(mkt.RegimeTrend, 10)
	if !ok {
		t.Fatal("expected trend→sideways only")
	}
	// Must not invent trend→trend across the gap; only trend→sideways.
	if look.SampleSize != 1 {
		t.Fatalf("sampleSize=%d want 1 (no self-loop across gap)", look.SampleSize)
	}
	if look.Probabilities.Trend >= look.Probabilities.Sideways {
		t.Fatalf("fabricated self-loop? probs=%+v", look.Probabilities)
	}
}

func TestBuildMatrix_SparseBucketFallsBackToPooled(t *testing.T) {
	end := int64(1)
	periods := make([]mkt.RegimePeriod, 0, 90)
	// 5 young (dur 10) + 35 mature (dur 100) compression→expansion.
	for i := 0; i < 5; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		)
	}
	for i := 0; i < 35; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 100, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 100, EndTimestamp: &end},
		)
	}
	m := transition.BuildMatrix(periods)
	young, ok := m.Lookup(mkt.RegimeCompression, 10)
	if !ok {
		t.Fatal("lookup failed")
	}
	if !young.Pooled || young.SampleSize != 40 {
		t.Fatalf("sparse young must use pooled n=40, got sample=%d pooled=%v", young.SampleSize, young.Pooled)
	}
	if young.Probabilities.Expansion < 0.9 {
		t.Fatalf("pooled expansion=%f", young.Probabilities.Expansion)
	}
}

func TestBuildMatrix_OpenTailExcludedFromMedian(t *testing.T) {
	end := int64(1)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		// Open current with huge duration must not pull median toward stale/young flip.
		{Regime: mkt.RegimeCompression, DurationCandles: 10_000, EndTimestamp: nil},
	}
	m := transition.BuildMatrix(periods)
	look, ok := m.Lookup(mkt.RegimeCompression, 10)
	if !ok {
		t.Fatal("lookup failed")
	}
	// Closed median = 10 → ratio 1.0 → mature bucket (not young). Including the
	// open 10k in the median would make duration 10 classify as young and force pooled.
	if look.Pooled {
		t.Fatal("open tail must not skew median into a young/pooled path for dur=10")
	}
	if look.SampleSize != 3 {
		t.Fatalf("sampleSize=%d want 3 closed compression→expansion", look.SampleSize)
	}
}

func TestBuildMatrix_EmptyHistory(t *testing.T) {
	m := transition.BuildMatrix(nil)
	if _, ok := m.Lookup(mkt.RegimeCompression, 12); ok {
		t.Fatal("empty matrix must not Lookup")
	}
}

func TestClassifyAge_Boundaries(t *testing.T) {
	med := 100.0
	cases := []struct {
		dur  int
		want transition.AgeBucket
	}{
		{10, transition.AgeYoung},
		{24, transition.AgeYoung},
		{25, transition.AgeMid},
		{74, transition.AgeMid},
		{75, transition.AgeMature},
		{150, transition.AgeMature},
		{151, transition.AgeStale},
	}
	for _, tc := range cases {
		if got := transition.ClassifyAge(tc.dur, med); got != tc.want {
			t.Fatalf("dur=%d got %d want %d", tc.dur, got, tc.want)
		}
	}
}

func TestWeightFromSamples_Cap(t *testing.T) {
	if w := transition.WeightFromSamples(120); math.Abs(w-0.7) > 1e-9 {
		t.Fatalf("weight=%f want 0.7", w)
	}
	if w := transition.WeightFromSamples(40); math.Abs(w-0.4) > 1e-9 {
		t.Fatalf("weight=%f want 0.4", w)
	}
}

func TestBlend_CapsAtProductWeight(t *testing.T) {
	emp := mkt.TransitionProbabilities{Trend: 1}
	heur := mkt.TransitionProbabilities{Sideways: 1}
	got := transition.Blend(emp, heur, 1.0) // request full empirical — capped at 0.7
	if math.Abs(got.Trend-0.7) > 1e-9 || math.Abs(got.Sideways-0.3) > 1e-9 {
		t.Fatalf("blend=%+v", got)
	}
}

type stubHistory struct {
	mu      sync.Mutex
	periods []mkt.RegimePeriod
	err     error
	calls   int
}

func (s *stubHistory) GetHistory(string, int) ([]mkt.RegimePeriod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.periods, s.err
}

func (s *stubHistory) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestMatrixCache_TTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hist := &stubHistory{periods: []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
	}}
	cache := transition.NewMatrixCache(hist, time.Minute, 100)
	cache.SetClock(func() time.Time { return now })

	_ = cache.Matrix("4h")
	_ = cache.Matrix("4h")
	if hist.callCount() != 1 {
		t.Fatalf("expected 1 history call within TTL, got %d", hist.callCount())
	}
	now = now.Add(2 * time.Minute)
	_ = cache.Matrix("4h")
	if hist.callCount() != 2 {
		t.Fatalf("expected rebuild after TTL, calls=%d", hist.callCount())
	}
}

func TestMatrixCache_ErrorNegativeCaches(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hist := &stubHistory{periods: []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
	}}
	cache := transition.NewMatrixCache(hist, time.Minute, 100)
	cache.SetClock(func() time.Time { return now })

	_ = cache.Matrix("4h")
	now = now.Add(2 * time.Minute)
	hist.err = context.DeadlineExceeded
	_ = cache.Matrix("4h") // fail once, bump builtAt
	for i := 0; i < 5; i++ {
		_ = cache.Matrix("4h")
	}
	if hist.callCount() != 2 {
		t.Fatalf("persistent errors must not retry within TTL, calls=%d want 2", hist.callCount())
	}
}

func TestMatrixCache_FirstFailureNegativeCaches(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hist := &stubHistory{err: context.DeadlineExceeded}
	cache := transition.NewMatrixCache(hist, time.Minute, 100)
	cache.SetClock(func() time.Time { return now })
	_ = cache.Matrix("1h")
	_ = cache.Matrix("1h")
	_ = cache.Matrix("1h")
	if hist.callCount() != 1 {
		t.Fatalf("first failure must negative-cache, calls=%d", hist.callCount())
	}
}

func TestMatrixCache_PerTimeframeIsolation(t *testing.T) {
	started4h := make(chan struct{})
	release4h := make(chan struct{})
	hist := &blockingHistory{
		periods: []mkt.RegimePeriod{
			{Regime: mkt.RegimeTrend, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
			{Regime: mkt.RegimeSideways, DurationCandles: 10, EndTimestamp: int64Ptr(1)},
		},
		blockTF:    "4h",
		started:    started4h,
		release:    release4h,
	}
	cache := transition.NewMatrixCache(hist, time.Minute, 100)

	slowDone := make(chan struct{})
	go func() {
		_ = cache.Matrix("4h")
		close(slowDone)
	}()
	<-started4h // 4h rebuild is inside GetHistory

	fastDone := make(chan struct{})
	go func() {
		_ = cache.Matrix("1h")
		close(fastDone)
	}()

	select {
	case <-fastDone:
		// 1h completed while 4h was still blocked — per-TF lock works.
	case <-time.After(2 * time.Second):
		t.Fatal("1h rebuild blocked behind 4h GetHistory")
	}
	close(release4h)
	<-slowDone
}

type blockingHistory struct {
	periods  []mkt.RegimePeriod
	blockTF  string
	started  chan struct{}
	release  chan struct{}
	mu       sync.Mutex
	calls    int
	startedOnce sync.Once
}

func (b *blockingHistory) GetHistory(tf string, _ int) ([]mkt.RegimePeriod, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	if tf == b.blockTF {
		b.startedOnce.Do(func() { close(b.started) })
		<-b.release
	}
	return b.periods, nil
}

func TestTransitionService_EmptyHistoryHeuristic(t *testing.T) {
	provider := &fakeTransitionRegimeProvider{
		summary: mkt.Summary{
			State:               mkt.StateCompression,
			Breadth:             mkt.Breadth{Compression: 0},
			VolatilityExpansion: 1.0,
		},
	}
	svc := transition.NewTransitionService(provider, transition.NewTransitionEngine())
	svc.SetMatrixCache(transition.NewMatrixCache(&stubHistory{}, time.Minute, 50))

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "heuristic" {
		t.Fatalf("source=%q want heuristic", result.Source)
	}
	if result.EmpiricalWeight != 0 || result.SampleSize != 0 {
		t.Fatalf("weight=%f sample=%d", result.EmpiricalWeight, result.SampleSize)
	}
	if math.Abs(result.Probabilities.Compression-0.6) > 1e-9 {
		t.Fatalf("probs=%+v", result.Probabilities)
	}
}

func TestTransitionService_LowSampleStaysHeuristic(t *testing.T) {
	end := int64(1)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
		{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
	}
	provider := &fakeTransitionRegimeProvider{
		summary: mkt.Summary{
			State:               mkt.StateCompression,
			Breadth:             mkt.Breadth{Compression: 0},
			VolatilityExpansion: 1.0,
		},
	}
	svc := transition.NewTransitionService(provider, transition.NewTransitionEngine())
	svc.SetAgeProvider(fixedAge(10))
	svc.SetMatrixCache(transition.NewMatrixCache(&stubHistory{periods: periods}, time.Minute, 50))

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "heuristic" {
		t.Fatalf("n=1 must not blend, source=%q", result.Source)
	}
	if result.SampleSize != 1 {
		t.Fatalf("sampleSize=%d want 1 (reported, not blended)", result.SampleSize)
	}
	if result.EmpiricalWeight != 0 {
		t.Fatalf("weight=%f want 0", result.EmpiricalWeight)
	}
}

func TestTransitionService_SparseYoungBlendsPooled(t *testing.T) {
	end := int64(1)
	periods := make([]mkt.RegimePeriod, 0, 90)
	for i := 0; i < 5; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		)
	}
	for i := 0; i < 35; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 100, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 100, EndTimestamp: &end},
		)
	}
	provider := &fakeTransitionRegimeProvider{
		summary: mkt.Summary{
			State:               mkt.StateCompression,
			Breadth:             mkt.Breadth{Compression: 0},
			VolatilityExpansion: 1.0,
		},
	}
	svc := transition.NewTransitionService(provider, transition.NewTransitionEngine())
	svc.SetAgeProvider(fixedAge(10)) // young vs median ~100
	svc.SetMatrixCache(transition.NewMatrixCache(&stubHistory{periods: periods}, time.Minute, 200))

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "blend" || !result.Pooled || result.SampleSize != 40 {
		t.Fatalf("want blend pooled n=40, got source=%s pooled=%v n=%d",
			result.Source, result.Pooled, result.SampleSize)
	}
	if result.Probabilities.Expansion <= 0.2 {
		t.Fatalf("expected pooled expansion lift, got %+v", result.Probabilities)
	}
}

func TestTransitionService_BlendUsesHistory(t *testing.T) {
	end := int64(1)
	periods := make([]mkt.RegimePeriod, 0, 80)
	for i := 0; i < 40; i++ {
		periods = append(periods,
			mkt.RegimePeriod{Regime: mkt.RegimeCompression, DurationCandles: 10, EndTimestamp: &end},
			mkt.RegimePeriod{Regime: mkt.RegimeExpansion, DurationCandles: 10, EndTimestamp: &end},
		)
	}
	provider := &fakeTransitionRegimeProvider{
		summary: mkt.Summary{
			State:               mkt.StateCompression,
			Breadth:             mkt.Breadth{Compression: 0},
			VolatilityExpansion: 1.0,
		},
	}
	svc := transition.NewTransitionService(provider, transition.NewTransitionEngine())
	svc.SetAgeProvider(fixedAge(10))
	svc.SetMatrixCache(transition.NewMatrixCache(&stubHistory{periods: periods}, time.Minute, 200))

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "blend" {
		t.Fatalf("source=%q", result.Source)
	}
	if result.Pooled {
		t.Fatal("uniform-bucket history must use the age bucket, not pooled")
	}
	if result.SampleSize != 40 {
		t.Fatalf("sampleSize=%d", result.SampleSize)
	}
	wantW := 0.4
	if math.Abs(result.EmpiricalWeight-wantW) > 1e-9 {
		t.Fatalf("weight=%f want %f", result.EmpiricalWeight, wantW)
	}
	if result.Probabilities.Expansion <= 0.2 {
		t.Fatalf("expected empirical lift on expansion, got %+v", result.Probabilities)
	}
}

func TestMarketTransitionHandler_EmpiricalFields(t *testing.T) {
	calc := &fakeTransitionCalculator{
		result: mkt.MarketTransition{
			Timeframe:       "1h",
			CurrentRegime:   mkt.RegimeCompression,
			Probabilities:   mkt.TransitionProbabilities{Trend: 0.4, Expansion: 0.6},
			Horizon:         "12 candles",
			Source:          "blend",
			EmpiricalWeight: 0.4,
			SampleSize:      40,
			Pooled:          true,
		},
	}
	handler := adhttp.NewMarketTransitionHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/transition?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Source          string  `json:"source"`
		EmpiricalWeight float64 `json:"empiricalWeight"`
		SampleSize      int     `json:"sampleSize"`
		Pooled          bool    `json:"pooled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Source != "blend" || resp.SampleSize != 40 || !resp.Pooled || math.Abs(resp.EmpiricalWeight-0.4) > 1e-9 {
		t.Fatalf("dto=%+v", resp)
	}
}

func TestMarketTransitionHandler_InvalidTimeframe(t *testing.T) {
	handler := adhttp.NewMarketTransitionHandler(&fakeTransitionCalculator{})
	req := httptest.NewRequest(http.MethodGet, "/api/market/transition?timeframe=not-a-tf", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func int64Ptr(v int64) *int64 { return &v }
