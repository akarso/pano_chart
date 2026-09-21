# ROADMAP — v3 "Killer App" Round

This document is the delivery plan for the next big round of work. It is written so that a
contributor (human or LLM) with **no prior context** can pick up any slice and implement it
end-to-end. Read it together with `AGENTS.md`, `backend/BACKEND.md`, `frontend/FRONTEND.md`
and `COMMON.md`.

PR numbering continues from PR-084. Every slice below gets its own spec file when picked up
(`backend/docs/v2/PR-0xx.md` or `frontend/docs/v2/PR-0xx.md`), following the existing format
(🎯 Objective / 🧱 Scope / numbered sections / 🚀 Result / 📝 Implementation notes).

---

## Hotfixes (land immediately; independent of tracks)

Ship these as soon as they are ready. They do not depend on Track A–F and should not wait
on the broader round.

| PR | Title | Why |
|---|---|---|
| **PR-114** | fix(events): FinanceFlow error backoff | Without this, a mid-day upstream blip after cache TTL expiry becomes a **1/min retry storm (~1440 calls/day)** for the rest of the day. Healthy days stay ~50. Spec: `backend/docs/v2/PR-114.md`. |

### PR-114 — Events cache: error backoff (FinanceFlow retry storm)

**Layer:** application. **Depends on:** nothing. **Priority:** ship before / alongside Track A.

**Objective.** Stop the notification scheduler (`MacroCheckInterval = 1m`) from billing
FinanceFlow on every tick when the upstream is failing after a normal cache TTL miss.

**Context.**
- `application/usecases/get_events.go` — in-memory cache, upcoming TTL 30m, past TTL 6h.
- `application/notifications/scheduler.go` — `checkMacroEvents` every 1 minute →
  `eventsAdapter` → `GetEvents.Execute` (US, short lead window).
- Cache key is date-granular (`country|from|to`), so a warm cache yields ~48 scheduler
  upstream calls/day. Observed production: some days ~50, some days ~1440.

**Bug.** On provider error, code served stale data but did **not** refresh freshness.
Next minute: miss → fail → stale → miss… for the rest of the day.

**Spec.**
1. Add `errorHoldUntil` on the cache entry and `errorBackoff` default **15m** on `GetEvents`.
2. `getCached`: if `now < errorHoldUntil`, return the entry (soft hit).
3. On `FetchEvents` error with stale present: set `errorHoldUntil = now + errorBackoff`,
   return stale.
4. On error with no cache: `putCache` empty list with the same hold (avoid empty 1/min storm).
5. On success: `putCache` clears any hold.

**Tests.** See `get_events_backoff_test.go` (expired TTL + fail backs off; cold fail backs off empty).

**Definition of Done.** Spec tests pass; bad-day ceiling ≈ `24×60/15 ≈ 96` scheduler calls
during a full-day outage, not 1440.

---

## 0. How to implement a slice (read this first)

1. Read the slice's **Context** section and open every file it lists. Do not start coding
   until you can explain in one sentence what each listed function does today.
2. Write the tests listed under **Tests** first. Run them; they must fail.
3. Implement the **Spec** exactly. If the spec is silent on something, choose the simplest
   option and write it down in the PR doc's 📝 section. Do not widen scope.
4. Run `cd backend && gofmt -l . && go vet ./... && go test ./...`.
   Run `cd frontend && flutter analyze && flutter test`.
5. If any JSON field changes or is added, update `COMMON.md` in the same PR (additive only;
   never rename or remove a field without a coordinated release note).
6. Write the PR doc. Fill in **Definition of Done** as a checklist and tick it.
7. One slice = one PR. Do not merge two slices into one PR even if they touch the same file.

### Architecture reminders

- `backend/domain` — pure logic, no IO, no config loading, no logging.
- `backend/application` — use cases; depends on domain + ports. Optional dependencies are
  injected with `SetXxx(...)` setters and `nil` means "feature off", never an error.
- `backend/adapters/http` — handlers + DTOs. Rounding happens here (`roundTo`).
- `backend/infrastructure` — Redis/SQLite/HTTP clients, decorators (`RedisCachedXxx`).
- `backend/cmd/api/main.go` — composition root. All wiring lives here.
- Frontend features live in `frontend/lib/features/<feature>/`, each with `*_data.dart`
  (models), `http_*_api.dart` (client), and screens/widgets. Tests mirror in `frontend/test/`.

### Existing building blocks you will reuse constantly

| Thing | Where | What it gives you |
|---|---|---|
| Candle access | `application/ports/candlerepository.go` (`GetLastNCandles`, `GetSeries`) | OHLCV, oldest→newest, completed candles only |
| Symbol universe | `cachedUniverse` in `main.go` (`SymbolUniverseProvider`) | ~150 Binance symbols |
| Per-symbol scoring | `domain/scoring/*_score_calculator.go` (`SymbolScoreCalculator.Score(series)`) | Trend Predictability, Sideways V5, Compression, Breakout, Gain/Loss |
| Rankings pipeline | `application/usecases/get_rankings.go` (`GetRankings.Execute`, `RankedResult`, `assignBadges`, `ScoreKeyForSort`) | Scores + sparkline (110 bars) per symbol |
| Evaluation snapshots | `domain/evaluation_snapshot.go`, `infrastructure/market/rankings_evaluation_provider.go` | Rankings → `EvaluationSnapshot` (scores, bias, ATR proxy, price, volume) |
| Market state | `application/market/market_state_service.go`, `tape_regime.go`, `health.go`, `state_classifier.go` | Headline regime (tape), participation breadth |
| Composite index | `application/market/metrics/composite_index_service.go` (`Calculate`, `CalculateTape`) | Median + volume-weighted index; synthetic OHLCV series |
| Regime history | `application/market/regimehistory/` (`Repository`, `Tracker`, SQLite) | Past regime periods per timeframe |
| Transition model | `application/market/transition/` (`TransitionEngine`, `pressure_model.go`) | Heuristic transition probabilities |
| Setups | `application/setups/service.go` (`SetupService.Evaluate`), `domain/setup/setup_score.go` | Per-symbol setup + confidence |
| Notifications | `application/notifications/scheduler.go` (`checkMarketState`, `checkSetupOfDay`), `infrastructure/social/fcm_notifier.go` | Push pipeline, per-user config |
| Redis cache decorators | `infrastructure/market/redis_cached_composite.go`, `infrastructure/rankings/redis_cached_rankings.go` | Copy this pattern for any new cached service |
| SQLite repos | `application/market/regimehistory/sqlite_repository.go`, `infrastructure/notifications/sqlite_config_store.go` | Copy this pattern for any new table |
| Score telemetry | `infrastructure/scoring/logging_score_calculator.go` | Sampled score logging decorator |
| Market Pulse UI | `frontend/lib/features/market_state/market_pulse_screen.dart` | Cards, `_showInfoDialog`, chart painter |
| Rankings UI | `frontend/lib/features/overview/` (`overview_view_model.dart`, `overview_widget.dart`) | Grid tiles, sort modes, badges |

