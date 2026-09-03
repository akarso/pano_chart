# Pano Charts — Project Summary

**Pano Charts** is a crypto market intelligence platform for Android that combines a 150-token technical-analysis screener with market-level regime detection, transition probability forecasting, per-token risk/behavior scoring, and real-time auto-refresh. Built with **Flutter** (frontend) and **Go** (backend), following strict **hexagonal architecture** (ports & adapters).

---

## Backend (Go 1.24)

**Architecture:** Domain → Application (use cases + port interfaces) → Adapters (HTTP handlers, infra). All wired in a single composition root (`cmd/api/main.go`).

### Core Domain

**Domain models:** `Symbol`, `Timeframe`, `Candle`, `CandleSeries` (sorted/deduped/gap-detected), `EvaluationSnapshot` (algo v5), `Event`, `NewsArticle`, `Purchase`, `Subscription`, `PaymentVerificationResult`.

**Scoring engine** (`domain/scoring/`): MetaScore system that composes subscores via configurable modes (weighted additive, multiplicative, hybrid-structural). Calculators include:

- **SidewaysV5** — structural equilibrium via channel structure, oscillation quality, drift control, volatility metrics, extrema detection, per-timeframe ideal ATR ranges
- **Compression** — width/volatility contraction, boundary convergence, directional pressure (squeeze detection)
- **Breakout** — boundary violation, close conviction, volume/volatility expansion (directional up/down scores)
- **TrendPredictability** — linear regression R² × normalized slope
- **Gain/Loss** — simple close-to-close return

Each evaluation produces: SidewaysScore, CompressionScore, BreakoutUp/Down, TrendScore, Bias, ChannelType, Price, ATR, Volume.

**Confidence pipeline** (post-evaluation): Engine.Evaluate → ApplyContextModifier → ApplyMarketModifier → ComputeConfidence → ApplyBreakoutConfidence. The confidence score feeds back into breakout probability, creating setup-level signal quality gating.

### v2 — Market Intelligence Layer

**Market State & Breadth** (`domain/market/`, `application/market/`): classifies overall market structure as sideways, compression, breakout, or trend with confidence scores and per-regime breadth breakdown. Reuses existing evaluations — zero new data fetches.

**Composite Index** (`application/market/metrics/`): normalized price index (median of per-symbol series) for the Market Pulse chart. Median resists crypto outlier distortion. ~30k float ops for 150 symbols × 200 candles (<1ms).

**Regime Detection** (`application/market/metrics/`): single-pass aggregator combining volatility expansion (ATR_short / ATR_long), dispersion (mean absolute deviation of returns), and compression/trend breadth to classify the market regime (compression → sideways → trend → expansion).

**Transition Probability Engine** (`application/market/transition/`): heuristic model that calculates regime transition probabilities from compression breadth, volatility slope, market dispersion, and regime age. Core signal: breakout pressure = `compressionBreadth × (1 + volatilitySlope) × regimeAgeFactor`, clamped 0–1.

**Regime History Tracker** (`application/market/regimehistory/`): SQLite-backed persistence of regime periods (start, end, duration in candles). Feeds real regime age into the transition engine instead of hardcoded values.

### v2 — Per-Token Analysis

**Setup Quality Engine** (`application/setups/`): scores three independent trade setup types — compression breakout, trend continuation, range reversion — with isolated evaluators that avoid mixing incompatible metrics.

**Unified Confidence Score** (`application/setups/confidence.go`): multi-factor 0–1 score answering "how much should I trust this setup right now?" Combines trend health, market health, crowding (inverted), and volatility fit using regime-specific weights — trend regimes emphasize trend health (40%), compression regimes emphasize volatility fit (30%), sideways regimes emphasize crowding (30%). Displayed as a color-coded dot (green >0.75, yellow >0.55, red ≤0.55) next to the setup quality score.

**Confidence-Aware Breakout Probability** (`application/setups/breakout.go`): adjusts raw breakout-up and breakout-down scores by the unified confidence level. Formula: `probability = baseScore × (0.5 + 0.5 × confidence)`. High confidence passes through the raw score unchanged; low confidence halves it. An additional floor penalty applies when confidence < 0.3, and a directional bias fix penalises up-breakouts when trend health is weak (< 0.4).

**Fragility Score** (`application/risk/`): measures position crowding / unwind risk from funding extremeness (25%), OI expansion (30%), long/short imbalance (20%), and liquidation proximity (25%). Exposes directional side and squeeze risk type (long_squeeze / short_squeeze).

**Retail Behavior Engine** (`application/behavior/`): translates positioning signals into four behavioral dimensions — greed, fear, patience, panic — with soft normalization and deterministic summary text. Market-wide aggregation averages all 150 tokens.

**Signal Labels** (`application/insights/`): deterministic rule-based engine that picks max 3 human-readable signal labels per token (e.g. "Breakout pressure building", "Crowded longs") with strength and type (info / warning / opportunity).

### Volatility Aggregation Pipeline

