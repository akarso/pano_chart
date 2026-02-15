# Pano Chart — Project Summary for Frontend Development

> Generated: 2026-02-15
> Purpose: Serve as a comprehensive base for frontend development continuation.

---

## 1. Project Overview

**Pano Chart** is a crypto market visualization app. Users see many market charts on a single scrollable screen, quickly compare instruments (sideways action, volatility, trend), and drill into individual symbols for detailed candlestick analysis.

**Architecture**: Monorepo with Go backend (hexagonal architecture) and Flutter frontend (layered, feature-oriented). Shared contracts in `COMMON.md`.

---

## 2. Backend API Surface (Production)

The backend is effectively **MVP-complete**. All endpoints are live via `cmd/api/main.go` on `:8080`. Redis caching decorators wrap overview and rankings use cases.

### 2.1 Endpoint Map

| Method | Path | Purpose | Status |
|--------|------|---------|--------|
| `GET` | `/health` | Liveness probe | ✅ Live |
| `GET` | `/api/v1/candles` | Raw candle series for one symbol | ✅ Live |
| `GET` | `/api/overview` | Ranked symbols with sparklines | ✅ Live |
| `GET` | `/api/rankings` | Paginated ranked symbol list | ✅ Live |
| `GET` | `/api/symbol/{symbol}` | Symbol detail with candles + scores | ✅ Live |

### 2.2 Endpoint Details

#### `GET /api/v1/candles`

**Purpose**: Fetch raw OHLCV candle data for a single symbol.

| Param | Type | Required | Notes |
|-------|------|----------|-------|
| `symbol` | string | ✅ | e.g. `BTCUSDT` |
| `timeframe` | string | ✅ | `1m`, `5m`, `15m`, `1h`, `4h`, `1d` |
| `from` | string | ✅ | RFC 3339 timestamp |
| `to` | string | ✅ | RFC 3339 timestamp |

**Response**:
```json
{
  "symbol": "BTCUSDT",
  "timeframe": "1h",
  "candles": [
    { "timestamp": "2026-02-15T00:00:00Z", "open": 42000.0, "high": 42100.0, "low": 41900.0, "close": 42050.0, "volume": 1234.56 }
  ]
}
```

#### `GET /api/overview`

**Purpose**: Top-ranked symbols with sparkline data for the overview grid.

| Param | Type | Required | Default | Notes |
|-------|------|----------|---------|-------|
| `timeframe` | string | ✅ | — | Candle aggregation interval |
| `limit` | int | ❌ | 10 | Number of results; must be > 0 |

**Response**:
```json
{
  "timeframe": "1h",
  "count": 10,
  "precision": 30,
  "results": [
    { "symbol": "BTCUSDT", "totalScore": 2.75, "sparkline": [42000.0, 42100.0, 41900.0] }
  ]
}
```

#### `GET /api/rankings`

**Purpose**: Paginated, sortable scored symbol list.

| Param | Type | Required | Default | Notes |
|-------|------|----------|---------|-------|
| `timeframe` | string | ✅ | — | |
| `sort` | string | ❌ | `total` | `total`, `gain`, `sideways`, `trend`, `volume` |
| `page` | int | ❌ | 1 | Page number, ≥ 1 |
| `pageSize` | int | ❌ | 30 | Clamped 1–100 |

**Response**:
```json
{
  "timeframe": "1h",
  "sort": "total",
  "page": 1,
  "pageSize": 30,
  "totalItems": 50,
  "totalPages": 2,
  "results": [
    {
      "symbol": "BTCUSDT",
      "totalScore": 2.75,
      "scores": { "sideways": 0.9, "trend": 0.85, "gainLoss": 1.0 },
      "volume": 123456.78
    }
  ]
}
```

#### `GET /api/symbol/{symbol}`

**Purpose**: Full detail for a single symbol — candles + computed scores.

| Param | Type | Required | Default | Notes |
|-------|------|----------|---------|-------|
| `{symbol}` | path | ✅ | — | e.g. `/api/symbol/BTCUSDT` |
| `timeframe` | string | ✅ | — | |
| `limit` | int | ❌ | 200 | Max 1000 |

**Response**:
```json
{
  "symbol": "BTCUSDT",
  "timeframe": "1h",
  "candles": [
    { "openTime": "2026-02-15T00:00:00Z", "open": 42000.0, "high": 42100.0, "low": 41900.0, "close": 42050.0, "volume": 1234.56 }
  ],
  "stats": {
    "totalScore": 2.75,
    "scores": { "sideways": 0.9, "trend": 0.85, "gainLoss": 1.0 }
  }
}
```

### 2.3 Error Shape (all endpoints)

```json
{ "error": { "code": "INVALID_SYMBOL", "message": "..." } }
```