---

## 1. Glossary (authoritative vocabulary)

Every number shown to a user must map to exactly one term below. PR-085 enforces this.

| Term | Definition | Where it comes from |
|---|---|---|
| **Tape** | The merged market series (composite OHLCV) | `CompositeIndexService.CalculateTape` |
| **Regime** | Dominant structure of the tape: trend / sideways / compression / expansion / indecisive / silent | `ScoreMarketTape` |
| **Tape confidence** | Share of the dominant structure in the tape's score mix (0–1) | `TapeRegime.Confidence` |
| **Structure** | The tape's four-way score mix | `TapeRegime.Structure` |
| **Participation** | Average per-token score mix across the universe (0–1 each) | `MarketStateService` participation breadth |
| **Bias** | Direction of the tape's trend: up / down / neutral | `TapeRegime.Bias` |
| **Composite (median)** | Equal-weight median rebased index | `CompositeIndex.Points` |
| **Composite (volume-weighted)** | Quote-volume-weighted mean rebased index | `CompositeIndex.VolumeWeightedPoints` |
| **Relative strength (RS)** | Symbol return minus composite return over the same window | Track C |
| **Signal** | Any user-facing call the app makes at a point in time (badge, setup, regime, transition) | Track B |
| **Outcome** | What happened after a signal over a fixed horizon | Track B |
| **Hit rate** | Fraction of signals whose outcome met the success rule | Track B |

Deprecated words: *prevalence*, *breadth* (in UI copy), *scores* (as a UI label). Backend JSON
keeps old field names for compatibility; UI must use glossary terms.

---

## 2. Track overview

| Track | Theme | Slices | Depends on |
|---|---|---|---|
| A | Foundations & hygiene | PR-085 … PR-089 | — |
| B | Outcome tracking & calibration | PR-090 … PR-094 | A (085, 086) |
| C | Composite v2 & relative strength | PR-095 … PR-098 | A (086) |
| D | Multi-timeframe & alerts | PR-099 … PR-102 | A (089), C (096) |
| E | Algorithm upgrades | PR-103 … PR-109 | A (088), B (090–094) |
| F | Trade execution UX | PR-110 … PR-112 | A (089), E (104) |

Recommended order: A → B → C → D → E → F. Tracks C and D can run in parallel with B once
Track A lands. Track E must not start before PR-088 (golden fixtures) and should not tune
anything before PR-092 (scorecards) exists.

---

## Track A — Foundations & hygiene

### PR-085 — Glossary enforcement in UI and docs

**Layer:** frontend + docs. **Depends on:** nothing.

**Objective.** The PR-084 bug was a vocabulary bug. Make it impossible to reintroduce.

**Context.** Glossary is section 1 of this file. UI strings live in
`frontend/lib/features/market_state/market_pulse_screen.dart`,
`market_state_dialog.dart`, `frontend/lib/features/overview/overview_widget.dart`,
help pages in `webpage/help/*.md`.

**Spec.**
1. Copy section 1 (Glossary) into `COMMON.md` under a new `## Glossary` heading.
2. Audit every user-visible string in the files above. Replace deprecated words with glossary
   terms. Keep JSON keys unchanged.
3. Add `frontend/test/vocabulary_test.dart`: reads all `.dart` files under `lib/features/`,
   extracts string literals, fails if any contains (case-insensitive, whole word) `prevalence`
   or `breadth` unless the line has a `// glossary-ok` comment.
4. Every `_showInfoDialog` body in Market Pulse must begin with the glossary definition of the
   card's main number.

**Tests.** The vocabulary test itself; existing widget tests updated for new copy.

**Definition of Done.** Vocabulary test passes; `COMMON.md` has Glossary; help pages match.

---

### PR-086 — Cache the composite tape per timeframe

**Layer:** application + infrastructure + main. **Depends on:** nothing.

**Objective.** `MarketStateService.Calculate` (PR-084) builds the tape on every call by
constructing a `CompositeIndexService` inline and fanning out ~150 candle fetches. The
notification scheduler and setup scanner call `Calculate` frequently. Cache it.

**Context.**
- `application/market/market_state_service.go` → `scoreCompositeTape` builds the tape.
- `application/market/metrics/composite_index_service.go` → `CalculateTape`, `CompositeTape`.
- `infrastructure/market/redis_cached_composite.go` → existing cache decorator for `Calculate`.
- `cmd/api/main.go` lines ~330–346 wire `marketService` and `compositeService`.

**Spec.**
1. In `application/market`, add:
   ```go
   type TapeProvider interface {
       CalculateTape(ctx context.Context, timeframe string, limit int) (metrics.CompositeTape, error)
   }
   ```
   Add `func (s *MarketStateService) SetTapeProvider(tp TapeProvider)`. `scoreCompositeTape`
   uses `s.tape` when non-nil; falls back to current inline construction only when `s.tape == nil`
   and `s.candles != nil` (keeps tests working).
2. In `infrastructure/market/redis_cached_composite.go`, add `CalculateTape` to
   `RedisCachedComposite` with key `{prefix}:tape:{timeframe}:{limit}`. `CompositeTape` contains
   `domain.CandleSeries` which has unexported fields — serialize a flat DTO
   (`[]struct{T int64; O,H,L,C,V float64}` per series + `PreferredSource` + index points) and
   rebuild with `domain.NewCandleUnsafe` + `domain.NewCandleSeries` on read.
3. TTL: same as `compositeCacheTTL` (3 min). Make the TTL timeframe-aware: `min(3m, tf/2)` so 1m
   doesn't serve 3-minute-old tapes.
4. `main.go`: `marketService.SetTapeProvider(compositeUC)`.

**Tests.**
- `RedisCachedComposite.CalculateTape` round-trips a tape through a fake Redis (existing fake
  in tests) and returns an identical `PreferredSeries().Len()` and first/last close.
- `MarketStateService` with a fake `TapeProvider` never calls `CandleProvider.Symbols`.

**Definition of Done.** Second `Calculate` call within TTL performs zero candle fetches
(assert with a spy provider).

---

### PR-087 — Per-timeframe SidewaysV5 config actually per timeframe

**Layer:** application + main. **Depends on:** nothing.

**Objective.** `cmd/api/main.go` builds `SidewaysV5ScoreCalculator{Config:
NewSidewaysV5ConfigForTimeframe("1h")}` once and uses it for every timeframe. The config has
timeframe-specific ideal ATR ranges (`IdealATRRangeMap`), so 15m and 1d are mis-scored.

**Context.**
- `domain/scoring/config.go` → `NewSidewaysV5ConfigForTimeframe(tf string)`.
- `domain/scoring/sideways_v5_score_calculator.go`.
- `application/usecases/rank_symbols_wiring.go`, `get_rankings.go` → `rankerForAlgo`, `weights`.
- `application/market/tape_regime.go` already does it right (`NewSidewaysV5ConfigForTimeframe(timeframe)`).

