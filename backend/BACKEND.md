# BACKEND.md

## Purpose

This document defines **backend-specific architecture, rules, and delivery sequencing**.

It must be read **together with `AGENTS.md`**, which defines global collaboration and quality rules.

Anything not explicitly overridden here inherits from `AGENTS.md`.

---

## Backend Mission

Provide a fast, deterministic, and extensible backend that:

* aggregates crypto market data
* produces multi-symbol chart overviews
* supports future indicators (RSI, liquidity, etc.)
* minimizes external API usage via caching

---

## Architectural Style

The backend follows **Hexagonal Architecture (Ports & Adapters)**.

### Layers

```
Domain        → Pure business logic
Application   → Use cases / orchestration
Ports         → Interfaces (inbound & outbound)
Adapters      → Infrastructure implementations
```

### Direction Rules

* Domain depends on nothing
* Application depends only on Domain + Ports
* Adapters depend on Ports (never the opposite)
* No adapter logic leaks upward

---

## Folder Structure

```
/backend
  /domain
  /app
  /ports
  /adapters
    /infrastructure
    /http
  /tests
    /domain
    /app
    /ports
    /adapters
```

Rules:

* Folder boundaries are architectural boundaries
* Cross-layer imports are forbidden

---

## Domain Layer Rules

* Domain contains only:

  * Value Objects
  * Entities
  * Domain services (pure)
* No IO
* No frameworks
* No configuration

Examples:

* Symbol
* Timeframe
* Candle
* CandleCollection

---

## Ports

Ports are **interfaces only**.

### Outbound Ports

* MarketDataProvider
* CandleRepository
* CacheStore
* TimeProvider

### Inbound Ports

* Application use cases

Rules:

* No default implementations
* No infrastructure assumptions

---

## Application Layer

* Use cases orchestrate domain + ports
* No business rules inside controllers
* One use case = one intent

Examples:

* RefreshMarketData
* GetOverviewCharts

---

## Adapters

Adapters provide concrete implementations for ports.

### Infrastructure Adapters

* Redis cache
* Exchange APIs (Binance, etc.)

### HTTP Adapters

* REST controllers
* DTO mapping only

Rules:

* Adapters may depend on frameworks
* Adapters must conform strictly to port contracts

---

## Testing Strategy

* Domain: pure unit tests
* Application: use case tests with fake adapters
* Adapters: contract tests

No test may cross architectural boundaries.

---

## Pull Request Roadmap

PRs must be merged **in order**.

### Phase 0 — Baseline

* PR-000: Repo scaffolding & CI

### Phase 1 — Domain

* PR-001: Symbol
* PR-002: Timeframe
* PR-003: Candle
* PR-004: CandleCollection
* PR-005: MarketSnapshot

### Phase 2 — Ports

* PR-006: Core ports

### Phase 3 — Fake Adapters

* PR-007: In-memory CandleRepository
* PR-008: Fake MarketDataProvider
* PR-009: In-memory CacheStore

### Phase 4 — Application

* PR-010: RefreshMarketData
* PR-011: BuildMarketSnapshot
* PR-012: GetOverviewCharts

### Phase 5 — Infrastructure

* PR-013: Redis CacheStore
* PR-014: First exchange adapter

### Phase 6 — API

* PR-015: Overview endpoint

---

## PR Discipline (Backend)

* One PR = one layer concern
* Tests before implementation
* No refactors outside scope
* Each PR must satisfy its Definition of Done

---

## Relationship to Other Docs

* Global rules: `AGENTS.md`
* Shared contracts: `COMMON.md`
* Frontend rules: `FRONTEND.md`

---

## Guiding Principle

Backend correctness precedes performance. Performance precedes features.
