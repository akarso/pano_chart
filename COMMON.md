# COMMON.md

## Purpose

This document defines **shared contracts and semantics** between backend and frontend.

It is the **only place** where cross-repository coupling is allowed.

Both backend and frontend must treat this document as **authoritative**.

---

## Guiding Rules

* Backend and frontend evolve independently
* Shared meaning lives here, not in code comments
* Any breaking change here requires coordinated releases
* Prefer additive changes over breaking ones

---

## Core Concepts (Shared Semantics)

### Symbol

Represents a tradable market instrument.

**Rules**:

* Case-insensitive
* Normalized representation is uppercase
* Allowed characters: `A–Z`, `0–9`, `-`, `_`

**Examples**:

* `BTCUSDT`
* `ETH-USD`

---

### Timeframe

Represents candle aggregation interval.

**Canonical values**:

* `1m`
* `5m`
* `15m`
* `1h`
* `4h`
* `1d`

**Rules**:

* Timeframes are discrete and finite
* Backend may reject unsupported values

---

### Candle

Represents OHLCV market data for a symbol and timeframe.

**Fields**:

* `timestamp` (UTC, epoch milliseconds)
* `open`
* `high`
* `low`
* `close`
* `volume`

**Invariants**:

* `high >= max(open, close)`
* `low <= min(open, close)`
* All numeric values are non-negative

---

## Glossary

Every number shown to a user must map to exactly one term below.
Backend JSON may keep legacy field names for compatibility; UI copy must use
these terms.

| Term | Definition | Where it comes from |
|---|---|---|
| **Tape** | The merged market series (composite OHLCV) | `CompositeIndexService.CalculateTape` |
| **Regime** | Dominant structure of the tape: trend / sideways / compression / expansion / indecisive / silent | `ScoreMarketTape` |
| **Tape confidence** | When state is trend: raw `TapeTrend`. Otherwise: share of the dominant structure in the tape's score mix (0–1) | `TapeRegime.Confidence` |
| **Structure** | The tape's four-way score mix | `TapeRegime.Structure` |
| **Participation** | Count of tokens with Trend Predictability ≥ 0.5 and sparkline bias up/down (else ranging). Legacy JSON `metrics.*Breadth` remains the average per-token score mix | `MarketStateService` `participation` / breadth |
| **Bias** | Direction of the tape's trend: up / down / neutral | `TapeRegime.Bias` |
| **Composite (median)** | Equal-weight median of aligned log-returns, index from 100 | `CompositeIndex.Points` |
| **Composite (volume-weighted)** | Quote-volume-weighted mean of aligned log-returns, index from 100 | `CompositeIndex.VolumeWeightedPoints` |
| **Relative strength (RS)** | Symbol log-return minus tape log-return on shared timestamps (`rs`); also `beta`, `rsRank`, `rsAvailable` | Track C / PR-096 |
| **Sector RS** | Sector composite log-return minus market return over the clamped intersection of sector and market timestamps (`rs` + `rsAvailable`); market tape from the same bar fetch | Track C / PR-098 |
| **Signal** | Any user-facing call the app makes at a point in time (badge, setup, regime, transition) | Track B |
| **Outcome** | What happened after a signal over a fixed horizon | Track B |
| **Hit rate** | Fraction of signals whose outcome met the success rule | Track B |

Deprecated words in UI copy: *prevalence*, *breadth*, *scores* (as a UI label).
JSON keys stay unchanged.

---

## API Contracts (Versioned)

### Overview Request

```
GET /overview
```

**Query Parameters**:

* `symbols`: list of symbols
* `timeframe`: timeframe identifier

---

### Overview Response (v1)

```json
{
  "timeframe": "15m",
  "symbols": [
    {
      "symbol": "BTCUSDT",
      "candles": [
        {
          "timestamp": 1700000000000,
          "open": 42000.0,
          "high": 42100.0,
          "low": 41900.0,
          "close": 42050.0,
          "volume": 1234.56
        }
      ]
    }
  ]
}
```

---

## Error Semantics

Errors are returned in a consistent shape.

```json
{
  "error": {
    "code": "INVALID_SYMBOL",
    "message": "Symbol contains illegal characters"
  }
}
```