**Spec.**
1. Add to `domain/scoring`:
   ```go
   // TimeframeAwareCalculator picks a per-timeframe configured calculator.
   type TimeframeAwareCalculator struct {
       Name_   string
       Factory func(tf string) SymbolScoreCalculator
       cache   sync.Map // tf → SymbolScoreCalculator
   }
   func (c *TimeframeAwareCalculator) Name() string
   func (c *TimeframeAwareCalculator) Score(series domain.CandleSeries) (float64, error)
   // uses series.Timeframe().String() to pick/create the inner calculator
   ```
2. In `main.go`, replace the single SidewaysV5 instance with
   `&scoring.TimeframeAwareCalculator{Name_: "Sideways Consistency", Factory: func(tf string) ... }`
   wrapping the logging decorator inside the factory.
3. Same treatment for `CompressionScoreCalculator` and `BreakoutScoreCalculator` if they have
   per-timeframe config (check `config.go`); otherwise leave.

**Tests.**
- Same candle shape scored at `15m` and `1d` through the wrapper produces different results when
  the underlying config differs (assert not equal), and identical results when called twice on
  the same timeframe (cache works, deterministic).

**Definition of Done.** No hardcoded `"1h"` in `main.go` for score calculators.

---

### PR-088 — Golden candle fixtures

**Layer:** tests. **Depends on:** nothing. **Blocks:** all of Track E.

**Objective.** Current tests score synthetic lines and sine waves. Real markets are neither.
Record real snippets per regime archetype and pin score ranges so algorithm changes can be
judged against reality.

**Spec.**
1. Add `backend/tests/fixtures/candles/README.md` explaining the format and how to add a
   fixture.
2. Fixture format: JSON, one file per archetype:
   `{"symbol":"BTCUSDT","timeframe":"15m","archetype":"clean_uptrend","source":"binance","recorded":"2026-09-18","candles":[{"t":..., "o":..,"h":..,"l":..,"c":..,"v":..}]}`
   Exactly 110 candles (matches `sparklinePrecision`).
3. Archetypes to record (use `adapters/candle_repository` or a one-off script under `/tmp`;
   do **not** commit the script): `clean_uptrend`, `messy_uptrend` (the PR-084 case: +8–12%
   with chop), `clean_downtrend`, `tight_range`, `wide_range`, `compression_pre_breakout`,
   `breakout_up_with_volume`, `failed_breakout`, `v_reversal`, `flat_dead` (silent).
4. Add `tests/fixtures/candles/loader.go` (package `fixtures`) with
   `func Load(t *testing.T, name string) domain.CandleSeries`.
5. Add `tests/domain/scoring/golden_test.go`: for each archetype, a table of expected score
   *ranges* per calculator (e.g. `clean_uptrend`: Trend ≥ 0.6, Sideways ≤ 0.3). Start with
   ranges the **current** code satisfies; tighten in Track E.
6. Add `tests/application/market/golden_tape_test.go`: `ScoreMarketTape` on `messy_uptrend`
   must give `Structure.Trend > Structure.Sideways` (this is the PR-084 regression guard).

**Definition of Done.** 10 fixtures committed; golden tests pass on current code; README
explains how to regenerate.

---

### PR-089a — Evaluation store: compute once, read everywhere (writer)

**Layer:** application + infrastructure + main. **Depends on:** PR-086, PR-087.

**Objective.** Rankings, setups, Market Pulse and notifications each fetch and score candle
windows independently. Compute each `(symbol, timeframe)` evaluation once on a schedule, store
it, and let everything read from the store. Cheaper, consistent, and it is the data source
Tracks B and D need.

**Context.**
- `GetRankings.Execute` already produces `RankedResult` (scores + sparkline) for the whole
  universe and `RankingsEvaluationProvider` converts them to `EvaluationSnapshot`.
- `infrastructure/rankings/redis_cached_rankings.go` caches per `(timeframe, sort, algo)`.
- `infrastructure/scheduler/adaptive_scheduler.go` exists for periodic jobs.

**Spec.**
1. New port `application/ports/evaluation_store.go`:
   ```go
   type EvaluationStore interface {
       Put(ctx context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error
       Get(ctx context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error)
       GetSymbol(ctx context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error)
   }
   ```
2. Redis implementation `infrastructure/evaluation/redis_evaluation_store.go`: key
   `eval:{tf}` → JSON array + `eval:{tf}:at` timestamp; per-symbol via a hash `eval:{tf}:sym`.
3. Extend `EvaluationSnapshot` (additive) with `Sparkline []float64` and `ComputedAt int64`
   so readers don't need rankings for sparklines.
4. Writer job `application/evaluation/refresher.go`: for each timeframe in
   `[]string{"15m","1h","4h","1d"}` (1m/5m on demand only), call `GetRankings.Execute` with
   `SortByTotal`, convert via `EnrichFromSparkline`, `Put`. Interval per timeframe:
   `max(30s, tf/4)`. Wire in `main.go` behind env `PC_EVAL_REFRESH=1` (default on).
5. Do **not** change any reader in this PR.

**Tests.** Store round-trip with fake Redis; refresher calls `Put` once per timeframe per tick.

**Definition of Done.** Store populated in background; nothing else changes.

---

### PR-089b — Evaluation store: migrate readers

**Layer:** application + main. **Depends on:** PR-089a.

**Spec.**
1. `RankingsEvaluationProvider.GetLatestEvaluations` → read from `EvaluationStore.Get`; if
   store is empty or older than `2 × refresh interval`, fall back to computing (current path)
   and log `[eval] cache miss tf=…`.
2. `SetupService.Evaluate` → use `EvaluationStore.GetSymbol` for scores it currently recomputes
   (check `buildContext`); keep candle fetch only for things not in the snapshot.
3. Notification scheduler `checkMarketState` → unchanged API, benefits automatically.
4. `GetRankings` HTTP path unchanged (still computes; it is the writer). Optionally serve from
   store when `?fresh=0`.

**Tests.** Provider returns store data without calling rankings when fresh; falls back when
stale (use a fake clock).

**Definition of Done.** `Calculate` for Market Pulse performs zero candle fetches for
participation on a warm cache; only the tape (already cached by PR-086) touches candles.

---

## Track B — Outcome tracking & calibration

### PR-090 — Signal log (domain + storage + writers)

**Layer:** domain + application + infrastructure + main. **Depends on:** PR-086.

**Objective.** Record every user-facing call the app makes so it can be graded later.

**Spec.**
1. Domain `domain/signal/signal.go`:
   ```go
   type Kind string
   const (
       KindBadge        Kind = "badge"          // rankings ↑T / ↔S / gain badge
       KindSetup        Kind = "setup"          // SetupService result with Confidence ≥ 0.5
       KindRegime       Kind = "regime"         // tape regime change (observer)
       KindTransition   Kind = "transition"     // transition prob for a target regime ≥ 0.5
   )
   type Signal struct {
       ID          string    // uuid
       Kind        Kind
       Symbol      string    // "" for market-wide
       Timeframe   string
       Label       string    // e.g. "trend_up", "sideways", "compression", "regime:trend"
       Score       float64   // primary score at emission (0–1)
       Price       float64   // close at emission (0 for market-wide; use composite close)
       ATR         float64   // ATR proxy at emission (for excursion normalization)
       Context     map[string]float64 // any extra numbers (confidence, breakout up/down…)
       EmittedAt   time.Time
       HorizonBars int       // how many bars until evaluation (default 20)
   }
   ```
