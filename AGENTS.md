# AI Agent Guidelines for Alkemio Webhook Gateway

This document provides essential guidance for AI coding assistants working on this codebase.

## Core Engineering Principles

### Always Root Cause Analysis
- **Never apply opportunistic or speculative fixes** hoping they might resolve an issue
- Before any bug fix, identify and document the actual root cause with evidence
- If the root cause is unclear, invest time in debugging first — guessing wastes more time than investigating
- Fixes must directly address the identified root cause, not symptoms

### No Legacy Code
- **Never silently assume backward compatibility is required** — we control the full stack and all consumers
- Do not leave code "just in case" — dead, deprecated, or unused code must be removed
- When a feature requires changes across multiple services, coordinate those changes rather than maintaining compatibility shims
- Remove backward-compatibility hacks, unused exports, commented-out code, and defensive code for scenarios that no longer apply
- Every line of code must justify its existence

### Single Source of Truth
- **No two methods should implement the same logic** in different modules
- If duplication exists, extract to a single shared utility
- When methods share partial logic, extract the common part to a shared helper
- Before implementing new logic, search for existing implementations — extend rather than duplicate
- Configuration, constants, and type definitions live in one canonical location

### No Busywork
- Every task, test, and artifact must deliver demonstrable value
- Reject make-work activities that exist only to satisfy process checkboxes
- Do not create documentation, comments, or abstractions "just in case"
- Specifications should be lean: only what is necessary to communicate intent

### Meaningful Tests Only
- Tests must defend real invariants or catch real regressions
- Never write tests for the sake of coverage metrics
- Avoid testing implementation details, trivial getters/setters, or scenarios that cannot fail
- If a test does not help catch bugs or document critical behaviour, do not write it

### Meaningful Success Criteria
- Success criteria must be directly testable within this service
- Never invent arbitrary metrics pulled from thin air (e.g., "50ms response time", "40% improvement") without baseline measurements or explicit stakeholder requirements
- Avoid vanity metrics or external business outcomes that cannot be validated during development

### Latest Dependencies Always
- When adding or updating any dependency, **always verify the latest stable version online** (pkg.go.dev, GitHub releases, npm registry, etc.)
- **Never rely on AI training data for version numbers** — it is likely outdated
- Pin dependencies to specific versions, but ensure those versions are current at time of addition

### No Assumptions
- **Never assume** requirements, behavior, or implementation details that are not explicitly defined
- If something is unclear or unknown, **ask the user** for clarification before proceeding
- If factual information is needed (versions, API specs, library behavior), **search online** to verify
- Do not guess — guessing leads to rework; asking or searching takes less time than fixing wrong assumptions

## Active Technologies

- **Language**: Go 1.24
- **Logging**: go.uber.org/zap (structured JSON logging)
- **Message Broker**: github.com/rabbitmq/amqp091-go (RabbitMQ, NestJS envelope format)
- **Cache/Idempotency**: github.com/redis/go-redis/v9 (Redis)
- **Configuration**: github.com/joho/godotenv (env-based config)

## Architecture Quick Reference

```text
cmd/server/              # Application entrypoint
configs/                 # Environment config examples (.env.example)
contracts/               # OpenAPI specs
internal/
├── config/              # Typed configuration loading
├── health/              # Health check handlers (/health/live, /health/ready)
├── middleware/           # HTTP middleware (correlation ID, logging, recovery)
├── clients/             # Redis and RabbitMQ clients
│   ├── redis.go         # Idempotency key management (welcome_sent:{id})
│   └── rabbitmq.go      # RabbitMQ publisher with NestJS envelope wrapping
└── webhooks/
    └── kratos-verification/  # Kratos post-verification webhook handler
        ├── handler.go        # HTTP handler (always returns 200 for fail-open)
        ├── service.go        # Business logic: validate, deduplicate, transform, publish
        └── models.go         # Request/response types, event payload structs
manifests/               # K8s deployment manifests for CI/CD
```

## Key Patterns

1. **Fail-Open Semantics**: Always return HTTP 200 to Kratos, even on internal errors — verification must never be blocked by webhook failures
2. **Idempotency via Redis**: Each webhook is deduplicated using `welcome_sent:{identity_id}` keys with 90-day TTL
3. **NestJS Envelope Format**: RabbitMQ messages are wrapped as `{"pattern": "<event>", "data": {...}}` for compatibility with the NestJS notifications service
4. **Domain-Oriented Packages**: Each webhook type gets its own package under `internal/webhooks/`
5. **Deterministic Configuration**: All settings via env vars, with `REDIS_URL` taking precedence over `REDIS_HOST` + `REDIS_PORT`
6. **Observability**: Zap structured logging with correlation IDs propagated via middleware

## What NOT to Do

- Do not apply speculative fixes — find root cause first
- Do not keep code "just in case" or for backward compatibility unless explicitly requested
- Do not duplicate logic — find or create a single shared implementation
- Do not add superficial tests for coverage padding
- Do not invent performance SLAs without evidence
- Do not create abstractions for hypothetical future needs
- Do not add comments explaining obvious code
- Do not rely on training data for dependency versions — check online
- Do not create documentation files unless explicitly requested
- Do not assume — ask or search when something is unclear