Error codes: `INVALID_SYMBOL`, `INVALID_TIMEFRAME`, `RATE_LIMITED`, `INTERNAL_ERROR`.

### 2.4 Backend Scoring Logic

Three scoring calculators, each returning a float64:

| Calculator | What it measures | Score interpretation |
|---|---|---|
| **GainLoss** | `(last.close - first.close) / first.close` | Net % gain/loss over series |
| **TrendPredictability** | Linear regression: `slopeNorm × R²` | Higher = strong, predictable trend |
| **SidewaysConsistency** | NDR × RSS × ODS composite | Higher = range-bound, oscillatory action |

`TotalScore = Σ(score × weight)`. Symbols ranked descending by total score, then alphabetically.

---

## 3. Frontend — Current State

### 3.1 Technology Stack

| Concern | Choice |
|---------|--------|
| Framework | Flutter (Dart) |
| Min SDK | Dart ≥2.18.0, Flutter ≥3.0.0 |
| HTTP | `package:http` (^0.13.0) |
| State Management | Vanilla `StatefulWidget` / `setState` |
| DI | Manual constructor injection |
| Charts | Custom `CustomPainter` implementations |
| Testing | Fakes (no mockito/mocktail) |
| Dependencies | Minimal — `http`, `flutter_lints` only |

### 3.2 Architecture

```
lib/
├── main.dart                        ← barrel re-export
├── app/                             ← MaterialApp + Router
├── bootstrap/                       ← main() entry + bootstrapApp()
├── core/
│   ├── config/                      ← AppConfig (apiBaseUrl, flavor)
│   └── di/                          ← AppComponent + CompositionRoot
├── domain/                          ← Value objects (AppSymbol, Timeframe, SeriesViewMode)
├── features/
│   ├── candles/                     ← Data layer (port, DTOs, use case, HTTP adapter)
│   ├── overview/                    ← Grid screen (widget, mini charts, line renderer)
│   └── detail/                      ← Detail screen (candle renderer, full chart)
├── infrastructure/                  ← (empty / unused)
└── models/                          ← (empty / unused)
```

**Layers** (dependency direction → downward only):
1. **Presentation** — Widgets (`OverviewWidget`, `DetailScreen`)
2. **ViewModels** — `OverviewViewModel` (currently a stub)
3. **Application** — Use cases (`GetCandleSeries`)
4. **Infrastructure** — HTTP adapters (`HttpCandleApi`)

### 3.3 What's Implemented (22 files, 13 test files)

| Component | Status | Notes |
|-----------|--------|-------|
| **Project scaffold** | ✅ Complete | Bootstrap, routing shell, config, DI |
| **Domain objects** | ✅ Complete | `AppSymbol`, `Timeframe`, `SeriesViewMode` |
| **CandleApi port** | ✅ Complete | Interface + request/response DTOs |
| **HttpCandleApi adapter** | ✅ Complete | Talks to `/api/v1/candles`, handles errors |
| **GetCandleSeries use case** | ✅ Complete | Delegates to CandleApi |
| **CompositionRoot** | ✅ Complete | Wires http.Client → HttpCandleApi → GetCandleSeries |
| **LineSeriesChartRenderer** | ✅ Complete | Close-price polyline for overview |
| **CandleSeriesChartRenderer** | ✅ Complete | OHLC bodies + wicks for detail |
| **OverviewWidget** | ✅ Functional | Grid with column selector, timeframe dropdown, mini charts |
| **DetailScreen** | ✅ Functional | Candle chart, symbol name, % change, favourite toggle |
| **OverviewViewModel** | ⚠️ Stub | Returns empty data; not connected to use case |
| **App Router** | ⚠️ Placeholder | Only maps `'/'` → empty placeholder; overview/detail not wired |
| **Rankings feature** | ❌ Missing | Backend API exists, no frontend code |
| **Symbol Detail API** | ❌ Missing | Backend `/api/symbol/{symbol}` exists, frontend only uses `/api/v1/candles` |
| **Overview API** | ❌ Missing | Backend `/api/overview` exists, frontend fetches raw candles instead |
| **Error/retry states** | ❌ Missing | Only partial: "No data" text, `HttpCandleApiException` exists but not surfaced |
| **Offline handling** | ❌ Missing | |
| **Performance tuning** | ❌ Missing | |

### 3.4 Key Wiring Gap

The biggest integration issue: **the app doesn't actually show the feature screens**.

```
main() → bootstrapApp(config) → AppComponent → App → AppRouter → _RootPlaceholder ← dead end
                                                                      ↕ NOT CONNECTED
CompositionRoot → GetCandleSeries → OverviewWidget → DetailScreen   ← live but unreachable
```