2. Port `application/ports/signal_repository.go`:
   `Append(ctx, Signal) error`, `Unresolved(ctx, before time.Time, limit int) ([]Signal, error)`,
   `MarkResolved(ctx, id string, outcome signal.Outcome) error`,
   `Query(ctx, filter SignalFilter) ([]SignalWithOutcome, error)`.
3. SQLite implementation `infrastructure/signal/sqlite_repository.go`; two tables `signals`,
   `outcomes` (outcome schema defined in PR-091; create both tables now). DB path env
   `PC_SIGNAL_DB`, default `./signals.sqlite`.
4. Writers (all best-effort, never fail the caller, dedupe by
   `(kind, symbol, timeframe, label)` within one bar):
   - Rankings: after `assignBadges`, emit `KindBadge` for each `BadgeComponent != ""`.
   - Setups: in `SetupService.Evaluate`, emit `KindSetup` when `Confidence ≥ 0.5`.
   - Market: a second `RegimeObserver` that emits `KindRegime` only when the regime differs
     from the previous emission for that timeframe.
   - Transition: in `transition_service.go`, emit `KindTransition` when any target ≥ 0.5.
5. Dedup: in-memory `map[string]time.Time` keyed as above, TTL = one bar of the timeframe.

**Tests.** SQLite round-trip; dedup blocks second emission within a bar and allows after;
writers are no-ops when repository is nil.

**Definition of Done.** Signals appear in the DB during normal operation; API unchanged.

---

### PR-091 — Outcome evaluator job

**Layer:** application + infrastructure + main. **Depends on:** PR-090.

**Objective.** Grade resolved signals.

**Spec.**
1. Domain `domain/signal/outcome.go`:
   ```go
   type Outcome struct {
       SignalID        string
       ResolvedAt      time.Time
       ForwardReturn   float64 // (close_h - price)/price, signed
       MaxFavorable    float64 // max favorable excursion in ATR units
       MaxAdverse      float64 // max adverse excursion in ATR units
       Success         bool
       Rule            string  // which rule decided Success
   }
   ```
2. Success rules (implement as pure functions in domain, table-tested):
   - `badge trend_up`: `ForwardReturn > 0 && MaxAdverse < 2.0`.
   - `badge trend_down`: mirror.
   - `badge sideways` / `setup range`: price stayed within `[low, high]` of the emission window
     (store window bounds in `Context["range_low"]`, `Context["range_high"]`) for
     `HorizonBars`; success if never closed outside by more than `0.5 × ATR`.
   - `setup compression` / `badge compression`: realized range over horizon ≥ `1.5 ×`
     emission-window range (expansion happened).
   - `setup breakout_up`: `MaxFavorable ≥ 2.0 && MaxAdverse < 1.0` within horizon.
   - `regime:trend`: composite forward return sign matches `Context["bias"]` (+1/−1) and
     `|ForwardReturn| > 0.5 × ATR/price`.
   - `regime:sideways`: composite `|ForwardReturn| < 0.5 × ATR/price`.
   - `transition:X`: the regime at `EmittedAt + horizon` equals X (read regime history).
3. Job `application/signal/evaluator.go`: every 5 min, `Unresolved(before = now − horizon)`,
   fetch candles via `GetSeries(symbol, tf, EmittedAt, EmittedAt + HorizonBars×tf)` (for
   market-wide use the composite tape from `TapeProvider`), compute, `MarkResolved`.
4. Bound work: max 200 signals per tick.

**Tests.** Each rule table-tested with hand-built candle arrays (success and failure case
each). Evaluator resolves a signal end-to-end with fake repo + fake candles.

**Definition of Done.** Outcomes table fills; no signal older than horizon + 10 min stays
unresolved in steady state.

---

### PR-092 — Scorecard API

**Layer:** application + adapters + infrastructure. **Depends on:** PR-091.