**Common Error Codes**:

* `INVALID_SYMBOL`
* `INVALID_TIMEFRAME`
* `RATE_LIMITED`
* `INTERNAL_ERROR`

---

## Versioning Rules

* This document is versioned implicitly via git
* Breaking changes require:

  * explicit section annotation
  * coordinated backend + frontend update

---

## Change Process

Any change to this file requires:

* a dedicated PR
* clear description of impact
* agreement from backend and frontend owners

---

## Relationship to Other Docs

* Global rules: `AGENTS.md`
* Backend rules: `BACKEND.md`
* Frontend rules: `FRONTEND.md`

---

## Guiding Principle

If backend and frontend disagree, this document wins.

---

## Market Pulse Semantics (PR-084)

### Headline regime

`GET /api/market/regime` and `GET /api/market/state` set `regime`/`state`,
`prevalence`/`confidence`, `label`, and `bias` from scoring the **merged
composite tape** (volume-weighted when available, else median). The trend
leg is `TapeTrend` (PR-115); sideways / compression / expansion still use
the same calculators as rankings. Rankings / per-token scores are unchanged
(Trend Predictability).

When `state` is `trend`, `confidence` is the raw `TapeTrend` score. Otherwise
`confidence` / `prevalence` is the dominant share of the measured structure
mix.

Additive fields:

* `regimeSource`: `composite_volume_weighted` | `composite_median` | `participation`
* `structure` (state endpoint): tape score mix
* regime `scores`: tape structure (not token vote share)
* `windowBars`: candles actually scored for the headline (`series.Len()`);
  `0` when the headline fell back to participation or data is unavailable
* `trendScore`: raw composite `TapeTrend` (0–1) when composite-backed; `0` on
  the participation fallback. Not the same number as each token's stored
  Trend Predictability score.
* `participation`: `{ up, down, ranging, total }` — counts of stored
  evaluations with `|EvaluationSnapshot.trendScore| ≥ 0.5` (Trend
  Predictability) and sparkline `bias` up/down; else ranging. Feeds PR-116
  copy; it is not a TapeTrend vote.

Tape captions (`label`) match `state`:
* `trend` — V2 health: "Strong trend" (health > 0.75), "Trend weakening"
  (> 0.4), or "Trend breaking down"
* `compression` / `expansion` / `sideways` — "Compression" / "Expansion" /
  "Sideways"
* `indecisive` — "Mixed conditions"
* `silent` — "No clear trend"

The adverse-move (crash/squeeze) penalty looks at the last 8 bars' return in
Wilder true-ATR units, not the full-window net. Compression/expansion on the
tape still use their own SMA ATR (`rollingATR`); only TapeTrend and tape /
participation health use Wilder `TrueATR` when a long enough series is
available. Participation fallback captions follow the headline `state` via
`BuildTapeLabel` (same V2 health cutovers as the tape when `state` is
`trend`):
* "Strong trend" (health > 0.75), "Trend weakening" (> 0.4), or
  "Trend breaking down"
and the same state-matched captions otherwise. Snapshot `RecentReturn` used
by silent/bias overrides is full-window / mean|Δclose| (not Wilder).

### Participation (metrics)

`metrics.trendBreadth` / `sidewaysBreadth` / `compressionBreadth` /
`expansionBreadth` remain **per-token score-mix averages** (legacy). The
count-based `participation` object is the glossary term for up/down/ranging
counts. UI copy must not present either as the headline regime.

### Transition probabilities (PR-107)

`GET /api/market/transition?timeframe=` returns heuristic probabilities
blended with an empirical matrix when the **age bucket (or pooled all-age
row) used for the current regime** has ≥ 30 raw samples:

* `source`: `heuristic` | `blend` (`blend` only at ≥ 30 samples for the row used)
* `empiricalWeight`: `w = clamp(n/100, 0, 0.7)` from that row’s sample count (0 when heuristic)
* `sampleSize`: raw count for the bucket or pooled row consulted (0 if none)
* `pooled`: true when `sampleSize` / probs came from the all-age row (empty or
  sparse age bucket below the blend threshold with a rich pooled row)