**Offline CLI** (`cmd/vol_aggregate/`): fetches 150 days of 1-minute candles from Binance (with SQLite cache for incremental updates), computes ATR-normalized move statistics per minute-of-day and minute-of-week, derives all timeframe buckets (1m/5m/15m/1h/4h), and aggregates weekly data into 7 day-of-week buckets for 1d charts. Output: JSON file with `FullResult{Intraday, Weekly}`.

### Data Sources & Caching

Binance (candles, symbol universe, 24h volume), FinanceFlow (macro events), Redis (candles 5min, overview 45s, rankings 60s, Fear & Greed 6h, market state 45s, composite 30–60s), SQLite (payments, regime history, volatility candle cache).

### API Routes

| Endpoint | Description |
|---|---|
| `/api/overview` | Paginated sparkline grid (rankings + scores) |
| `/api/rankings` | Sorted symbol rankings |
| `/api/symbol/{symbol}` | Symbol detail |
| `/api/v1/candles` | OHLCV candle series |
| `/api/v1/events` | Macro economic events |
| `/api/v1/fear-greed` | Fear & Greed index |
| `/api/news` | News articles |
| `/api/payments/verify` | Google Play purchase verification |
| `/api/subscription/status` | Subscription status check |
| `/api/sol/price` | SOL price for crypto payments |
| `/api/market/state` | Market state + breadth (v2) |
| `/api/market/composite` | Composite price index (v2) |
| `/api/market/regime` | Regime detection + metrics (v2) |
| `/api/market/regime/history` | Regime period history (v2) |
| `/api/market/transition` | Transition probabilities (v2) |
| `/api/token/{symbol}/setup` | Setup quality scores (v2) |
| `/api/token/{symbol}/fragility` | Fragility / crowding score (v2) |
| `/api/token/{symbol}/behavior` | Retail behavior dimensions (v2) |
| `/api/volatility` | Intraday & day-of-week volatility profiles |

---

## Frontend (Flutter/Dart)

**Main screen** — `OverviewWidget`: scrollable sparkline grid with settings overlay, sort options, timeframe switching, favourites, stablecoin filtering, columns toggle, normalized/hi-res sparklines, flash-dot glow on updated tickers. Eager-loads all ~150 symbols at startup.

**Auto-refresh (Pro)** — staggered sparkline animation with 20ms delay between symbols + 5s margin. Detail view and bubble map auto-refresh at timeframe-dependent intervals (1m → 10s, 5m → 1m, 15m → 3m, 1h → 10m, 4h → 15m, 1d → 1h). Macro events re-fetched after 15m on detail chart. `AutoRefreshTimer` class uses fire-and-reschedule pattern to prevent overlapping cycles.

**Detail / Chart screen** — full interactive candlestick chart with:

- Custom `CandlePainter`, `VolumePainter`, `OscillatorPainter`
- **Volatility/Activity overlay** — in-chart panel showing intraday or day-of-week activity patterns derived from 150 days of 1-minute candle history. Adaptive percentile-based coloring (green/yellow/red) shifts with zoom/pan. Timeframe-aware: intraday charts show minute-of-day patterns, 1d chart shows day-of-week seasonality.
- Crosshair overlay with price/time readout
- Indicator panel (configurable)
- Trade action buttons with exchange selection
- Event overlay (past + future markers)
- v2 panels: setup quality (with confidence dot), fragility gauge, retail behavior card, confidence-adjusted breakout bars

**Future Events Projection Zone** — the chart extends beyond the last candle to show upcoming macroeconomic events. Per-timeframe projection windows (e.g. 1h → 24h ahead, 4h → 2d ahead, 1d → 2d ahead) create virtual candle slots. Future markers render as dashed lines at 70% muted alpha.

**Other features:**

- **Bubble Map** — accelerometer-driven bubble visualization by market cap, auto-refreshing (pro)
- **Macro Events** — calendar with country/impact filtering, scroll & highlight
- **Fear & Greed** — market sentiment gauge dialog
- **News** — article list with detail view
- **Billing** — Google Play in-app subscription (`panocharts-pro`, $4.99/month), `BillingManager` lifecycle (init → purchase → verify server-side → activate), restore flow, `UpgradeScreen` UI, SOL crypto payment option
- **Pro tier gating** — auto-refresh, stale-data banner suppression; free tier retains manual refresh with flash-dot animation

**Composition root** (`core/di/composition_root.dart`): factory methods for all ViewModels and services.

---

## Webpage

Static site with dark theme + teal accents. `index.html` (About + Contact), separate pages for `blog.html`, `help.html`, `news.html`. Articles are `.md` files loaded on click via vanilla JS (`md-loader.js`) — fetched, parsed to HTML, rendered inline.

---

## Governance

- **142 test files** (63 backend, 79 frontend), 617+ tests passing
- **87 PR docs** tracking incremental delivery (v1: PR-000 through PR-042, v2: PR-043 through PR-052)
- TDD, small focused PRs, deterministic tests, static analysis enforced
- `AGENTS.md` / `BACKEND.md` / `FRONTEND.md` / `COMMON.md` define cross-repo collaboration rules
