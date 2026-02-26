# Implementation Plan: Login Backoff / Brute Force Protection

**Branch**: `002-login-backoff` | **Date**: 2026-02-25 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-login-backoff/spec.md`

## Summary

Add login brute force protection to the webhook-gateway with two endpoints: a before-login check that increments counters and enforces lockout thresholds, and an after-login reset that clears counters on successful authentication. State is tracked in Redis using atomic Lua scripts for race-free counter management with TTL-based expiry. The design follows the existing webhook segregation pattern (`internal/webhooks/kratos-login-backoff/`).

**Critical finding from research**: Kratos v1.3.1 `before` hooks fire at login flow initialization (page load), not at credential submission. The before-login endpoint must be called by an integration layer (reverse proxy or application server) at submission time, not by a Kratos before-hook. The after-login endpoint works as designed via Kratos webhook. See [research.md](./research.md) R-001 for full analysis.

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: go.uber.org/zap (logging), github.com/redis/go-redis/v9 (Redis) - both existing, no new dependencies
**Storage**: Redis (existing client in `internal/clients/redis.go`)
**Testing**: `go test -v -race ./...` (existing pattern)
**Target Platform**: Linux container (Kubernetes)
**Project Type**: Single backend service (existing webhook-gateway)
**Performance Goals**: <100ms before-login response (FR-013) - achievable via single Redis round-trip using Lua script
**Constraints**: Fail-open on Redis failure; no new external dependencies; endpoints callable from multiple integration points
**Scale/Scope**: Two new webhook endpoints, one reverse proxy handler, one new webhook package, config extensions, Redis operation additions, Traefik routing, client-web lockout display

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Notes |
|---|-----------|--------|-------|
| 1 | Deterministic Configuration | PASS | New config vars (thresholds, TTLs) loaded via `internal/config` with env defaults |
| 2 | Operational Observability | PASS | Structured zap logging with correlation IDs for all backoff events (lockout, reset, skip, errors) |
| 3 | Fail-Fast Maintenance Controls | PASS | Existing maintenance middleware applies to new endpoints |
| 4 | Test-Driven Delivery | PASS | Contract tests against OpenAPI (Phase 2.5, before handlers), integration tests with mocked Redis |
| 5 | Secure Build Pipeline | PASS | Existing Dockerfile and CI pipeline, no changes needed |
| 6 | CI Integrity | PASS | Existing pipeline covers new code via `go test ./...` and `golangci-lint` |
| 7 | Evidence-Based Performance Goals | PASS | FR-013's 100ms target is meaningful: we control the Redis round-trip, verified achievable via Lua script |
| 8 | Meaningful Success Criteria | PASS | All success criteria (lockout, reset, fail-open, logging) are testable within the service |
| 9 | No Busywork | PASS | Lean design: 4 files in webhook package (handler, models, service, proxy), minimal config additions |
| 10 | Meaningful Tests Only | PASS | Tests defend real invariants: lockout thresholds, counter reset, fail-open behavior |
| 11 | Latest Dependencies Always | PASS | No new dependencies; existing ones already current |
| 12 | Always Root Cause Analysis | PASS | N/A for new feature |
| 13 | No Legacy Code | PASS | Clean implementation, no backward-compatibility concerns |
| 14 | Single Source of Truth | PASS | Redis operations in `internal/clients/redis.go`, config in `internal/config`, business logic in webhook package |
| 15 | No Assumptions | PASS | Kratos hook behavior verified via source code analysis (research.md R-001) |
| 16 | Webhook Segregation | PASS | Dedicated `internal/webhooks/kratos-login-backoff/` folder with handler, models, service |

**Pre-design gate**: PASS (all principles satisfied)
**Post-design gate**: PASS (design verified against all principles; no violations found)

## Project Structure

### Documentation (this feature)

```text
specs/002-login-backoff/
├── plan.md              # This file
├── research.md          # Phase 0: Kratos hook behavior, Redis patterns, response format
├── data-model.md        # Phase 1: Redis keys, request/response types, state transitions
├── quickstart.md        # Phase 1: Local development guide
├── contracts/
│   └── login-backoff-openapi.yaml  # Phase 1: OpenAPI 3.0.3 contract
├── decisions/
│   └── ADR-001-reverse-proxy-for-login-interception.md  # Proxy vs Kratos hooks (Constitution #17)
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
cmd/server/
└── main.go                          # Add: login-backoff handler registration, proxy routes, config fields