* Silent/indecisive history maps to sideways and adjacent equals are merged
  before counting; unknown regimes break the sequence (no fabricated transitions)
* Matrix rebuilds in-memory every 15 minutes per timeframe; history errors
  are negative-cached for the same TTL; timeframe query must be a known TF

### Replay `asOf` (PR-112a)

`GET /api/rankings`, `/api/market/regime`, `/api/market/composite`,
`/api/market/transition`, and `/api/market/regime/history` accept optional
`asOf=<unix seconds>`:

* Candle reads use a historical window ending at `asOf`, then keep only bars
  fully closed at `asOf` (`open + tf ≤ asOf`) — same intent as live
  `GetLastNCandles` dropping the in-progress bar. Live caches are bypassed.
* Regime history is point-in-time: closed predecessors plus the period
  covering `asOf` (shown open) with `currentAge` measured at `asOf`.
  The store read over-fetches `max(limit×3, limit+10)` newest periods before
  filtering — deep scrubber times older than that window may omit the covering
  period (empty / incomplete timeline). Intended for shallow Market Pulse
  scrubbing, not an unbounded historical archive API.
* Transition under `asOf` uses history age at that instant; if age is unknown
  (gap / missing provider), age is `0` — never the live default (`12`).
* `asOf` must be a positive unix second **not after** `now` (`400` if future).
* Replay is **Pro-only**: missing/invalid Bearer → `401`; inactive
  subscription → `403` (`pro required`); subscription check failure → `503`.
  Unauthenticated live reads (no `asOf`) are unchanged.
* Replay traffic is rate-limited separately at **10 req/min/user** (burst 5)
  across all replay endpoints; excess → `429`. Rankings `?mtf=1` is ignored
  when `asOf` is set (MTF overlay is live-only until PR-112b).
* Replay does **not** write badge or transition signals to the signal log.

### Composite index

`GET /api/market/composite`:

* `points` — equal-weight median of log-returns, coverage-aligned (PR-095)
* `volumeWeightedPoints` — quote-volume-weighted mean of the same log-returns
* Stables / wrappers listed under `composite.exclude` in `config.yaml` are skipped

### Sector composites (PR-098)

`GET /api/market/sectors?timeframe=&limit=`:

| Field | JSON | Meaning |
|-------|------|---------|
| Timeframe | `timeframe` | Normalized query timeframe (default `4h`) |
| Market symbol count | `marketSymbolCount` | Contributors to the market baseline after `activePaths` (thin baselines → treat RS cautiously) |
| Sectors | `sectors[]` | Configured sectors with ≥2 contributing symbols; RS-available first (by `rs` desc), then unavailable |
| Sector id | `id` | Configured id (`l1`, `defi`, …) |
| Name | `name` | Display name |
| Symbol count | `symbolCount` | Symbols that contributed to the sector composite |
| Points | `points[{t,v}]` | Clamped sector series on the market-overlap window, **rebased to 100 at the first shared stamp** (same stamps as `return`/`rs` when available). When `rsAvailable` is false, the unclamped own series (already indexed from 100). |
| Return | `return` | When `rsAvailable`: `ln(last/first)` on the clamped overlap; when unavailable: `ln(last/first)` on the sector’s own points |
| RS | `rs` | Sector return − market return over that overlap; **ignore when `rsAvailable` is false** |
| RS available | `rsAvailable` | `true` when ≥2 shared timestamps exist with the market series |

Universe membership comes from the live symbol list; sectors are defined in
`backend/config/sectors.yaml` (each symbol belongs to at most one sector). Unlisted
symbols are not published as a rotation sector. Stables/wrappers matching
`composite.exclude` never enter sector composites. Missing auto-resolved
`sectors.yaml` disables only this route (process still boots); an explicit
`SECTORS_CONFIG_PATH` that is missing, or a malformed catalog, fails boot. The
route is absent (HTTP 404) when the sector catalog is not configured; clients
must treat 404 as feature-disabled.

**Market baseline.** Assembled from the **same** per-symbol bar fetch as sector
partitions (one fan-out).

**Path policy.** VW only when **both** sector and market have a usable
volume-weighted series; otherwise **both** use median.