`AppComponent.createApp()` creates the `App` widget, but `AppRouter` maps `'/'` to a placeholder `Scaffold`, not to `OverviewWidget`. The `CompositionRoot` that actually wires the HTTP client is never called from the bootstrap path.

### 3.5 Rendering Strategy

Two renderer implementations behind a `SeriesChartRenderer` interface:

| Renderer | Purpose | Technique |
|----------|---------|-----------|
| `LineSeriesChartRenderer` | Overview mini-charts | Close prices → normalized polyline via `CustomPainter` |
| `CandleSeriesChartRenderer` | Detail screen | OHLC bodies + wicks, green/red coloring via `CustomPainter` |
| `MiniChartRenderer` (inline) | Overview grid items | Embedded `CustomPainter` drawing mini OHLC bars |

---

## 4. Gap Analysis: Backend → Frontend Feature Mapping

| Backend Feature | Endpoint | Frontend Status |
|---|---|---|
| Raw candle fetch | `/api/v1/candles` | ✅ Fully wired (HttpCandleApi) |
| Overview (ranked sparklines) | `/api/overview` | ❌ Not consumed — frontend fetches raw candles instead |
| Rankings (paginated, sortable) | `/api/rankings` | ❌ No frontend feature exists |
| Symbol detail (candles + scores) | `/api/symbol/{symbol}` | ❌ Not consumed — frontend uses `/api/v1/candles` |
| Scoring display (sideways, trend, gain/loss) | In overview/rankings/detail responses | ❌ Not surfaced in UI |

---

## 5. Suggested Next Steps (Priority Order)

### Phase A — Wire Existing Screens to Backend APIs

1. **Connect CompositionRoot to App bootstrap** — so the real screens are reachable
2. **Switch Overview to `/api/overview` endpoint** — consume sparklines + scores from backend instead of fetching individual candle series; add new OverviewApi port + HttpOverviewApi adapter + DTOs
3. **Switch Detail to `/api/symbol/{symbol}` endpoint** — consume candles + scores; add SymbolDetailApi port + adapter
4. **Wire OverviewViewModel to real use case** — replace stub with actual data fetching + state management

### Phase B — New Features

5. **Rankings screen** — new feature folder consuming `/api/rankings`; paginated list with sort controls (total, gain, sideways, trend, volume)
6. **Score visualisation in UI** — display sideways/trend/gainLoss scores as badges or indicators on overview cards and detail screen
7. **Route table completion** — wire `/`, `/rankings`, `/symbol/:id` into `AppRouter`

### Phase C — Hardening

8. **Error states** — map `HttpCandleApiException` + new API errors to user-facing UI states (retry button, error messages)
9. **Loading/empty states** — skeleton loaders or shimmer effects
10. **Offline/retry** — network detection, retry policies
11. **Performance** — lazy loading, pagination awareness, widget `const`-ness audit

---

## 6. Shared Contracts Reference

From `COMMON.md`:

- **Symbol**: uppercase, `[A-Z0-9\-_]`, case-insensitive
- **Timeframe**: `1m`, `5m`, `15m`, `1h`, `4h`, `1d`
- **Candle**: `timestamp` (UTC epoch ms), OHLCV, invariants enforced
- **Error shape**: `{ "error": { "code": "...", "message": "..." } }`
- Error codes: `INVALID_SYMBOL`, `INVALID_TIMEFRAME`, `RATE_LIMITED`, `INTERNAL_ERROR`

---

## 7. Testing Landscape

### Backend
Full test coverage across domain, application, ports, adapters, and composition. Tests use fakes and in-memory implementations.

### Frontend (13 test files)

| Layer | Tested | Approach |
|-------|--------|----------|
| Domain DTOs | ✅ JSON round-trip, validation, immutability | Unit tests |
| Use case | ✅ Delegation, error propagation | Fake CandleApi |
| HTTP adapter | ✅ URL building, status codes, parsing | Fake http.Client |
| Composition Root | ✅ Wiring verification | Fake http.Client |
| OverviewWidget | ✅ Loading, grid items, empty state | Fake use case |
| DetailScreen | ✅ Symbol text, favourite toggle | Fake ViewModel |
| Renderers | ✅ Math verification, edge cases | Unit + widget tests |

**Testing pattern**: Hand-written fakes, no mocking libraries. Tests verify behavior, not implementation.

---

## 8. Dev Environment & Commands

```bash
# Frontend
cd frontend
flutter pub get
dart format --set-exit-if-changed .
flutter analyze
flutter test

# Backend
cd backend
go test ./...
go build ./...
```

Backend server: `cd backend/cmd/api && go run .` (listens on `:8080`)

Frontend config: `AppConfig(apiBaseUrl: 'http://localhost:8080', flavor: 'dev')`
