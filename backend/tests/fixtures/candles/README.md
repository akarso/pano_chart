# Golden candle fixtures (PR-088)

Pinned OHLCV snippets used by scoring and tape regression tests. Each file is
one **regime archetype** with exactly **`RequiredBars` (110)** candles
(matches production `sparklinePrecision` default in `cmd/api/main.go`).

## Format

```json
{
  "symbol": "BTCUSDT",
  "timeframe": "15m",
  "archetype": "clean_uptrend",
  "source": "synthetic",
  "recorded": "2026-09-19",
  "candles": [
    {"t": 1717200000, "o": 50000, "h": 50100, "l": 49900, "c": 50050, "v": 1000}
  ]
}
```

| Field | Meaning |
|---|---|
| `t` | Bar open time, Unix seconds UTC |
| `o/h/l/c/v` | Open, high, low, close, quote volume |
| `source` | `synthetic` (deterministic generator) or `binance` (live snippet) |
| `recorded` | ISO date the fixture was written |
| `archetype` | Must match the filename stem |

## Archetypes

| File | Intent |
|---|---|
| `clean_uptrend` | Smooth +~10% grind |
| `messy_uptrend` | +8–12% with mid-window chop (PR-084 case) |
| `clean_downtrend` | Smooth −~10% grind |
| `tight_range` | ~1% oscillating channel |
| `wide_range` | ~4% constant-width channel (not compressing) |
| `compression_pre_breakout` | ATR contracts into a coil |
| `breakout_up_with_volume` | Swing channel → piercing bar + volume |
| `failed_breakout` | Pierce then re-enter the channel |
| `v_reversal` | Sharp sell-off then recovery |
| `flat_dead` | Near-flat price, thin volume (silent) |

## Measured score vectors (current code)

Approximate fingerprints from the domain calculators (Trend = |Trend
Predictability|). Use this table when judging whether a fixture still matches
its name after a calculator change.

| Fixture | Trend | Sideways | Compression | Breakout | Gain/Loss | Tape (T/S/state) |
|---|---:|---:|---:|---:|---:|---|
| clean_uptrend | ~1.00 | 0 | 0 | 0 | ~0.10 | T≫S / trend |
| messy_uptrend | ~0.98 | 0 | 0 | 0 | ~0.10 | T≫S / trend |
| clean_downtrend | ~1.00 | 0 | 0 | 0 | ~−0.10 | T≫S / trend, bias down |
| tight_range | 0 | ~0.68 | 0 | 0 | ~0 | S / sideways |
| wide_range | 0 | ~0.76 | 0 | 0 | ~0 | S / sideways |
| compression_pre_breakout | 0 | 0 | ~0.52 | 0 | ~0 | C / compression |
| breakout_up_with_volume | 0 | ~0.87 | 0 | **≥0.2** | ~0.02 | Expansion share > 0 |
| failed_breakout | 0 | ~0.79 | 0 | **≈0** | ~0 | S / sideways |
| v_reversal | 0 | 0 | 0 | 0 | ~0.04 | S (no clean trend) |
| flat_dead | 0 | ~0.52 | 0 | 0 | ~0 | S / sideways |

## Loading in tests

```go
series := fixtures.Load(t, "messy_uptrend")
```

Package: `pano_chart/backend/tests/fixtures/candles` (`RequiredBars = 110`).

## Regenerating

Current fixtures are **intentional synthetics** (`source: synthetic`) for offline
CI: each archetype is shaped so domain calculators produce a differentiated
fingerprint (see table above). Swapping in live Binance windows is encouraged
for Track E when a real-market wick/volume structure is needed:

1. Pull klines (e.g. `GET /api/v3/klines?symbol=BTCUSDT&interval=15m&limit=110`).
2. Map each row to `{t: openTime/1000, o,h,l,c, v: quoteVolume}`.
3. Set `"source": "binance"`, matching `"archetype"`, and today's `"recorded"`.
4. Run `go test ./tests/domain/scoring/ -run Golden` and
   `go test ./tests/application/market/ -run GoldenTape`.
5. Update the score-vector table above and tighten golden ranges only when the
   change is intentional.

`fixtures.Load` builds candles with `domain.NewCandle` (full OHLC / alignment
validation) — regenerated files that fail those checks will fail the suite.

Breakout fixtures must exercise `DetectBreakout` (swing channel + BVS/CCS on
the last bar). A ramp with elevated volume alone is not enough.

Do not commit one-off fetch scripts into the repo.