**Alignment.** `points`, `return`, and `rs` share the intersection of sector and
market timestamps (clamped window), so a sector whose members refreshed one bar
ahead of the majority still gets a reading on the shared prefix — and a sparkline
drawn from `points` matches the published `return`. After clamping, `points` are
rebased to 100 at the first shared stamp so overlays line up with the market
composite. `rsAvailable` is false only when fewer than 2 shared stamps exist.

Redis TTL is `min(3m, timeframe/2)` (`market_sectors_v1`). Empty results are not
cached; non-empty payloads (including `rsAvailable: false` rows) are. Invalid
`timeframe` / `limit` → HTTP 400.

### Multi-timeframe regime stack (PR-099)

`GET /api/symbol/{symbol}/regimes` reads the shared evaluation store (no
candle fetch or rescoring) across the fixed timeframe set `15m`, `1h`, `4h`,
`1d`:

```json
{
  "symbol": "BTCUSDT",
  "frames": [
    {
      "timeframe": "15m",
      "structure": { "trend": 0.72, "sideways": 0.15, "compression": 0.08, "expansion": 0.05 },
      "dominant": "trend",
      "bias": "up",
      "score": 0.72
    }
  ],
  "alignment": 1.0,
  "alignedState": "trend"
}
```

| Field | Meaning |
|---|---|
| `frames[]` | One entry per timeframe that had a fresh, current-algo-version store snapshot for this symbol — a missing, stale, or algo-version-mismatched frame is omitted, not an error |
| `structure` | Same four-way proportional mix as the Market Pulse regime score (`trend`/`sideways`/`compression`/`expansion`, sums to 1) |
| `dominant` | The `structure` component with the highest weight for that frame |
| `alignment` | Share of present `frames` whose `dominant` matches the most common one (0–1); `0` with an empty `frames[]` |
| `alignedState` | That most-common `dominant` when `alignment ≥ 0.75`, else `indecisive` |

### Range trade plan (PR-110)

`GET /api/symbol/{symbol}/plan?timeframe=1h&risk=100` derives a range-reversion
sketch from confirmed swing pivots over the last 110 bars (3-bar pivot rule;
only bars with three neighbors on each side can confirm — so a last-bar wick
cannot move High/Low, but an interior wick that forms a confirmed pivot can)
and Wilder ATR(14) on that same window:

```json
{
  "symbol": "BTCUSDT",
  "timeframe": "1h",
  "low": 100,
  "high": 110,
  "mid": 105,
  "atr": 1,
  "price": 105,
  "longEntry": 100.25,
  "longStop": 99,
  "longTarget": 105,
  "longTargetFull": 109.75,
  "shortEntry": 109.75,
  "shortStop": 111,
  "shortTarget": 105,
  "shortTargetFull": 100.25,
  "riskReward": 3.8,
  "rangeQuality": 0.72,
  "position": 0.5,
  "valid": true,
  "size": 80,
  "shortSize": 80
}
```

| Field | Meaning |
|---|---|
| `low` / `high` / `mid` | Channel from max confirmed swing high / min confirmed swing low |
| `longEntry` / `longStop` / `longTarget` | Long: `Low+0.25×ATR`, `Low−1×ATR`, conservative Mid |
| `longTargetFull` | Aggressive long target `High−0.25×ATR` |
| `short*` | Mirror of long |
| `riskReward` | Mid-target `(longTarget−longEntry)/(longEntry−longStop)`; for this geometry equals `widthATR/2.5 − 0.2` |
| `rangeQuality` | Sideways V5 score (channel-aligned; **not** Lo–MacKinlay MRS) |
| `position` | `(price−low)/(high−low)` clamped to `[0,1]` — 0 at support, 1 at resistance |
| `valid` | `rangeQuality≥0.5` && `(high−low)/atr≥3.5` (width≥3.5 already implies Mid RR≥1.2) |
| `reason` | Present when `valid=false` (`range quality`, `channel width`, `non-positive stop`, …) |
| `size` / `shortSize` | `risk / \|entry−stop\|` when `risk>0` and `valid`; else 0 |

