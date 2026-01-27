# Tasks: Ory Kratos Post-Verification Webhook

**Input**: Design documents from `/specs/001-kratos-verification-webhook/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/openapi.yaml

**Tests**: Not explicitly requested in the feature specification. Test tasks are omitted.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

## Path Conventions

- **Go project**: `cmd/`, `internal/`, `pkg/`, `test/` at repository root (per plan.md)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, Go module setup, and base configuration

- [X] T001 Initialize Go 1.24 module with `go mod init` in project root (go.mod)
- [X] T002 Add dependencies (verified latest as of 2026-01-26): go.uber.org/zap@v1.27.1, github.com/redis/go-redis/v9@v9.17.3, github.com/rabbitmq/amqp091-go@v1.10.0, github.com/joho/godotenv@v1.5.1, github.com/google/uuid@v1.6.0
- [X] T003 [P] Create .golangci.yml linter configuration in project root
- [X] T004 [P] Create Makefile with build, test, lint, run targets in project root
- [X] T005 [P] Create configs/.env.example with all environment variables
- [X] T006 [P] Create Dockerfile with multi-stage build (gcr.io/distroless/static-debian12 runtime) in project root

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T007 Implement typed Config struct with environment loading in internal/config/config.go
- [X] T008 Implement Zap logger factory with JSON/console modes in internal/config/logger.go
- [X] T009 [P] Implement correlation ID extraction/generation middleware in internal/middleware/correlation.go
- [X] T010 [P] Implement request logging middleware with correlation IDs in internal/middleware/logging.go
- [X] T011 [P] Implement maintenance mode middleware (503 responses) in internal/middleware/maintenance.go
- [X] T012 Implement Redis client wrapper in internal/clients/redis.go
- [X] T013 Implement RabbitMQ publisher client in internal/clients/rabbitmq.go
- [X] T014 [P] Implement health check handlers (/health/live, /health/ready) in internal/health/handlers.go
- [X] T015 Create HTTP server entrypoint with graceful shutdown in cmd/server/main.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - New User Receives Welcome Email After Verification (Priority: P1) 🎯 MVP

**Goal**: When a user verifies their email via Ory Kratos, the webhook gateway receives the post-verification webhook, extracts user information, and publishes a UserSignupWelcome notification event to RabbitMQ.

**Independent Test**: Send a POST request to `/api/v1/webhooks/kratos/verification` with a valid Kratos payload and verify the UserSignupWelcome event is published to the `alkemio-notifications` RabbitMQ queue.

### Implementation for User Story 1

- [X] T016 [P] [US1] Create KratosVerificationPayload model (request) in internal/webhooks/kratos-verification/models.go
- [X] T017 [P] [US1] Create UserPayload, ProfileInfo, PlatformInfo models in internal/webhooks/kratos-verification/models.go
- [X] T018 [P] [US1] Create UserSignupWelcomeEvent model (RabbitMQ message) in internal/webhooks/kratos-verification/models.go
- [X] T019 [P] [US1] Create WebhookResponse model in internal/webhooks/kratos-verification/models.go
- [X] T020 [US1] Implement payload validation logic in internal/webhooks/kratos-verification/service.go
- [X] T021 [US1] Implement TransformToNotificationEvent function in internal/webhooks/kratos-verification/service.go
- [X] T022 [US1] Implement Redis idempotency check (MarkWelcomeSentIfNew) in internal/webhooks/kratos-verification/service.go
- [X] T023 [US1] Implement RabbitMQ event publishing logic in internal/webhooks/kratos-verification/service.go
- [X] T024 [US1] Implement HTTP handler for POST /api/v1/webhooks/kratos/verification in internal/webhooks/kratos-verification/handler.go
- [X] T025 [US1] Register webhook route in cmd/server/main.go
- [X] T026 [US1] Add structured logging with correlation IDs for webhook processing in internal/webhooks/kratos-verification/handler.go

**Checkpoint**: At this point, User Story 1 should be fully functional - verified users receive welcome email notifications

---

## Phase 4: User Story 2 - System Handles Verification Failures Gracefully (Priority: P2)

**Goal**: When downstream services (RabbitMQ, Redis) are unavailable or payloads are malformed, the webhook gateway handles errors gracefully without blocking the Kratos verification flow.

**Independent Test**: Simulate RabbitMQ unavailability and verify the webhook returns HTTP 200 to Kratos. Send malformed payloads and verify appropriate status/message responses.

### Implementation for User Story 2

- [X] T027 [US2] Implement fail-open semantics for RabbitMQ errors (log warning, return success) in internal/webhooks/kratos-verification/service.go
- [X] T028 [US2] Implement fail-open semantics for Redis errors (log warning, proceed with notification) in internal/webhooks/kratos-verification/service.go
- [X] T029 [US2] Add comprehensive validation error handling with descriptive messages in internal/webhooks/kratos-verification/handler.go
- [X] T030 [US2] Add structured error logging for all failure scenarios in internal/webhooks/kratos-verification/service.go

**Checkpoint**: At this point, User Stories 1 AND 2 are complete - the system handles both happy path and error scenarios

---

## Phase 5: Edge Cases (Cross-Cutting)

**Purpose**: Handle re-verification and duplicate webhook scenarios

- [X] T031 Ensure duplicate webhook deliveries trigger at-most-once notification via Redis idempotency in internal/webhooks/kratos-verification/service.go
- [X] T032 Add warning log when skipping notification due to missing traits in internal/webhooks/kratos-verification/handler.go
- [X] T033 Add info log when skipping notification due to already-sent welcome email in internal/webhooks/kratos-verification/handler.go

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final improvements and deployment readiness

- [X] T034 [P] Copy contracts/openapi.yaml to repository root contracts/ directory
- [X] T035 [P] Create configs/README.md with configuration documentation
- [X] T036 Validate implementation against quickstart.md test scenarios
- [X] T037 Run golangci-lint and fix any issues
- [X] T038 Build Docker image and verify startup

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) completion
- **User Story 2 (Phase 4)**: Depends on User Story 1 core implementation (T020-T024)
- **Edge Cases (Phase 5)**: Depends on User Story 1 (Phase 3) completion
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Builds on User Story 1 implementation but adds error handling behavior

### Within Each User Story

- Models before services (T016-T019 before T020-T024)
- Services before handlers (T020-T023 before T024)
- Handler before route registration (T024 before T025)

### Parallel Opportunities

**Phase 1 - Setup**:
```bash
# These can run in parallel:
T003: Create .golangci.yml
T004: Create Makefile
T005: Create configs/.env.example
T006: Create Dockerfile
```

**Phase 2 - Foundational**:
```bash
# These can run in parallel (after T007, T008):
T009: Implement correlation ID middleware
T010: Implement logging middleware
T011: Implement maintenance middleware
T014: Implement health handlers

