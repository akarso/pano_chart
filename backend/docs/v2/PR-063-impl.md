## PR-063: feat(market): trend health layer + effective aggregation + UI & notification improvements

### Summary
Introduces a **trend health / integrity layer** that measures how "healthy" a trend actually is per token, then aggregates this into market-level metrics. This fixes the mismatch between structural trend detection (correct) and user perception (trend already breaking down).

### Backend Changes

**New file: `application/market/health.go`**
- `ComputeTrendHealth(state, price, recentHigh, recentLow, atr, recentReturn) float64` — per-token health score (0–1)
  - Uptrend: `1 - clamp(drawdown/ATR, 0, 1)` where drawdown = recentHigh - price
  - Downtrend: `1 - clamp(bounce/ATR, 0, 1)` where bounce = price - recentLow
  - Crash penalty: if recentReturn < -1.5 → health × 0.3
  - Non-trend states return 0
- `BuildMarketLabel(trendPrevalence, effectiveTrend) string` — produces human-readable label:
  - prevalence > 0.6 + effective > 0.5 → "Strong trend"
  - prevalence > 0.6 + effective > 0.3 → "Trend weakening"
  - prevalence > 0.6 + effective ≤ 0.3 → "Trend breaking down"
  - prevalence > 0.4 → "Mixed conditions"
  - else → "No clear trend"

**`domain/evaluation_snapshot.go`** — added `RecentHigh`, `RecentLow`, `RecentReturn` fields

**`domain/market/market_state.go`** — added `EffectiveTrend`, `BreakdownRate`, `Label` to `Summary`

**`application/market/market_state_service.go`** — `Calculate()` now:
1. Identifies trending tokens (trend is dominant regime)
2. Computes per-token health via `ComputeTrendHealth`
3. Aggregates `EffectiveTrend` (avg health / total tokens) and `BreakdownRate` (fraction of trending tokens with health < 0.4)
4. Generates `Label` via `BuildMarketLabel`

**`adapters/http/market_handler.go`** — response DTO now includes `effectiveTrend`, `breakdownRate`, `label`

**`application/notifications/scheduler.go`**:
- Legacy broadcast: trend state uses `Summary.Label` when available (falls back to "Market trending today")
- Per-user path: uptrend candidate uses `Summary.Label` instead of hardcoded "Uptrend"

### Frontend Changes

**`market_state_data.dart`** — added `effectiveTrend`, `breakdownRate`, `label` fields (backward-compatible defaults)

**`market_pulse_screen.dart`**:
- State card shows health label next to regime name: `TREND · Strong`, `TREND · Weakening ↓`, `TREND · Breaking ↓↓`
- Color-coded: green (strong), orange (weakening), red (breaking)
- Icons: `trending_up` / `trending_flat` / `trending_down`

### Tests
- 8 unit tests for `ComputeTrendHealth` (uptrend/downtrend at various drawdowns, crash penalty, zero ATR, non-trend states)
- 5 unit tests for `BuildMarketLabel` (all threshold tiers)
- 4 integration tests for `Calculate()` health aggregation (populated fields, no-price fallback, empty evals, breakdown rate)
- All 102 market tests pass, all 44 notification tests pass, all HTTP tests pass

### Backward Compatibility
- New fields default to zero values — no breaking API changes
- Existing regime detection logic untouched
- Health layer is purely additive