When `valid=false`, entry/stop/target fields and sizes are zeroed; channel
`low`/`high`/`mid`, `atr`, `rangeQuality`, `position`, and `riskReward` may
still be set for diagnostics. Clients must not use `riskReward` (or zeroed
levels) for sizing or trade display when `valid=false` — treat `riskReward` as
read-only context only. Default `timeframe` is `1h`. Invalid
symbol/timeframe/risk → 400. Candle data missing/empty → 422
`DATA_UNAVAILABLE`. Client cancel → 499; deadline exceeded → 504.

`GET /api/rankings?mtf=1` adds `alignment` / `alignedState` (omitempty) to
each row of the **current page only** — a per-row store read, not a
per-request recomputation over the full universe. Rows are read concurrently
under one shared deadline for the page, so a slow store bounds the overlay's
added latency instead of scaling with page size. Both fields are omitted
when the overlay wasn't requested, or when the row has no fresh frames at
all (cold start / store outage — `alignment` would otherwise read a
misleading `0`/`"indecisive"` instead of "no data"), or when the shared
deadline elapsed first. With at least one fresh frame, `alignment` is always
`> 0` (minimum `1/frames.length`), so a present `alignment` field is never a
placeholder zero.

### Watchlist (PR-101)

`GET/PUT/DELETE /api/watchlist` — auth required (`Authorization: Bearer
<device secret>`; unlike most other endpoints this one hard-enforces auth
unconditionally, no unauthenticated fallback). Body for PUT/DELETE:

```json
{ "symbols": ["BTCUSDT", "ETHUSDT"] }
```

* `GET` returns the caller's current watchlist, oldest-added first:
  `{ "symbols": [...] }`.
* `PUT` **replaces** the entire watchlist with `symbols` — the app always
  resyncs its full local (offline-cached) state, not a per-symbol add. A
  symbol already on the list keeps its original add order even across a
  resync. Capped at 50 symbols per user; over the cap → `400`, nothing
  written.
* `DELETE` removes only the given `symbols` from the watchlist (a single
  star-off does not need to resend the full list).
* Both `PUT` and `DELETE` respond with the resulting watchlist, same shape
  as `GET`.

Per-user notification config (`/api/notification/config`) gains
`watchlist_transitions` (bool) and `watchlist_timeframe` (string, e.g.
`"1h"`) — one flag/timeframe pair covers every symbol on the watchlist,
unlike the per-regime timeframes above.

When enabled, the backend scans each watchlisted symbol's PR-099 regime
stack on `watchlist_timeframe` and pushes a notification on three tracked
dominant-regime transitions: `compression → expansion`, `sideways →
trend`, `trend → sideways|compression`. At most one push per `(user,
symbol)` per 4 bars of `watchlist_timeframe`. Payload:

```json
{
  "type": "watchlist_transition",
  "symbol": "BTCUSDT",
  "timeframe": "1h",
  "from": "compression",
  "to": "expansion",
  "bias": "up",
  "score": "0.7200"
}
```

### Alert context (PR-102)

Market, setup, and watchlist pushes also carry a stringified JSON
`context` key in the FCM data map (values are strings only) when at least
one field is available (an empty `{}` is not attached):

```json
{
  "tapeRegime": "trend",
  "tapeBias": "up",
  "tapeConfidence": 0.62,
  "symbolScore": 0.81,
  "rs": 0.032,
  "alignment": 0.75,
  "sparkline": [/* ≤30 closes */]
}
```

* `tapeRegime` / `tapeBias` / `tapeConfidence` come from the market
  summary for the alert's timeframe.
* `symbolScore`, `rs`, and `sparkline` appear only when the alert names a
  symbol (setup / watchlist). Prefer a fresh matching evaluation snapshot
  (`AlgoVersion` + `EvaluationStoreFresh`). Stale or wrong-algo snaps omit
  those store fields; the push still sends. Setup alerts on `1m` / `5m`
  (no store Put — refresher only writes `15m`/`1h`/`4h`/`1d`) fall back to
  the rankings row already scanned to pick the setup, only when that row
  is under the current `AlgoVersion` (rankings Redis keys include the
  version so a scoring bump does not serve pre-change scores).
