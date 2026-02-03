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