# These can run in parallel:
T012: Implement Redis client
T013: Implement RabbitMQ client
```

**Phase 3 - User Story 1**:
```bash
# All models can be created in parallel:
T016: KratosVerificationPayload model
T017: UserPayload, ProfileInfo, PlatformInfo models
T018: UserSignupWelcomeEvent model
T019: WebhookResponse model
```

---

## Parallel Example: User Story 1 Models

```bash
# Launch all models for User Story 1 together:
Task: "Create KratosVerificationPayload model in internal/webhooks/kratos-verification/models.go"
Task: "Create UserPayload, ProfileInfo, PlatformInfo models in internal/webhooks/kratos-verification/models.go"
Task: "Create UserSignupWelcomeEvent model in internal/webhooks/kratos-verification/models.go"
Task: "Create WebhookResponse model in internal/webhooks/kratos-verification/models.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Test webhook end-to-end with Kratos payload
5. Deploy/demo if ready - users receive welcome emails after verification

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 → Error handling hardened → Deploy/Demo
4. Add Edge Cases → Idempotency fully validated
5. Polish → Production ready

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- All models live in single models.go file (Go convention) but are marked [P] as they define independent types
- Fail-open semantics: Always return HTTP 200 to Kratos regardless of downstream failures
- Redis key format: `welcome_sent:{identity_id}` with 90-day TTL
- RabbitMQ queue: `alkemio-notifications` (durable)
