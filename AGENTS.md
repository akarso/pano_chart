# Project Overview

## Working Name

**Panorama Charts** (placeholder)

## One‑sentence mission

Provide crypto traders with a fast, visual, multi‑chart overview that makes it easy to scan dozens of markets at once, spot actionable price behavior (ranges, volatility, trends), and then dive deeper — without building complex dashboards.

---

## Problem Statement

Active crypto traders lack a simple way to visually scan many markets at once. Existing tools focus either on:

* single full‑size charts (great depth, poor overview), or
* numeric screeners (good filters, weak visual intuition).

This forces traders to juggle tabs, pay for expensive tools, or miss opportunities entirely.

---

## Product Vision

A **mobile‑first (Flutter) application** that displays **30–100+ simplified charts on one scrollable screen**, optimized for pattern recognition and rapid decision‑making.

The app prioritizes:

* speed and clarity over indicator overload
* visual pattern scanning over precise execution
* extensibility into more advanced analytics over time

It complements, rather than replaces, full charting platforms.

---

## Core Principles

* **Overview first**: the main screen is always a multi‑chart overview
* **Low friction**: no heavy setup, minimal configuration
* **Cheap to run**: leverage free exchange APIs + aggressive caching
* **Backend‑centric**: clients never hit exchanges directly
* **Extensible**: indicators and new chart types can be added later

---

## Target Users

* Retail crypto traders (spot & perpetuals)
* Swing traders scanning for consolidation / breakouts
* Power users who already use TradingView but want faster scanning

Non‑targets (initially):

* High‑frequency traders
* On‑chain analysts
* Execution‑focused trading terminals

---

## Platform & Tech Constraints

### Client

* **Flutter (Dart)**
* Single shared codebase for iOS & Android
* Mobile‑first UX, tablet‑friendly later

### Backend

* Simple, VPS‑friendly stack (no cloud lock‑in)
* Stateless API server + Redis cache
* Easy to deploy via Docker or bare metal
* Language candidates: Go, Node.js, Python (FastAPI)

### Data

* Free exchange REST APIs (Binance, Bybit, OKX, Kraken)
* No client‑side exchange calls
* Centralized polling, normalization, and caching
* Redis used for:

  * candle storage
  * pre‑aggregated chart data
  * lightweight metrics

---

## Functional Scope (High Level)

### Included in Initial Vision

* Multiple markets on one scrollable screen
* Simplified candle or sparkline‑style charts
* Configurable timeframe (e.g. 5m, 15m, 1h, 4h)
* Sorting and grouping (by volume, volatility, % change)
* Tap to expand into a larger single‑chart view

### Explicitly Excluded (for MVP)

* Order execution
* Drawing tools
* User strategies or bots
* On‑chain data

---

# Development Milestones

## Milestone 0 — Concept Validation

**Goal:** Ensure the problem is real and the solution resonates.

Deliverables:

* Written problem & value proposition
* Competitive comparison (TradingView, Coinigy, CoinMarketCap)
* Definition of "success" for MVP (daily active use, retention)

---

## Milestone 1 — System Architecture Definition

**Goal:** Lock down a simple, scalable architecture.

Deliverables:

* Backend responsibilities (polling, caching, aggregation)
* Client responsibilities (rendering, interaction)
* Data flow diagrams (exchange → backend → Redis → client)
* API contract draft (endpoints, payload shape)

---

## Milestone 2 — Backend MVP

**Goal:** Serve reliable, cached market data to clients.

Deliverables:

* VPS‑deployable API server
* Exchange adapters (1–2 exchanges initially)
* Candle normalization logic
* Redis caching strategy
* Simple health & metrics endpoints

Non‑goals:

* Authentication
* Billing
* High availability

---

## Milestone 3 — Flutter Client MVP

**Goal:** Render multi‑chart overview smoothly on mobile.

Deliverables:

* Scrollable multi‑chart overview screen
* Efficient chart rendering (canvas‑based)
* Basic settings (timeframe, number of charts)
* Stateless client behavior

Key risk addressed:

* Performance with many charts on one screen

---

## Milestone 4 — Usability & Performance Pass

**Goal:** Make the app feel instant and usable daily.

Deliverables:

* Skeleton loading & caching on client
* Backend polling optimization
* Memory & battery usage review
* UI tweaks based on real usage

---

## Milestone 5 — Pattern‑Aware Enhancements

**Goal:** Increase perceived value without complexity.

Examples:

* Volatility highlighting
* Simple range detection
* Color‑coded consolidation markers

Still overview‑focused — no advanced indicators yet.

---

## Milestone 6 — Monetization Readiness

**Goal:** Prepare for paid tiers without locking in decisions.

Deliverables:

* Feature flags (free vs paid)
* Usage limits enforced server‑side
* Analytics hooks (no user profiling)
* App Store subscription plumbing (disabled by default)

---

## Milestone 7 — Extensibility Phase (Post‑MVP)

**Goal:** Add depth without losing simplicity.

Potential additions:

* RSI / ATR overlays (lightweight)
* Liquidity or volume‑profile style charts
* Saved layouts
* Alerts (server‑side)

---

## Non‑Goals (Explicit)

* Becoming a full trading terminal
* Competing with TradingView feature‑by‑feature
* Supporting every exchange immediately

---

## Success Metrics (Early)

* App opens per user per day
* Time to identify a chart worth deeper analysis
* Retention after 7 / 30 days
* Backend cost per active user

---

## Development Guidelines

These guidelines are **normative**. If unsure, ask. Do not guess or hallucinate behavior.

---

### 1. General Engineering Principles

* Prefer **clarity over cleverness**.
* Optimize for **reviewability**, not speed of initial implementation.
* Every class should have **one reason to change** (SRP).
* Methods must do **one thing**; if a method grows beyond ~15–20 lines, split it.
* Keep **cyclomatic complexity low**; flatten conditionals where possible.
* High cohesion, loose coupling at all times.

---

### 2. Architecture Style

* Follow **Ports & Adapters (Hexagonal Architecture)** where applicable.
* Define **interfaces as ports**; concrete implementations are adapters.
* Business logic must not depend on:

  * frameworks
  * external APIs
  * persistence details
* Dependency direction must always point **inward**.

---

### 3. Test-Driven Development (TDD)

* Tests act as **executable specifications**.
* Write tests **before or alongside** production code whenever feasible.
* Every public class must have:

  * clear unit tests defining expected behavior
  * failure cases explicitly covered
* Avoid testing implementation details; test **observable behavior**.

Acceptable test types:

* Unit tests (primary)
* Contract tests (for adapters)

Non-goals:

* Full end-to-end tests in early milestones

---

### 4. Backend-Specific Guidelines

#### Language & Style

* Backend language must:

  * run easily on a VPS
  * have fast startup and low memory footprint
* Follow **PSR standards** strictly (PSR-1, PSR-12, PSR-4) if using PHP.
* Enforce static analysis (PHPStan / Psalm equivalent).

#### Structure

* Separate modules for:

  * market data fetching
  * normalization
  * caching
  * API presentation
* Redis access must go through an interface (port).
* Exchange clients must be swappable without touching business logic.

---

### 5. Flutter / Client Guidelines

* UI must be **stateless by default**.
* Business logic must live outside widgets.
* Rendering logic must be deterministic and testable.
* Avoid unnecessary rebuilds; performance regressions are treated as bugs.

---

### 6. API & Data Contracts

* API responses are **contracts**, not implementation details.
* Any breaking change requires:

  * version bump, or
  * explicit migration path
* Backend must never expose raw exchange payloads directly.

---

### 7. Pull Request Discipline

* Prefer **small, focused PRs** over large changesets.
* Each PR must:

  * address a single concern
  * include tests
  * include a short rationale
* PRs without tests require explicit justification.

---

### 8. Review & Collaboration Rules

* If requirements are unclear: **stop and ask**.
* Do not assume future features.
* No speculative abstractions.
* Every abstraction must earn its existence.

---

### 9. Definition of Done

A change is considered done only if:

* tests pass
* static analysis passes
* code is readable without explanation
* diff can be reviewed in under 10 minutes

---

## Backend Folder Structure & Module Boundaries

This structure enforces **ports & adapters**, testability, and small PRs. Imports must respect dependency direction.

---

## Top-Level Layout