internal/
├── config/
│   └── config.go                    # Add: LoginBackoff* config fields and env loading
├── clients/
│   └── redis.go                     # Add: Lua script, IncrementLoginAttempts, ResetLoginAttempts
└── webhooks/
    ├── kratos-verification/         # Existing (unchanged)
    │   ├── handler.go
    │   ├── models.go
    │   └── service.go
    └── kratos-login-backoff/        # NEW
        ├── handler.go               # HTTP handlers for before-login and after-login
        ├── models.go                # Request/response types
        ├── proxy.go                 # Reverse proxy intercepting Kratos login submissions
        └── service.go               # Business logic: validate, increment, check, reset

configs/
└── .env.example                     # Add: LOGIN_BACKOFF_* variables

contracts/
└── openapi.yaml                     # Add: login-backoff endpoint definitions
```

**Structure Decision**: Follows existing webhook segregation pattern (Constitution #16). New webhook gets its own package under `internal/webhooks/kratos-login-backoff/` with handler/models/service/proxy files. Shared infrastructure (Redis client, config, middleware) is extended in-place. No new packages or abstractions introduced.

## Key Design Decisions

### D-001: Increment-on-Check Model

The before-login endpoint atomically increments the counter AND checks the threshold in a single operation (single Redis round-trip via Lua script). A successful login resets the counter via the after-login endpoint. No explicit "failure" signal is needed.

### D-002: Fixed-Window TTL

Counter TTL is set on the first increment only (not refreshed on subsequent increments). The lockout window expires N seconds after the first failed attempt, regardless of continued attempts. This matches the spec's description and provides a predictable lockout duration for users.

### D-003: Caller-Agnostic Endpoints

Endpoints accept identifier and/or IP as optional fields, returning simple JSON responses. The integration layer (Kratos hook, proxy, or application server) is responsible for calling the endpoint at the right time and adapting the response format if needed. This decouples the webhook-gateway logic from Kratos-specific hook timing limitations.

### D-004: Partial Data Support

If only identifier or only IP is provided, the endpoint checks/increments only the available dimension. If neither is provided, the endpoint logs a warning and allows the request (fail-open). This supports:
- Kratos after-login hook (has identifier + IP via Jsonnet)
- Proxy integration (has both)
- Degraded scenarios (missing fields)

### D-005: Lockout Precedence

When both identifier and IP exceed their thresholds, the lockout with the longer remaining TTL takes precedence in the response (as specified in edge cases). The check evaluates identifier first, then IP; if both are locked, the one with more remaining time is reported.

### D-006: Built-in Reverse Proxy

Research R-001 identified that Kratos before-hooks cannot intercept credential submissions. Rather than leaving the integration layer as an external infrastructure concern, the webhook-gateway includes a built-in reverse proxy (`proxy.go`) using Go's `httputil.ReverseProxy`. Traefik routes login traffic (`/ory/kratos/public/self-service/login*`) to the webhook-gateway, which checks backoff on POST requests and proxies all requests (allowed POSTs + all GETs) to Kratos via `KratosInternalURL`. This keeps the entire backoff flow self-contained within the webhook-gateway.

### D-007: Browser vs API Response Differentiation

The proxy detects browser requests via the `Accept: text/html` header. Browser requests that are blocked receive HTTP 303 redirect to `/login?lockout=true&retry_after=N` (which the client-web LoginPage handles). API requests receive HTTP 429 with a JSON error body. This supports both native browser form submissions and programmatic API clients without requiring the frontend to handle raw JSON error pages.

### D-008: Client-Web Lockout Display

The lockout message is injected into the Kratos flow's `ui.messages` array (id: `9000429`, type: `error`) via the existing `produce()` pattern in LoginPage.tsx. This reuses the `KratosMessages` component that already renders Kratos error alerts, ensuring consistent styling. The message ID `9000429` is in a custom range (`9xxxxxx`) outside all Kratos-reserved ranges (`1xxxxxx`–`5xxxxxx`).

## Spec Corrections Required

Based on research findings, the following spec items need correction:

1. **FR-004 response format**: Spec says `{"error": {"message": "..."}}`. Kratos expects `{"messages": [...]}` format. However, since the before-login endpoint will likely be called by a proxy (not a Kratos hook), a simpler format is used. See research.md R-002.

2. **Assumption about before-login hooks**: Spec assumes Kratos before-login hooks fire at credential submission with access to the identifier. Research (R-001) shows they fire at flow initialization without identifier access. The before-login endpoint design is correct, but the integration mechanism differs from what the spec assumed.

3. **Assumption about existing Jsonnet templates**: The templates for extracting identifier and client_ip from login flow context do not exist and cannot extract the identifier at the before-hook point. A Jsonnet template IS possible for the after-login hook (which has identity context).