**Spec.**
1. `application/signal/scorecard_service.go`:
   ```go
   type Bucket struct { Lo, Hi float64; N int; Hits int; HitRate float64; AvgReturn float64 }
   type Scorecard struct {
       Kind, Label, Timeframe string
       Since time.Time
       Total, Hits int
       HitRate float64
       Buckets []Bucket   // score deciles: [0,0.1), [0.1,0.2) …
       Baseline float64   // hit rate of a random signal of same kind (see below)
   }
   func (s *ScorecardService) Get(ctx, kind, label, tf string, since time.Time) (Scorecard, error)
   ```
   Baseline: for `badge`/`setup` kinds, fraction of **all** symbols that would have passed the
   success rule over the same period (compute lazily by sampling 50 random `(symbol, time)`
   pairs from the signal log's time span; cache 1h). Without a baseline, a 55% hit rate is
   meaningless.
2. Endpoint `GET /api/scorecards?kind=&label=&timeframe=&since=30d`. Redis cache 10 min.
3. Add `GET /api/scorecards/summary?timeframe=` returning one line per `(kind,label)` for the UI
   chip: `{ "kind":"badge","label":"trend_up","hitRate":0.58,"baseline":0.49,"n":412 }`.
4. Document both in `COMMON.md`.

**Tests.** Handler JSON shape; bucket math with 20 fake outcomes; baseline > 0 when
sampled.

**Definition of Done.** Curl returns non-empty scorecards after one day of operation.

---

### PR-093 — Scorecards in the app

**Layer:** frontend. **Depends on:** PR-092.

**Spec.**
1. `features/scorecards/` with `scorecard_data.dart`, `http_scorecard_api.dart`,
   `scorecards_screen.dart` (list of `(kind,label)` rows: hit rate vs baseline, n, last 30d,
   tap → deciles bar chart).
2. Reliability chip: small pill `58% · n=412` next to
   - the `↑T` badge on rankings tiles (`overview_widget.dart`),
   - the headline in Market Pulse (`_buildRegimeCard`),
   - the setup card in symbol detail.
   Color: green if `hitRate − baseline ≥ 0.05`, grey if within ±0.05, red if below.
   Tooltip/dialog copy: "Of the last N times the app showed this, X% worked out. Chance: Y%."
3. Entry point: Settings → "Reliability" and a link in the Market Pulse info dialog.
4. Chips hidden when `n < 30`.

**Tests.** Widget tests for chip color thresholds and hidden-when-small-n; JSON parsing.

---

### PR-094 — Score distribution telemetry store

**Layer:** infrastructure + adapters. **Depends on:** PR-089a.

**Objective.** `LoggingScoreCalculator` prints samples to logs. Persist them so Track E can
compute percentiles.

**Spec.**
1. Port `ports.ScoreSampleSink` with `Record(calculator, symbol, tf string, score float64, at time.Time)`.
2. SQLite sink `infrastructure/scoring/sqlite_sample_sink.go` (table `score_samples`, indexed
   on `(calculator, tf, at)`); retention 90 days (delete on startup and daily).
3. `LoggingScoreCalculator` gets `SetSink(sink)`; when set, records instead of logging.
   Sample rate from env `PC_SCORE_SAMPLE_RATE` (default 0.1).
4. Query `application/scoring/percentiles.go`:
   `Percentile(ctx, calculator, tf string, score float64) (float64, error)` and
   `Distribution(ctx, calculator, tf string) ([]float64 /*p5..p95 step 5*/, error)`.
5. Endpoint `GET /api/debug/score-distribution?calculator=&timeframe=` behind env
   `PC_DEBUG_ENDPOINTS=1`.

**Tests.** Sink round-trip; percentile of a known sample set.

---

## Track C — Composite v2 & relative strength

### PR-095 — Composite math v2

**Layer:** application. **Depends on:** PR-086.

**Objective.** Fix three inherited weaknesses in `CompositeIndexService.CalculateTape`:
index-position alignment, arithmetic rebase, no exclusions.

**Spec.**
1. **Timestamp alignment.** Build the reference timeline as the sorted union of all symbols'
   timestamps that appear in ≥ 50% of symbols. For each symbol, map `ts → candle`. At each
   reference `ts`, include only symbols that have that exact `ts`. Never align by index `i`.
2. **Log-return aggregation.** Per symbol compute `r_i = ln(close_i / close_{i−1})`. Aggregate
   per timestamp: median (equal-weight) and weighted mean (volume-weighted) of `r_i`. Index
   value: `V_0 = 100`, `V_i = V_{i−1} × exp(agg_r_i)`. OHLC of the synthetic candle: apply the
   same aggregated log-return to open/high/low relative to the previous synthetic close
   (`O_i = V_{i−1} × exp(agg(ln(open_i/close_{i−1})))`, etc.), then clamp high/low around
   open/close as today.
3. **Exclusions.** `config.yaml` gains `composite.exclude: [USDCUSDT, FDUSDUSDT, TUSDUSDT,
   WBTCUSDT, WBETHUSDT, ...]` plus a regex `^(USD|EUR|.*USD)` guard for stablecoin quote
   pairs. Excluded symbols are skipped before fetching.
4. Weight for volume-weighted path stays `Σ volume×close` over the window, but computed **only
   over aligned bars**.
5. Keep the public API identical (`Calculate`, `CalculateTape`).

**Tests.**
- Two symbols with one missing bar in the middle: the median at that timestamp uses only the
  present symbol; the timeline still has that bar.
- One symbol +50% then −33% (back to start) and one flat: arithmetic mean would be +8%; log
  version ends at 100 ± 0.01.
- Excluded symbol never fetched (spy provider).
- Existing composite tests still pass (adjust tolerances where log math differs).

---

### PR-096 — Relative strength per symbol (backend)

**Layer:** application + adapters. **Depends on:** PR-095.

**Spec.**
1. In `GetRankings.Execute`, after all sparklines are fetched, compute the composite over the
   same 110 bars (reuse `CompositeIndexService.CalculateTape(ctx, tf, precision)` via a new
   optional `SetTapeProvider` on `GetRankings`; when nil, RS fields stay zero).
2. Per symbol:
   - `symRet = ln(spark[last]/spark[first])`, `mktRet = ln(tape[last]/tape[first])`
   - `RS = symRet − mktRet` (log excess return)
   - `Beta` = OLS slope of symbol log-returns on tape log-returns over the window (guard: if
     tape variance < 1e−12, beta = 0)
   - `RSRank` = percentile of RS across the universe (0–1)
3. Add to `RankedResult`: `RelativeStrength, Beta, RSRank float64`. Add to the rankings DTO
   (`adapters/http/rankings_v2_handler.go`, `dto.go`): `"rs"`, `"beta"`, `"rsRank"`.
4. New sort modes `SortByLeaders = "leaders"` (RS desc) and `SortByLaggards = "laggards"`
   (RS asc). Register in `ScoreKeyForSort`-adjacent switch (RS is not a calculator score; add
   a dedicated branch in the sort function).
5. `COMMON.md`: document the three fields and two sort modes.

**Tests.** Symbol identical to tape → RS ≈ 0, beta ≈ 1. Symbol = 2× tape returns → beta ≈ 2.
Sort `leaders` puts highest RS first.

---

### PR-097 — Leaders / Laggards in the grid

**Layer:** frontend. **Depends on:** PR-096.

**Spec.**
1. `overview_view_model.dart`: add sort cases `'leaders'`, `'laggards'` (sort by `rs`).
2. Sort menu entries "Leaders (vs market)" and "Laggards (vs market)".
3. Tile: small RS chip bottom-right, `+3.2% vs mkt` / `−1.1% vs mkt` (RS × 100, one decimal),
   green/red. Hide when `rs == 0 && beta == 0` (backend RS disabled).
4. Tap chip → info dialog: "Relative strength: this symbol's return minus the market
   composite's return over the same window. Beta: how much it moves per 1% market move."

**Tests.** Sort order test with three fake items; chip hidden when RS disabled.

---

### PR-098 — Sector composites & rotation card

**Layer:** backend + frontend. **Depends on:** PR-095.

**Spec.**
1. `backend/config/sectors.yaml`:
   ```yaml
   sectors:
     - id: l1
       name: Layer 1
       symbols: [BTCUSDT, ETHUSDT, SOLUSDT, ADAUSDT, AVAXUSDT, NEARUSDT, DOTUSDT, ATOMUSDT]
     - id: defi
       name: DeFi
       symbols: [UNIUSDT, AAVEUSDT, MKRUSDT, ...]
     - id: meme
       ...
     - id: ai
       ...
   ```
   Unlisted symbols → sector `other`.
2. `application/market/metrics/sector_index_service.go`: for each sector, run the same
   composite math (PR-095) restricted to its symbols; output
   `SectorIndex{ID, Name, SymbolCount, Points, Return, RS}` where `RS` = sector log return −
   market composite log return.
3. Endpoint `GET /api/market/sectors?timeframe=&limit=` → `{ "timeframe":..., "sectors":[...] }`.
   Redis cache 3 min.
4. Frontend Market Pulse card "Sector rotation": horizontal bars of sector RS sorted
   descending; tap → sector sparkline overlay on the composite chart.

**Tests.** Sector with 2 symbols → SymbolCount 2; RS of "all symbols" pseudo-sector ≈ 0.

---

## Track D — Multi-timeframe & alerts

### PR-099 — Multi-timeframe regime stack (backend)

**Layer:** application + adapters. **Depends on:** PR-089b.

**Spec.**
1. `application/mtf/service.go`:
   ```go
   type TFRegime struct { Timeframe string; Structure mkt.Breadth; Dominant mkt.State; Bias string; Score float64 }
   type Stack struct { Symbol string; Frames []TFRegime; Alignment float64; AlignedState mkt.State }
   ```
   For each tf in `["15m","1h","4h","1d"]`, read `EvaluationStore.GetSymbol`, build the
   four-way structure with `scoreWeights` (export it as `ScoreWeights`), dominant = max.
   `Alignment` = share of frames whose dominant equals the most common dominant (0.25–1.0).
   `AlignedState` = that most common dominant when `Alignment ≥ 0.75`, else `indecisive`.
2. Endpoint `GET /api/symbol/{symbol}/regimes` (add route under existing
   `/api/symbol/` handler or a new `mtf_handler.go`).
3. Bulk endpoint for the grid: `GET /api/rankings` gains optional `?mtf=1` that adds
   `"alignment"` and `"alignedState"` to each item (compute from store; zero cost per symbol).

**Tests.** Four frames all trend → alignment 1.0, alignedState trend. Two trend, two sideways
→ 0.5, indecisive.

---

### PR-100 — MTF strip and aligned badge (frontend)

**Layer:** frontend. **Depends on:** PR-099.

**Spec.**
1. Symbol detail: horizontal strip of four pills `15m · 1h · 4h · 1d`, each colored by
   dominant regime (use the same colors as Market Pulse: teal trend, blue-grey sideways, amber
   compression, red expansion) with a small arrow for bias.
2. Rankings tile: when `alignment ≥ 0.75`, show a thin colored top border in the aligned
   regime's color and a tooltip "Aligned: trend on 3/4 timeframes".
3. Sort mode `aligned` (alignment desc, then total score).

**Tests.** Pill colors per regime; border shown only ≥ 0.75.

---

### PR-101 — Watchlist with state-transition alerts

**Layer:** backend + frontend. **Depends on:** PR-099, notifications infra.

**Context.** Notifications today: `application/notifications/scheduler.go` handles macro
events, market state, setup-of-day; per-user config in `sqlite_config_store.go`; FCM in
`infrastructure/social/fcm_notifier.go`. There is no watchlist yet (grep `watchlist` → 0).

**Spec.**
1. Backend watchlist store `infrastructure/watchlist/sqlite_store.go`: table
   `(user_id, symbol, added_at)`. Endpoints (auth middleware): `GET/PUT/DELETE /api/watchlist`
   with body `{ "symbols": [...] }`. Max 50 symbols per user.
2. Notification config gains `watchlistTransitions bool` and `watchlistTimeframe string`
   (default `1h`).
3. Scheduler: new `checkWatchlistTransitions` every `tf/2`: for each user with the flag on, for
   each symbol, compare current dominant structure (PR-099 service) with the last notified
   state (store per `(user, symbol, tf)` in a `watchlist_state` table). Notify on:
   - `compression → expansion` ("Breakout starting")
   - `sideways → trend` ("Trend starting, bias up/down")
   - `trend → sideways|compression` ("Trend pausing")
   Dedup: at most one push per `(user, symbol)` per 4 bars.
4. Payload: `{ "type":"watchlist_transition","symbol":..,"timeframe":..,"from":..,"to":..,
   "bias":..,"score":.. }`.
5. Frontend: star toggle on tile and in detail persists via the API (local cache in
   `SharedPreferences` for offline); "Watchlist" filter chip in the grid; settings toggle for
   the alert.

**Tests.** Transition detection table; dedup window; endpoint auth (401 without token).

---

### PR-102 — Contextual alert payloads

**Layer:** backend + frontend. **Depends on:** PR-101.

**Spec.**
1. Every market/setup/watchlist push includes `context`:
   `{ "tapeRegime":"trend","tapeBias":"up","tapeConfidence":0.62,"symbolScore":0.81,
      "rs":0.032,"alignment":0.75,"sparkline":[... 30 closes ...] }`.
   Sparkline is the last 30 closes downsampled from the 110-bar snapshot (take every 4th
   value, keep first and last).
2. Frontend notification rendering (`features/social/notification_service.dart` or the
   notifications feature): when `context.sparkline` present, show a big-picture style
   notification with a rendered mini chart (use `flutter_local_notifications` big picture with
   an image painted via `CustomPainter` → `ui.Image` → PNG bytes).
3. Tap → deep link to symbol detail on the alert's timeframe (`initialTimeframe` already exists
   on Market Pulse; add the same to detail route).