* `alignment` is independent of the evaluation store — it comes from the
  regime stack (`knownAlignment` on watchlist, or `RegimeStackProvider` on
  setup) and is still attached when the snap is stale or wrong-algo.
* Pointer / presence semantics: a real `0` for score/alignment/confidence
  is encoded and must not be treated as “missing”. Omitted keys mean
  unavailable.
* Sparkline closes are downsampled from the 110-bar evaluation snapshot
  (even spacing, first and last kept) and rounded to 4 decimal places.
* A missing snapshot or empty sparkline does not suppress the push —
  whatever fields are available are still attached.

**Client display:** the sparkline big-picture chart is rendered only on
Android when the app is in the foreground (`FirebaseMessaging.onMessage`
→ local notification). Background / killed delivery uses the system tray
with title/body only (the `context` JSON is still available to the app
on open). iOS has no rich attachment in this PR.

### Rankings relative strength (PR-096)

`GET /api/rankings` includes response-level and per-row fields:

| Field | JSON | Meaning |
|-------|------|---------|
| RS available | `rsAvailable` | `true` only when a usable tape scored ≥1 row |
| Effective sort | `sort` | May fall back from `leaders`/`laggards` → `total` when `rsAvailable` is false |
| Requested sort | `requestedSort` | Query `sort` before fallback |
| Relative strength | `rs` | Log excess return on that symbol’s timestamp overlap with the tape (omitted when unset). Overlap must be at least `max(2, min(20, ceil(n/2)))` shared stamps (`n` = tape length) — not a universe-wide first/last timestamp |
| Beta | `beta` | OLS slope on the **same** aligned return pairs (omitted when unset) |
| RS rank | `rsRank` | Percentile among scored RS rows only (omitted when unset) |

`percentile` remains position after the effective sort (score/total/volume/…), not `rsRank`.

Tape window for RS is `metrics.CompositeTapeWindow` (110) — the same limit as Market
Pulse — so both share the Redis composite key. If `OVERVIEW_SPARKLINE_PRECISION` is
set below the overlap floor (20 on a 110-stamp tape), every row stays unscored,
`rsAvailable` stays false, and leaders/laggards always fall back to `total`.

Sort modes (query `sort=`):

* `leaders` — highest `rs` first (unscored / excluded names last)
* `laggards` — lowest `rs` first (unscored / excluded names last)

Names matching `composite.exclude` are left unscored for RS. Show the RS chip only
when `rsAvailable && rs != null`. A real `rs: 0` (matched the tape) still shows;
omitted `rs` (excluded / short overlap) and `rsAvailable: false` hide the chip.

Redis rankings (`rankings_v2`) skip `SET` on transient tape failures, incomplete
candle coverage, and capable-but-unaligned zero-scored boards (any sort), and on
leaders/laggards when `rsAvailable` is false (fallback or empty board). Stable
unscored cases (precision below the overlap floor / all excluded / all short
history) and intentional RS-off still cache non-RS sorts so full-universe
scoring is not repeated every request.

`requestedSort` is the query `sort` before any leaders/laggards → `total` fallback.

---

## Scorecards (PR-092)

Reliability of past badge / setup / regime / transition calls, graded by the
PR-091 outcome evaluator. Only rows with a resolved outcome enter the
denominator; administrative rules (`unsupported`, `invalid`,
`insufficient_context`, `path_unavailable`) are excluded in SQL.

Aggregates are computed in SQLite (counts / hits / sum return per score
decile) — no row-hydrate cap. Summary is grouped per `(kind, label)` with no
shared global `LIMIT`. Baselines for summary chips are derived from that same
grouping (`(kindHits − labelHits) / (kindN − labelN)`).

### `GET /api/scorecards`

Query params:

* `kind` (required) — allowlisted: `badge` | `setup` | `regime` | `transition`
* `label` (required) — e.g. `trend_up`, `sideways`, `regime:trend`
* `timeframe` (optional) — canonical TF; empty = all timeframes mixed
* `since` (optional) — relative (`30d`, `7d`, `24h`) or RFC3339; default `30d`.
  Relative tokens must be exact (`30d`, not `30dgarbage`). Cached under the
  token itself (not `time.Now().Unix()`), so `since=30d` hits for ~10 minutes.
  Absolute RFC3339 / RFC3339Nano uses the exact UTC instant for both the SQL
  window and the Redis key (no 10-minute absolute bucket). Instants older than
  10 years are rejected, the same cap as relative durations. Timeframe is
  canonicalized (`1H`→`1h`).

