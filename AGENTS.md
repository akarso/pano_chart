# AGENTS.md

## Purpose

This document defines **global collaboration rules** for both human contributors and LLM agents working on this project.

These rules apply **across repositories** (backend, frontend, infra) unless explicitly overridden by a repo-local document.

---

## Core Principles

* Tests are the primary specification
* Prefer small, reviewable pull requests
* High cohesion, loose coupling
* One reason to change per class
* Methods do one thing; long methods must be split
* Low cyclomatic complexity is enforced

---

## Development Workflow

* Work is delivered via small, focused PRs
* One PR = one responsibility
* Avoid large "everything" PRs
* Each PR must be reviewable independently

---

## Architecture Rules (Global)

* Ports & adapters pattern
* Interfaces define boundaries
* Implementations live behind adapters
* No infrastructure concerns in domain logic

---

## Testing Rules

* TDD when feasible
* Tests describe behavior, not implementation
* Tests act as living documentation
* Tests should be deterministic

---

## Communication Rules for LLMs

* If uncertain, ask
* Do not hallucinate APIs or behavior
* Follow existing naming and structure
* Do not introduce scope creep

---

## Code Quality

* Follow PSR standards where applicable
* Static analysis must pass
* Formatting and linting are mandatory

---

## Repository-Specific Rules

See:

* `BACKEND.md` for backend-specific architecture and PR sequencing
* `FRONTEND.md` for frontend-specific rules
* `COMMON.md` for shared contracts and API boundaries

---

## Guiding Principle

Clarity beats cleverness. Small steps beat big rewrites.