```
backend/
├── app/                    # Application layer (use cases)
├── domain/                 # Core business logic (pure)
├── ports/                  # Interfaces (inbound & outbound)
├── adapters/               # Implementations of ports
├── infrastructure/         # Framework / runtime glue
├── api/                    # HTTP layer (controllers, DTOs)
├── config/                 # Configuration & wiring
├── tests/                  # Unit & contract tests
└── bootstrap.php           # Application entrypoint
```

---

## Module Responsibilities

### 1. `domain/`

**What belongs here:**

* Core entities (e.g. Candle, Market, Timeframe)
* Pure domain services (e.g. CandleAggregator)
* Value objects

**Rules:**

* No framework imports
* No HTTP, Redis, or exchange knowledge
* No configuration

**Allowed dependencies:**

* Standard library only

---

### 2. `ports/`

**What belongs here:**

* Interfaces defining external dependencies

Examples:

* `MarketDataProvider`
* `CandleRepository`
* `CacheStore`

**Rules:**

* Interfaces only
* No logic
* Stable contracts

---

### 3. `app/`

**What belongs here:**

* Application services / use cases
* Orchestration logic

Examples:

* `GetOverviewCharts`
* `RefreshMarketData`

**Rules:**

* Depends only on `domain/` and `ports/`
* No framework-specific code
* No IO

---

### 4. `adapters/`

**What belongs here:**

* Implementations of ports

Subfolders:

```
adapters/
├── exchange/               # Binance, Bybit, etc.
├── persistence/            # Redis implementations
└── cache/
```

**Rules:**

* Must implement exactly one port per class
* No business logic beyond adaptation

---

### 5. `api/`

**What belongs here:**

* HTTP controllers
* Request/response DTOs

**Rules:**

* Thin layer only
* Translate HTTP → app use cases
* No business rules

---

### 6. `infrastructure/`

**What belongs here:**

* Framework bootstrapping
* Dependency injection
* Logging, metrics

**Rules:**

* Glue code only
* No domain logic

---

### 7. `config/`

**What belongs here:**

* Environment configuration
* Service wiring

**Rules:**

* No logic
* Read-only at runtime

---

### 8. `tests/`

**Structure mirrors source folders**

```
tests/
├── domain/
├── app/
├── adapters/
└── api/
```

**Rules:**

* Tests define behavior
* Adapter tests act as contract tests

---

## Forbidden Dependencies

| From   | Must NOT depend on            |
| ------ | ----------------------------- |
| domain | adapters, api, infrastructure |
| ports  | adapters, api                 |
| app    | adapters, api, infrastructure |

---

## Change Discipline

* A PR touching more than **one top-level module** must be justified
* New adapters require corresponding port tests
* Domain changes require updated specs (tests)

---

## Core Backend Ports (Interfaces Only)

These interfaces define **all external dependencies** of the backend. They are stable contracts. Implementations live in `adapters/`.

No interface may:

* reference a concrete class
* reference a framework
* perform IO

If behavior is unclear, add a test before changing the interface.

---

## 1. MarketDataProvider

**Purpose**: Fetch raw market data from an external source (exchange).

**Responsibilities**:

* Retrieve OHLCV candle data for a symbol and timeframe
* Handle pagination / limits internally

**Does NOT**:

* Cache
* Normalize across exchanges
* Interpret candles

**Conceptual Interface**:

* getCandles(symbol, timeframe, limit) → CandleCollection

---

## 2. CandleRepository

**Purpose**: Persist and retrieve normalized candle data.

**Responsibilities**:

* Store candles by (symbol, timeframe)
* Retrieve most recent candles

**Does NOT**:

* Fetch from exchanges
* Decide refresh policy

**Conceptual Interface**:

* saveCandles(symbol, timeframe, candles)
* getCandles(symbol, timeframe, limit) → CandleCollection

---

## 3. CacheStore

**Purpose**: Generic cache abstraction (Redis initially).

**Responsibilities**:

* Key-value storage
* TTL support

**Does NOT**:

* Know about candles or markets

**Conceptual Interface**:

* get(key)
* set(key, value, ttl)
* delete(key)

---

## 4. OverviewQuery

**Purpose**: Read-optimized access for overview screens.

**Responsibilities**:

* Return pre-aggregated data for many symbols
* Optimized for read performance

**Does NOT**:

* Fetch raw exchange data
* Perform heavy calculations

**Conceptual Interface**:

* getOverview(timeframe, symbols[]) → OverviewDataset

---

## 5. TimeProvider

**Purpose**: Abstract system time for testability.

**Responsibilities**:

* Provide current timestamp

**Conceptual Interface**:

* now() → DateTimeImmutable

---

## 6. Logger (Optional Port)

**Purpose**: Decouple logging from implementation.

**Responsibilities**:

* Structured logging

**Conceptual Interface**:

* info(message, context)
* error(message, context)

---

## Port Interaction Rules

* Application layer depends on ports, never adapters
* Adapters implement exactly one port
* Ports must remain stable across milestones

---

## Testing Rules

* Each port must have:

  * a fake or in-memory implementation for tests
  * contract tests for real adapters

---

## Domain Model + Tests (Specification First)

This section defines the **core domain model** and the **tests that specify its behavior**. Domain code is pure and framework-free.

Tests are the primary source of truth.

---

## Domain Objects

### 1. Symbol

**Description**:
Represents a tradeable market identifier (e.g. BTCUSDT, ETH-USD).

**Responsibilities**:

* Encapsulate symbol identity
* Enforce valid formatting rules

**Rules**:

* Immutable
* Value object semantics (equality by value)

**Test Specifications**:

* GIVEN a valid symbol string, WHEN creating Symbol, THEN it is accepted
* GIVEN an invalid or empty symbol, THEN creation fails
* Symbols with same normalized value are equal

---

### 2. Timeframe

**Description**:
Represents candle resolution (e.g. 1m, 5m, 1h).

**Responsibilities**:

* Encapsulate duration
* Prevent unsupported timeframes

**Rules**:

* Immutable
* Finite, predefined set

**Test Specifications**:

* GIVEN a supported timeframe, THEN it can be created
* GIVEN an unsupported timeframe, THEN creation fails
* Timeframes expose duration in seconds

---

### 3. Candle

**Description**:
Represents a single OHLCV candle.

**Fields**:

* openTime
* open
* high
* low
* close
* volume

**Rules**:

* high ≥ max(open, close)
* low ≤ min(open, close)
* volume ≥ 0
* openTime is immutable

**Test Specifications**:

* GIVEN valid OHLCV values, THEN candle is created
* GIVEN invalid price relationships, THEN creation fails
* Candle exposes derived properties (e.g. isBullish, range)

---

### 4. CandleCollection

**Description**:
Ordered collection of candles for one symbol + timeframe.

**Responsibilities**:

* Preserve chronological order
* Provide lightweight analytics

**Rules**:

* Immutable
* All candles share same timeframe

**Test Specifications**:

* GIVEN unordered candles, THEN collection is ordered
* GIVEN mixed timeframe candles, THEN creation fails
* Collection returns most recent candle

---

### 5. MarketSnapshot

**Description**:
Read-optimized snapshot for overview rendering.

**Fields**:

* symbol
* timeframe
* candles (compressed)
* metrics (e.g. range, volatility)

**Rules**:

* No raw exchange fields
* Serializable

**Test Specifications**:

* GIVEN candle collection, THEN snapshot metrics are consistent
* Snapshot creation is deterministic

---

## Domain Services

### CandleAggregator

**Purpose**:
Transform raw candles into compressed / aggregated forms for overview.

**Rules**:

* Pure function
* No IO

**Test Specifications**:

* GIVEN N candles, THEN output size is bounded
* Aggregation preserves trend direction

---

## Testing Conventions

* Tests live in `tests/domain/`
* Test names follow: `it_describes_expected_behavior`
* Use GIVEN / WHEN / THEN structure in test bodies
* No mocks in domain tests

---

## Change Rules

* Any change to a domain object requires updating tests first
* Domain objects must never depend on ports or adapters

---

## Test Naming & Structure Guide

Tests are **executable specifications**. They must be readable without opening the implementation.

If a test name cannot be read as an English sentence describing behavior, it is incorrectly named.

---

## 1. General Rules

* Prefer **clarity over brevity**
* One behavior per test
* One reason to fail per test
* Avoid asserting multiple unrelated outcomes

Tests should answer:

> *What must always be true about this unit?*

