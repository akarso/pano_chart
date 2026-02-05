# FRONTEND.md

## Purpose

This document defines **frontend-specific architecture, rules, and delivery sequencing**.

It must be read **together with `AGENTS.md`** (global rules) and **`COMMON.md`** (shared contracts).

Anything not explicitly overridden here inherits from `AGENTS.md`.

---

## Frontend Mission

Deliver a fast, readable, and low-friction mobile experience that:

* shows many market charts on a single scrollable screen
* allows quick visual comparison (sideways action, volatility, trend)
* stays responsive under large symbol counts
* is extensible for future indicators (RSI, liquidity, overlays)

---

## Technology Stack

* **Framework**: Flutter (Dart)
* **Platforms**: iOS, Android
* **Networking**: HTTP (REST)
* **State Management**: Explicit, testable (no global mutable state)

Framework or library choices must prioritize:

* testability
* determinism
* low cognitive overhead

---

## Architectural Style

Frontend follows a **layered, feature-oriented architecture** with clear boundaries.

### Layers

```
Presentation (Widgets)
View Models / Controllers
Application Logic
Infrastructure (API, storage)
```

### Direction Rules

* Widgets depend only on View Models
* View Models depend on Application Logic + COMMON contracts
* Infrastructure depends on Application Logic (never the opposite)

---

## Folder Structure

```
/frontend
  /lib
    /features
      /overview
        overview_screen.dart
        overview_view_model.dart
    /domain
      symbol.dart
      timeframe.dart
    /infrastructure
      api_client.dart
  /test
    /features
    /domain
```

Rules:

* Feature folders own their UI + state
* No shared god-components

---

## State Management Rules

* State must be explicit and observable
* No hidden side effects in widgets
* Business decisions live outside widgets

View Models:

* expose immutable state objects
* react to user intent
* coordinate async work

---

## Domain Mapping

Frontend domain objects must:

* mirror semantics from `COMMON.md`
* avoid backend-specific assumptions
* perform basic validation only

No business rules duplication.

---

## Networking Rules

* All API calls go through infrastructure adapters
* DTOs are mapped at the boundary
* Errors are mapped to user-facing states

No raw HTTP responses reach widgets.

---

## Testing Strategy

* Domain: unit tests
* View Models: behavior tests
* Widgets: shallow widget tests

Rules:

* No golden tests for logic
* UI tests focus on structure, not pixels

---

## Pull Request Roadmap

### Phase 0 — Baseline

* PR-000: Flutter project scaffolding

### Phase 1 — Shared Domain

* PR-001: Symbol
* PR-002: Timeframe

### Phase 2 — Infrastructure

* PR-003: API client
* PR-004: Error mapping

### Phase 3 — Features

* PR-005: Overview ViewModel
* PR-006: Overview Screen

### Phase 4 — Hardening

* PR-007: Performance tuning
* PR-008: Offline / retry behavior

---

## PR Discipline (Frontend)

* One feature or concern per PR
* ViewModel before Widget
* Tests before UI polish

---

## Relationship to Other Docs

* Global rules: `AGENTS.md`
* Shared contracts: `COMMON.md`
* Backend rules: `BACKEND.md`

---

## Guiding Principle

Clarity on small screens beats clever interactions.