**Tests.** Downsampling keeps first/last and length 30; payload JSON shape.

---

## Track E — Algorithm upgrades

All slices here: add the new calculator **next to** the old one, never replace in place.
Switch consumers via `config.yaml` flag. Prove improvement with PR-088 golden fixtures and
PR-092 scorecards before flipping the default.

### PR-103 — Trend Strength v2 (direction persistence)

**Layer:** domain + application + config. **Depends on:** PR-088, PR-094.

**Objective.** `TrendPredictabilityScoreCalculator` = R² × slope × shape penalties. R²
punishes noise, not lack of direction, so a messy +10% grind scores ~0.05. Add a calculator
that measures *persistent direction*.

**Spec.** New `domain/scoring/trend_strength_score_calculator.go`, name `"Trend Strength"`.
Inputs: last `N = 110` closes `c`, highs, lows. Compute on log prices `p = ln(c)`.
1. **Efficiency ratio** `ER = |p_N − p_1| / Σ|p_i − p_{i−1}|` (0–1). Random walk ≈ 0.1–0.2,
   clean trend → 1.
2. **Swing structure.** Find swing highs/lows with a 3-bar pivot rule (`h_i > h_{i±1..3}`).
   Let `HH` = fraction of consecutive swing highs that are higher, `HL` = fraction of
   consecutive swing lows that are higher. `Swing_up = (HH + HL)/2`; `Swing_down` mirror with
   lower highs/lows. `Swing = max(Swing_up, Swing_down)`, direction from which is larger.
   If fewer than 3 swings, `Swing = ER`.
3. **Time above/below trend line.** Fit robust slope via Theil–Sen (median of pairwise slopes,
   sample 500 pairs max) on `p`. `Persist = fraction of bars where sign(p_i − fit_i)` matches
   the previous bar (smoothness proxy, 0–1). Simpler alternative allowed if Theil–Sen is too
   slow: fraction of bars where `p_i > EMA_20(p)_i` (for up) or `<` (for down).
4. **Drawdown penalty.** `DD = (max(p) − p_N) / (max(p) − min(p))` for up direction (mirror for
   down); `DDpen = 1 − clamp(DD × 2, 0, 1)` (a 50% retrace of the range zeroes it).
5. **Magnitude gate.** `Mag = clamp(|p_N − p_1| / (ATR_pct × sqrt(N)), 0, 1)` where
   `ATR_pct` = mean `(h−l)/c`. Move must exceed noise expected from a random walk.
