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
composite tape** (volume-weighted when available, else median) with the same
per-chart calculators as rankings.

Additive fields:

* `regimeSource`: `composite_volume_weighted` | `composite_median` | `participation`
* `structure` (state endpoint): tape score mix
* regime `scores`: tape structure (not token vote share)

### Participation (metrics)

`metrics.trendBreadth` / `sidewaysBreadth` / `compressionBreadth` /
`expansionBreadth` remain **per-token participation** averages. UI copy must
not present these as the headline regime.

### Composite index

`GET /api/market/composite`:

* `points` — equal-weight median (unchanged)
* `volumeWeightedPoints` — quote-volume-weighted mean (additive)

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
(default `127.0.0.1:8081`). The host must be a loopback IP; `0.0.0.0`, an
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
  "distribution": [0.12, 0.18, 0.25]
}
```

A known calculator (any retained sample, on any timeframe) with no rows for
the requested timeframe → `404`. An unknown calculator → `400` with
`calculators` listing those names. Missing params → `400`. Other methods →
`405`. Errors are JSON `{"error":"..."}`.

