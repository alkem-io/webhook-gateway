# Implementation Plan: Ory Kratos Post-Verification Webhook

**Branch**: `001-kratos-verification-webhook` | **Date**: 2026-01-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-kratos-verification-webhook/spec.md`

## Summary

Implement an HTTP webhook endpoint that receives Ory Kratos post-verification webhooks after successful email verification. The webhook gateway extracts user identity information from the Kratos payload, constructs a `UserSignupWelcome` notification event, and publishes it to RabbitMQ for consumption by the Alkemio notifications service. Redis provides once-per-identity idempotency tracking to prevent duplicate welcome emails.

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: go.uber.org/zap (logging), github.com/redis/go-redis/v9 (Redis), github.com/rabbitmq/amqp091-go (RabbitMQ), github.com/joho/godotenv (config)
**Storage**: Redis (idempotency tracking - key-value store for "welcome sent" status)
**Testing**: go test with httptest for contract/integration tests
**Target Platform**: Linux server (Docker container, Kubernetes deployment)
**Project Type**: Single Go service
**Performance Goals**: N/A (per Constitution #7 - latency dominated by external services)
**Constraints**: Webhook response to Kratos should not block on RabbitMQ/Redis availability (fail-open semantics)
**Scale/Scope**: Single webhook endpoint, internal network only (no authentication required)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| #1 Deterministic Configuration | PASS | All config via typed `Config` struct in `internal/config` |
| #2 Operational Observability | PASS | Zap structured logging with correlation IDs |
| #3 Fail-Fast Maintenance | PASS | Maintenance middleware returns 503 immediately |
| #4 Test-Driven Delivery | PASS | Contract tests before handler implementation |
| #5 Secure Build Pipeline | PASS | Multi-stage Docker, distroless runtime |
| #6 CI Integrity | PASS | GitHub Actions with linting, tests, SBOM |
| #7 Evidence-Based Performance | PASS | No arbitrary SLAs; timeout budgets instead |
| #8 Meaningful Success Criteria | PASS | Testable outcomes within webhook handler |
| #9 No Busywork | PASS | Lean artifacts, no unnecessary documentation |
| #10 Meaningful Tests Only | PASS | Contract tests for OpenAPI, integration for flows |
| #11 Latest Dependencies | PASS | Will verify latest versions from pkg.go.dev |
| #12 Always Root Cause | N/A | New implementation, no bugs to fix |
| #13 No Legacy Code | PASS | Greenfield project, no backward compat |
| #14 Single Source of Truth | PASS | Shared types in single location |
| #15 No Assumptions | PASS | All unknowns researched and documented |
| #16 Webhook Segregation | PASS | `internal/webhooks/kratos-verification/` folder |

## Project Structure

### Documentation (this feature)

```text
specs/001-kratos-verification-webhook/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── openapi.yaml
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
└── server/
    └── main.go              # Entrypoint: config loading, HTTP server start

internal/
├── config/
│   ├── config.go            # Typed configuration struct
│   └── logger.go            # Zap logger factory
├── webhooks/
│   └── kratos-verification/
│       ├── handler.go       # HTTP handler for POST /api/v1/webhooks/kratos/verification
│       ├── models.go        # Request/response types (Kratos payload, notification event)
│       └── service.go       # Business logic: validation, event construction, idempotency
├── clients/
│   ├── rabbitmq.go          # RabbitMQ publisher client
│   └── redis.go             # Redis client for idempotency tracking
├── middleware/
│   ├── logging.go           # Request logging with correlation IDs
│   ├── correlation.go       # Correlation ID extraction/generation
│   └── maintenance.go       # Maintenance mode 503 responses
└── health/
    └── handlers.go          # /health/live and /health/ready endpoints

pkg/
└── telemetry/
    └── logger.go            # Shared logging utilities (exported)

contracts/
└── openapi.yaml             # OpenAPI 3.0 specification

test/
├── contract/
│   └── webhook_contract_test.go
└── integration/
    └── webhook_flow_test.go

configs/
├── .env.example
└── README.md

Dockerfile
Makefile
.golangci.yml
go.mod
go.sum
```

**Structure Decision**: Single Go service following Constitution webhook segregation principle. The `internal/webhooks/kratos-verification/` folder contains all webhook-specific code (handler, models, service). Shared utilities (clients, middleware, health) remain in separate packages.

## Complexity Tracking

No violations to justify. Design follows all Constitution principles with no exceptions required.