6. Score `= ER^0.5 × (0.4 × Swing + 0.3 × Persist + 0.3) × DDpen × Mag`, clamp 0–1.
   `ScoreWithDirection` returns sign from `p_N − p_1` (neutral if `Mag < 0.2`).
7. Config flag `scoring.trend_algo: predictability|strength` (default `predictability`).
   When `strength`, `main.go` uses it under the same `"Trend Predictability"` key so all
   consumers (rankings, `scoreWeights`, `ScoreMarketTape`) pick it up without changes.

**Tests.** Golden: `messy_uptrend` ≥ 0.5, `clean_uptrend` ≥ 0.7, `tight_range` ≤ 0.2,
`v_reversal` ≤ 0.3 (drawdown penalty). Direction correct on `clean_downtrend`. Pure random
walk (seeded) averages ≤ 0.25 over 100 seeds.

**Definition of Done.** Flag exists, default unchanged, golden tests pass for both algos.
Flip the default only after a scorecard comparison (`badge trend_up` hit rate) over ≥ 2 weeks.

---

### PR-104 — Sideways: variance-ratio mean-reversion component

**Layer:** domain. **Depends on:** PR-088.

**Spec.**
1. Add `VarianceRatio(closes []float64, q int) float64` in `domain/scoring/stats.go`:
   `VR(q) = Var(r_q) / (q × Var(r_1))` with `r_q` = q-period log returns, overlapping,
   Lo–MacKinlay bias correction optional. Mean-reverting series → `VR < 1`; trending → `> 1`.
2. `MeanReversionScore = clamp((1 − VR(4)) × 1.5, 0, 1)` averaged with `q = 8`.
3. Sideways V5 gains a config-weighted component `mean_reversion_weight` (default 0.0 → no
   behavior change). Expose in `SidewaysV5Config`.
4. Also expose `VR` in the setup context (`SetupContext.VarianceRatio`) for PR-110.

**Tests.** Seeded AR(1) with φ = −0.5 → VR(4) < 0.8; seeded random walk → 0.85–1.15; seeded
trend + noise → > 1.2. Golden `tight_range` mean-reversion score ≥ 0.5.

---

### PR-105 — Percentile-normalized compression

**Layer:** domain + application. **Depends on:** PR-094.

**Objective.** Compression uses absolute divisors (`MaxExpectedATRDrop: 0.005` etc.) that
cannot be right for every symbol and timeframe. Normalize against the symbol's own history.

**Spec.**
1. `CompressionPercentileScoreCalculator` (name `"Compression Pct"`): needs a longer window,
   `M = 500` bars (fetch via `GetLastNCandles(…, 500)`; if fewer than 200 available, return the
   legacy score). Compute per bar `bw_i = (max(h_{i−19..i}) − min(l_{i−19..i})) / c_i`
   (20-bar range width) and `atr_i` (14-bar). Current percentile rank `P_bw` of `bw_N` and
   `P_atr` of `atr_N` within the 500-bar history (0 = tightest ever).
2. `SqueezeDuration` = number of consecutive bars with `bw < 20th percentile`, capped at 20,
   normalized `/20`.
3. Score `= (1 − P_bw) × 0.5 + (1 − P_atr) × 0.3 + SqueezeDuration × 0.2`.
4. Flag `scoring.compression_algo: absolute|percentile` (default absolute). The rankings
   pipeline fetches 110 bars; when `percentile` is on, fetch 500 for this calculator only
   (add `WindowHint() int` optional interface; `GetRankings` uses `max` over calculators).

**Tests.** Synthetic 500-bar series where the last 20 bars have half the range of the rest →
score ≥ 0.7; uniform noise → ≈ 0.5 ± 0.15.

---

### PR-106 — Trend health v2 (true ATR, wider tolerance)

**Layer:** application. **Depends on:** PR-088.

**Context.** `health.go` → `ComputeTrendHealth`: health = `1 − (high − price)/atr` clamped;
`atr` is mean |Δclose| (from `EnrichFromSparkline` / `sparklineStats`). One average bar below
the window high = health 0. Real uptrends spend most of their life 1–3 ATR under the high.

**Spec.**
1. `ComputeTrendHealthV2(state string, price, recentHigh, recentLow, atr14, recentReturnATR float64, barsSinceExtreme int) float64`:
   - `dd = (recentHigh − price) / atr14` (mirror for down)
   - `ddScore = 1 − clamp((dd − 1.0) / 2.5, 0, 1)` → full health up to 1 ATR, zero at 3.5 ATR
   - `staleness = clamp(barsSinceExtreme / 40, 0, 1)`; `staleScore = 1 − 0.5 × staleness`
   - crash penalty as today (`recentReturnATR < −1.5 → × 0.3`)
   - health `= ddScore × staleScore`
2. `atr14` = true ATR (Wilder, 14) — add `TrueATR(candles, 14)` in `domain/scoring/stats.go`
   (reuse `rollingATR` from compression if signature fits).
3. `ScoreMarketTape` uses V2 (it has the full OHLC series). Participation fallback keeps V1.
4. `DampenTrendByHealth`: change floor from 0.1 to 0.35 — dampening should never turn a
   dominant trend into a 5% bar by itself.

**Tests.** Price 2 ATR under high → health ≈ 0.6 (not 0). Golden `messy_uptrend` tape:
`Structure.Trend` after dampening ≥ 0.8 × before dampening.

---

### PR-107 — Empirical transition matrix from regime history

**Layer:** application. **Depends on:** regime history (exists), PR-090 optional.

**Context.** `transition_engine.go` hand-codes probabilities per source regime using
`ExpansionPressure`. `regimehistory.Repository.GetHistory(tf, limit)` returns real past
periods with durations.

**Spec.**
1. `application/market/transition/empirical.go`:
   ```go
   type AgeBucket int // 0: <25% of median duration, 1: 25–75%, 2: 75–150%, 3: >150%
   func BuildMatrix(periods []mkt.RegimePeriod) Matrix // Matrix[from][ageBucket][to] = P
   ```
   Count transitions `from → to` grouped by the age bucket the `from` period had when it
   ended. Laplace smoothing `+1` per cell. Need ≥ 30 transitions per `from` row to be
   "confident"; else mark row as low-confidence.
2. Blend: `P = w × P_empirical + (1 − w) × P_heuristic` with
   `w = clamp(n_from / 100, 0, 0.7)` (never fully trust history).
3. `TransitionService.Calculate` uses the blend; response adds `"source":"blend"`,
   `"empiricalWeight": w`, `"sampleSize": n_from`.
4. Rebuild matrix every 15 min per timeframe (cache in memory).

**Tests.** 40 synthetic periods `compression → expansion` (all) → P(expansion | compression)
≥ 0.9 after smoothing; heuristic still returned when history is empty.

---

### PR-108 — Volatility seasonality per sector

**Layer:** infrastructure + application. **Depends on:** PR-098, PR-082.

**Spec.**
1. `cmd/vol_aggregate` accepts `--symbols` (comma list) and `--out` prefix; produce one result
   file per sector (`vol_l1.json`, `vol_defi.json`, …) plus the existing market-wide BTC file.