Response:

```json
{
  "kind": "badge",
  "label": "trend_up",
  "timeframe": "1h",
  "since": "2026-08-22T00:00:00Z",
  "sinceRaw": "30d",
  "total": 412,
  "hits": 239,
  "hitRate": 0.58,
  "baseline": 0.49,
  "buckets": [
    { "lo": 0.0, "hi": 0.1, "n": 12, "hits": 3, "hitRate": 0.25, "avgReturn": -0.01 },
    { "lo": 0.1, "hi": 0.2, "n": 20, "hits": 8, "hitRate": 0.4, "avgReturn": 0.0 }
  ]
}
```

`buckets` are score deciles `[0,0.1) … [0.9,1.0]`. `baseline` is the
deterministic hit rate of all gradable rows of the **same kind** (and
timeframe) in the window **excluding the scored label**. When there is no
comparison set (sole label for that kind), `baseline` is JSON `null` — not
`0`. Numeric `0` means the comparison set graded with zero hits. Hit rate and
baseline share one Redis card (~10 minutes); there is no separate longer
baseline TTL. Errors return JSON `{"error":"..."}`.

### `GET /api/scorecards/summary`

Query params: `timeframe` (optional), `since` (optional, default `30d`).

Response:

```json
{
  "timeframe": "1h",
  "since": "2026-08-22T00:00:00Z",
  "sinceRaw": "30d",
  "items": [
    { "kind": "badge", "label": "trend_up", "hitRate": 0.58, "baseline": 0.49, "n": 412 }
  ]
}
```

One row per `(kind, label)` for UI reliability chips. `since` is frozen with
the cached payload (not re-stamped from wall clock on a hit). Same exclusion /
nullable-baseline rules as the full scorecard.

---

## Score distribution (PR-094)

Sampled calculator scores, kept so Track E can place a live score in the
recent distribution. `LoggingScoreCalculator` records a fraction of `Score()`
calls (`PC_SCORE_SAMPLE_RATE`, default `0.1`; non-finite values also use `0.1`)
into SQLite (`PC_SCORE_SAMPLE_DB`, default `./score_samples.sqlite`) instead of
printing them. Samples are queued and written in batches; a full queue drops
the sample. Rows older than the retention window (`PC_SCORE_SAMPLE_RETENTION`,
`90d` or a Go duration, default 90 days) are deleted when the process starts
its retention loop and about once a day. Queries also ignore anything outside
that window. A percentile query uses the most recent 100000 retained rows
for that calculator and timeframe. `calculator` is the exact `Name()` string.

### `GET /api/debug/score-distribution`

Registered only when `PC_DEBUG_ENDPOINTS=1` and the sample DB opened. It is
not a route on the public API. The process listens for it on `PC_DEBUG_ADDR`
(default `127.0.0.1:8082`). The host must be a loopback IP; `0.0.0.0`, an
empty host, and public addresses are refused. A reverse proxy that forwards
to the public API port does not reach this socket unless it is configured to
dial the debug address itself. Not an app surface.

Query params:

* `calculator` (required) — exact calculator `Name()`, e.g. `Sideways Consistency`
* `timeframe` (required) — canonical TF (`1H` is accepted as `1h`)

Response is the nearest-rank sample at p5, p10, … p95 (19 values):

```json
{
  "calculator": "Sideways Consistency",
  "timeframe": "1h",
  "distribution": [0.05, 0.10, 0.14, 0.18, 0.22, 0.27, 0.31, 0.36, 0.41, 0.47, 0.52, 0.58, 0.63, 0.69, 0.74, 0.80, 0.85, 0.91, 0.96]
}
```

A known calculator (any retained sample, on any timeframe) with no rows for
the requested timeframe → `404`. An unknown calculator → `400` with
`calculators` listing those names. Missing params → `400`. Other methods →
`405`. Errors are JSON `{"error":"..."}`.