---

## 2. File & Folder Structure

Test structure mirrors source structure exactly:

```
tests/
├── domain/
│   ├── SymbolTest.php
│   ├── TimeframeTest.php
│   └── CandleTest.php
├── app/
├── ports/
└── adapters/
```

Rules:

* One test class per production class
* Test class name = `<ClassName>Test`

---

## 3. Test Method Naming

### Preferred Style

```
it_describes_expected_behavior
```

Examples:

* `it_accepts_a_valid_symbol`
* `it_rejects_empty_symbol`
* `it_orders_candles_chronologically`

### Forbidden Styles

* `testSomething`
* `shouldDoX`
* `whenX_thenY`

The test name alone should explain *why the test exists*.

---

## 4. GIVEN / WHEN / THEN Structure

Each test body must follow this logical structure:

```
// GIVEN some initial state

// WHEN an action occurs

// THEN the expected outcome is observed
```

Rules:

* GIVEN setup is minimal
* WHEN contains exactly one action
* THEN contains assertions only

---

## 5. Assertions

* Prefer **specific assertions** over generic ones
* Avoid asserting internal state unless it is part of the contract
* Assert invariants explicitly

Bad:

* asserting array sizes without meaning
* asserting multiple properties at once

Good:

* asserting ordering
* asserting equality semantics
* asserting failure on invalid input

---

## 6. Exceptions & Failure Cases

Failure behavior must be tested explicitly.

Rules:

* Use domain-specific exceptions where appropriate
* Assert *that* a failure happens, not *how* it is implemented

Example:

* `it_rejects_candles_with_invalid_price_relationships`

---

## 7. Mocks, Fakes, and Stubs

### Domain Tests

* No mocks
* No stubs
* Pure object testing only

### Application Tests

* Use fakes for ports
* Avoid mocks unless interaction order matters

### Adapter Tests

* Contract tests only
* External services may be simulated

---

## 8. Test Data Builders

* Use builders for complex setup
* Builders belong in `tests/support/`
* Builders must produce **valid objects by default**

---

## 9. Test Readability Checklist

Before merging, ask:

* Can a reviewer understand intent without reading implementation?
* Does the test name match the assertion?
* Would this test still make sense after a refactor?

If any answer is "no", rewrite the test.

---

## Pull Request (PR) Template

This template is **mandatory**. It enforces small, reviewable, intention-revealing changes.

A PR that does not follow this template is considered **not ready for review**.

---

## PR Title

Format:

```
[Module] Short, explicit description of change
```

Examples:

* `[Domain] Add Candle value object invariants`
* `[Ports] Introduce CandleRepository interface`
* `[Adapters][Redis] Implement CacheStore`

---

## 1. Purpose

**What problem does this PR solve?**

(1–3 sentences max. No implementation details.)

---

## 2. Scope of Change

**This PR includes:**

* [ ] New domain object
* [ ] New port (interface)
* [ ] Adapter implementation
* [ ] Tests only
* [ ] Refactor (no behavior change)

**Touched modules:**

* domain / app / ports / adapters / api / infrastructure

> If more than one top-level module is touched, explain why this could not be split.

---

## 3. Behavior (Specification)

**What behavior is introduced or changed?**

List behaviors in plain language. These must be traceable to tests.

Example:

* Rejects candles where `low > high`
* Orders candles chronologically on creation

---

## 4. Tests

**Tests added or updated:**

* `it_rejects_invalid_price_relationships`
* `it_orders_candles_chronologically`

Rules:

* Tests must describe behavior, not implementation
* If no tests were added, explain why

---

## 5. Non-Goals / Explicitly Not Done

State what this PR deliberately does **not** do.

Examples:

* No Redis implementation
* No API exposure
* No performance optimization

---

## 6. Review Notes

**Things reviewers should pay attention to:**

* Boundary decisions
* Naming
* Invariants

---

## 7. Checklist (Must Be True)

* [ ] Tests pass
* [ ] Static analysis passes
* [ ] No speculative abstractions added
* [ ] Code can be reviewed in under 10 minutes
* [ ] Domain logic has no external dependencies

---

## Application Use Cases (Interfaces + Tests as Specification)

Application layer coordinates domain objects via ports. It contains **no business rules**, only orchestration.