2. `SeasonalityProvider` gains `CurrentSpikeProbabilityFor(ctx, sector, tf string)`; falls back
   to market-wide when the sector file is missing.
3. `SetupService.buildContext` resolves the symbol's sector (PR-098 config) and uses it.

**Tests.** Missing sector file → fallback value equals market-wide value.

---

### PR-109 — Learned regime classifier (offline first)

**Layer:** tooling + domain. **Depends on:** PR-090–092 (≥ 4 weeks of outcomes), PR-088.

**Spec.**
1. Export tool `cmd/export_dataset`: joins `signals` + `outcomes` + the `EvaluationSnapshot`
   fields at emission into a CSV: features = the four raw scores, ER, VR, ATR pct, RS,
   alignment; label = `Success`.
2. Training is **out of repo** (notebook); ship the result as `config/regime_model.yaml`:
   ```yaml
   model: logistic
   features: [trend, sideways, compression, expansion, er, vr, atr_pct]
   weights: {...}
   bias: -1.23
   ```
3. `domain/scoring/logistic.go`: `Predict(features map[string]float64, model Model) float64`.
4. `ScoreMarketTape` and per-symbol classification gain an optional `Model`; when present, the
   dominant regime is the class with highest predicted success probability among the four; the
   four probabilities normalized become `Structure`. Flag `scoring.regime_model: heuristic|learned`.
5. Never ship `learned` as default without a scorecard A/B showing ≥ +5pp hit rate.

**Tests.** `Predict` matches hand-computed sigmoid; classifier falls back to heuristic when
model file is absent.

---

## Track F — Trade execution UX

### PR-110 — Range trade planner (backend)

**Layer:** application + adapters. **Depends on:** PR-104, setups service.

**Spec.**
1. `application/plan/range_planner.go`:
   ```go
   type RangePlan struct {
       Symbol, Timeframe string
       Low, High, Mid    float64   // channel from swing pivots (last 110 bars)
       ATR               float64   // true ATR 14
       LongEntry, LongStop, LongTarget    float64 // entry = Low + 0.25×ATR, stop = Low − 1.0×ATR, target = Mid (conservative) and High − 0.25×ATR (full)
       ShortEntry, ShortStop, ShortTarget float64 // mirror
       RiskReward        float64   // (target − entry)/(entry − stop) for the conservative target
       RangeQuality      float64   // sideways score × mean-reversion score (PR-104)
       Position          float64   // (price − Low)/(High − Low): 0 = at support, 1 = at resistance
       Valid             bool      // RangeQuality ≥ 0.5 && RiskReward ≥ 1.2 && (High−Low)/ATR ≥ 3
       Reason            string    // why invalid, if !Valid
   }
   ```
   Channel: `Low` = median of the 3 lowest swing lows, `High` = median of the 3 highest swing
   highs (3-bar pivot rule); if < 3 swings on a side use min/max of that side.
2. Position sizing helper (pure): `Size(accountRisk, entry, stop float64) float64 =
   accountRisk / |entry − stop|` — the client passes `accountRisk` in quote currency.
3. Endpoint `GET /api/symbol/{symbol}/plan?timeframe=&risk=100` → plan + `size`.

**Tests.** Hand-built range 100–110 with ATR 1: Low ≈ 100, High ≈ 110, LongEntry 100.25,
LongStop 99, conservative target 105, RR ≈ 3.8, Valid. Trending series → `Valid=false`,
`Reason="range quality"`.

---

### PR-111 — Range planner UI

**Layer:** frontend. **Depends on:** PR-110.

**Spec.**
1. Symbol detail: "Plan" tab/section. Draw `Low/Mid/High` as horizontal lines on the existing
   candle chart (`features/detail/candle_series_chart_renderer.dart`) with entry/stop/target
   ticks on the right axis (`sticky_price_labels.dart`).
2. Panel: Long / Short toggle, entry/stop/target values, R:R, "Position in range" bar, quality
   dots. Account-risk input (persisted in `SharedPreferences`, default 100 USDT) → size.
3. When `Valid=false` show the `Reason` and no levels.
4. Copy is educational, not advisory: "Levels derived from the last 110 bars' swing structure.
   Not financial advice."

**Tests.** Widget renders three lines when valid, none when invalid; size recomputes on risk
change.

---

### PR-112a — Replay mode (backend `asOf`)

**Layer:** application + adapters. **Depends on:** PR-089b.

**Spec.**
1. `GET /api/rankings`, `/api/market/regime`, `/api/market/composite`, `/api/market/transition`
   accept `asOf=<unix seconds>`. When present:
   - candle fetches use `GetSeries(symbol, tf, asOf − N×tf, asOf)` instead of `GetLastNCandles`
   - caches are bypassed (or keyed by `asOf` rounded to the bar)
   - regime history returns only periods ending before `asOf`
2. Rate-limit `asOf` requests separately (middleware bucket `replay`, 10/min/user) — they are
   expensive.
3. Pro-only (check `userHasProAccess` pattern from the scheduler).

**Tests.** Rankings with `asOf` = a fixture timestamp returns the fixture's scores; without
`asOf` unchanged.

---

### PR-112b — Replay mode (frontend scrubber)

**Layer:** frontend. **Depends on:** PR-112a.

**Spec.**
1. Market Pulse and grid get a "Replay" toggle (Pro). When on, a bottom scrubber with a date
   picker and step buttons (−1 bar / +1 bar / −1 day / +1 day) sets `asOf`.
2. All API calls carry `asOf`; a persistent banner "Replay: Sep 16 08:30 UTC" with an Exit
   button. Auto-refresh paused while in replay.
3. If PR-093 is present, show the scorecard chip for the replayed signal as it would have been
   known **then** (`since` ≤ `asOf`).

**Tests.** Scrubber emits `asOf` aligned to the bar; auto-refresh timer is paused in replay.

---

## Appendix A — PR doc template

```
## PR-0xx: <type>(<area>): <title>

🎯 Objective
<why; one paragraph; link the ROADMAP slice>

🧱 Scope
<layers and files touched; what is explicitly out of scope>

1️⃣ <section>
2️⃣ <section>
…

✅ Definition of Done
- [ ] tests listed in ROADMAP pass
- [ ] gofmt / go vet / flutter analyze clean
- [ ] COMMON.md updated (if JSON changed)
- [ ] help page updated (if user-facing copy changed)

🚀 Result
<what the user sees>

📝 Implementation notes
<decisions taken where the spec was silent; known limitations>
```

## Appendix B — Success criteria for the round

- A green tape never sits under a "sideways" headline unless the tape itself scores sideways
  (golden `messy_uptrend` guard).
- Every badge, setup, and regime call shows a hit rate vs baseline after 30 days.
- Market Pulse `Calculate` is O(1) candle fetches on a warm cache.
- No hardcoded timeframe in the composition root.
- At least one algorithm default flipped based on scorecard evidence, not intuition.