Use cases are defined by **what the system does**, not how.

---

## General Rules for Application Layer

* One class = one use case
* Use cases depend only on:

  * domain
  * ports
* No framework, HTTP, or persistence knowledge
* Constructor injection only

---

## 1. GetOverviewCharts

### Purpose

Provide read-optimized market snapshots for the multi-chart overview screen.

### Inputs

* timeframe
* list of symbols
* limit (candles per symbol)

### Outputs

* Collection of MarketSnapshot

### Responsibilities

* Coordinate retrieval of pre-aggregated data
* Ensure consistent timeframe and ordering

### Does NOT

* Fetch data from exchanges directly
* Perform heavy calculations
* Cache data itself

---

### Interface (Conceptual)

* execute(timeframe, symbols[], limit) → OverviewDataset

---

### Test Specifications

**Happy path**

* GIVEN cached overview data exists, WHEN executing, THEN snapshots are returned
* GIVEN multiple symbols, THEN one snapshot per symbol is returned

**Edge cases**

* GIVEN empty symbol list, THEN empty result is returned
* GIVEN unsupported timeframe, THEN execution fails

**Contract rules**

* Returned snapshots are deterministic
* No duplicate symbols in result

---

## 2. RefreshMarketData

### Purpose

Refresh candle data from external providers into storage.

### Inputs

* timeframe
* symbol
* limit

### Outputs

* None (side-effect only)

### Responsibilities

* Fetch raw candles via MarketDataProvider
* Normalize into domain objects
* Persist via CandleRepository

### Does NOT

* Decide refresh schedules
* Expose data to API

---

### Interface (Conceptual)

* execute(symbol, timeframe, limit) → void

---

### Test Specifications

**Happy path**

* GIVEN provider returns candles, THEN they are persisted

**Failure cases**

* GIVEN provider fails, THEN no partial data is stored
* GIVEN invalid candle data, THEN persistence is skipped

**Contract rules**

* Repository save is called once per execution
* No caching logic inside use case

---

## 3. BuildMarketSnapshot (Internal Use Case)

### Purpose

Transform CandleCollection into MarketSnapshot.

### Inputs

* Symbol
* Timeframe
* CandleCollection

### Outputs

* MarketSnapshot

### Responsibilities

* Delegate calculations to domain services
* Assemble read model

### Does NOT

* Fetch or persist data
* Know about HTTP or UI

---

### Interface (Conceptual)

* execute(symbol, timeframe, candles) → MarketSnapshot

---

### Test Specifications

* GIVEN valid candle collection, THEN snapshot metrics match domain calculations
* GIVEN empty candle collection, THEN snapshot is still valid

---

## Testing Rules for Application Layer

* Tests live in `tests/app/`
* Use **fakes** for ports
* No mocks unless interaction order is critical
* GIVEN / WHEN / THEN mandatory

---

## Change Discipline

* New use case = new test class
* Changing use case behavior requires test updates first
* Use cases must remain thin

---

## PR Roadmap (Sequencing & Scope Lock)

This roadmap defines the **exact order and scope** of pull requests. Each PR is intentionally small, test-driven, and reviewable in isolation.

PRs must be completed **in order** unless explicitly justified.

---

## Phase 0 — Repository & Tooling Baseline

### PR-000: Repository scaffolding

**Scope**:

* Folder structure (empty)
* Test framework setup
* Static analysis config

**Includes**:

* `/domain`, `/ports`, `/app`, `/adapters`, `/tests`
* CI running tests + static analysis

**Excludes**:

* Any production code

---

## Phase 1 — Domain Model (Pure, Tested)

### PR-001: Symbol value object

* Symbol invariants
* Equality semantics
* Domain tests only

---

### PR-002: Timeframe value object

* Supported timeframe set
* Duration exposure
* Domain tests only

---

### PR-003: Candle value object

* OHLCV invariants
* Derived properties
* Domain tests only

---

### PR-004: CandleCollection

* Ordering guarantees
* Homogeneous timeframe enforcement
* Domain tests only

---

### PR-005: MarketSnapshot + CandleAggregator

* Snapshot structure
* Aggregation rules
* Domain tests only

---

## Phase 2 — Ports (Interfaces Only)

### PR-006: Core ports introduction

* MarketDataProvider
* CandleRepository
* CacheStore
* TimeProvider

**Rules**:

* Interfaces only
* No implementations

---

## Phase 3 — Fake Adapters (In-Memory)

### PR-007: In-memory CandleRepository

* Test-only adapter
* Contract tests

---

### PR-008: Fake MarketDataProvider

* Deterministic candle generation
* Contract tests

---

### PR-009: In-memory CacheStore

* TTL simulation
* Contract tests

---

## Phase 4 — Application Use Cases

### PR-010: RefreshMarketData use case

* Tests first
* Uses fake adapters

---

### PR-011: BuildMarketSnapshot use case

* Snapshot assembly
* Tests only

---

### PR-012: GetOverviewCharts use case

* Overview orchestration
* Tests first

---

## Phase 5 — Real Infrastructure (Minimal)

### PR-013: Redis CacheStore adapter

* Production Redis adapter
* Contract tests

---

### PR-014: First Exchange Adapter (e.g. Binance)

* REST integration
* Rate-limit aware
* Contract tests

---

## Phase 6 — API Layer

### PR-015: Overview API endpoint

* HTTP controller
* DTOs only

---

## Phase 7 — Hardening & Readiness

### PR-016: Performance & limits

* Result size limits
* Guardrails

---

### PR-017: Observability

* Logging adapter
* Metrics hooks

---

## Merge Rules

* Each PR must:

  * pass all tests
  * follow PR template
  * touch only its declared scope
* Skipping PRs requires explicit architectural justification

---

## PR-001 Checklist — Symbol Value Object (Tests First)

This checklist **freezes Phase 1, PR-001**. No additional scope is permitted.

PR-001 must be fully complete and merged before PR-002 begins.

---

## PR Metadata

* **PR ID**: PR-001
* **Title**: `[Domain] Add Symbol value object invariants`
* **Scope**: Domain only
* **Change Type**: Tests first, then minimal implementation

---

## Allowed Files

✅ Allowed:

* `domain/Symbol.*`
* `tests/domain/SymbolTest.*`

❌ Forbidden:

* Any other domain objects
* Any ports, adapters, app code
* Any framework or infrastructure files

---

## Behavioral Specification (Must Be Covered by Tests)

### Creation Rules

* Accepts valid symbol strings (e.g. `BTCUSDT`, `ETH-USD`)
* Rejects empty strings
* Rejects whitespace-only strings
* Rejects symbols with illegal characters

---

### Normalization Rules

* Normalizes symbol representation consistently
* Equality is based on normalized value

Examples:

* `btc_usdt` == `BTCUSDT`
* `eth-usd` == `ETH-USD`

---

### Immutability Rules

* Symbol value cannot be modified after creation
* No setters

---

### Equality Semantics

* Two symbols with same normalized value are equal
* Different normalized values are not equal

---

## Test Requirements

* Tests must be written **before** implementation
* One test per behavior
* Test names must follow naming guide

Required tests (minimum):

* `it_accepts_valid_symbol`
* `it_rejects_empty_symbol`
* `it_rejects_symbols_with_illegal_characters`
* `it_normalizes_symbol_representation`
* `it_compares_symbols_by_value`

---

## Implementation Constraints

* Symbol is a value object
* Immutable
* No dependencies outside standard library
* Constructor validation only

---

## Definition of Done (PR-001)

PR-001 is considered **done** only when **all** conditions below are met:

* [ ] All required Symbol tests are implemented
* [ ] Tests fail before implementation is added
* [ ] All tests pass after implementation
* [ ] No tests unrelated to Symbol exist
* [ ] No production code outside `domain/Symbol.*`
* [ ] Symbol is immutable (no setters, no mutation)
* [ ] Equality is value-based and tested
* [ ] Normalization behavior is fully covered by tests
* [ ] Static analysis passes with no new warnings
* [ ] CI pipeline is green

---

## Explicit Non-Goals

* No symbol parsing beyond normalization
* No exchange-specific logic
* No registry or lookup

---

## Reviewer Checklist

Reviewer must verify:

* Tests fully describe behavior
* Implementation matches tests exactly
* No scope creep

---

## Guiding Philosophy

If PR-001 feels trivial, that means it is correct.
