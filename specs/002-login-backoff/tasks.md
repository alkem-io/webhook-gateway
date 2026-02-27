# Tasks: Login Backoff / Brute Force Protection

**Input**: Design documents from `/specs/002-login-backoff/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/login-backoff-openapi.yaml, quickstart.md

**Tests**: Included per Constitution #4 (Test-Driven Delivery). Contract tests exercise OpenAPI behaviour; integration tests defend core invariants (lockout, reset, fail-open).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Extend existing configuration to support login backoff thresholds and lockout durations

- [X] T001 Add LoginBackoff config fields (MaxIdentifierAttempts int default 10, MaxIPAttempts int default 20, IdentifierLockoutSeconds int default 120, IPLockoutSeconds int default 120) with env var loading to the Config struct in internal/config/config.go
- [X] T002 [P] Add LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS=10, LOGIN_BACKOFF_MAX_IP_ATTEMPTS=20, LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS=120, LOGIN_BACKOFF_IP_LOCKOUT_SECONDS=120 to configs/.env.example

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Redis operations and data model types that all user stories depend on

**CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 Add login backoff Redis operations to internal/clients/redis.go: define two-key Lua script (atomic increment-with-TTL per research.md R-003) and single-key Lua script using redis.NewScript; implement IncrementLoginAttempts(ctx, identifier, ip, idTTLSeconds, ipTTLSeconds) returning (idCount, idRemaining, ipCount, ipRemaining, error) using two-key script with key patterns login_backoff:id:{email} and login_backoff:ip:{ip}; implement IncrementIdentifierAttempt(ctx, identifier, ttlSeconds) and IncrementIPAttempt(ctx, ip, ttlSeconds) returning (count, remaining, error) using single-key script; implement ResetLoginAttempts(ctx, identifier, ip) using DEL for both keys
- [X] T004 [P] Create login backoff model types in internal/webhooks/kratos-login-backoff/models.go: BeforeLoginRequest (flow_id, identifier, client_ip string with json tags), BeforeLoginAllowedResponse (allowed bool, identifier_attempts int64, ip_attempts int64), BeforeLoginBlockedResponse (allowed bool, reason string, message string, retry_after_seconds int64), AfterLoginRequest (identity_id, email, client_ip string), AfterLoginResponse (status, message string); add constants for key prefixes (LoginBackoffIdentifierPrefix, LoginBackoffIPPrefix) and response values (StatusSuccess, StatusSkipped)

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 2.5: Contract Tests (Constitution #4 Gate)

**Purpose**: Contract tests MUST exercise OpenAPI behaviour before handler implementation (Constitution #4)

**CRITICAL**: Handler implementation (Phase 3+) MUST NOT begin until contract tests exist

- [X] T015 [P] Create contract test file internal/webhooks/kratos-login-backoff/handler_contract_test.go: test POST /api/v1/webhooks/kratos/login-backoff/before-login returns 200 with BeforeLoginAllowedResponse when under threshold, returns 403 with BeforeLoginBlockedResponse when over threshold, returns 200 on malformed input (fail-open); test POST /api/v1/webhooks/kratos/login-backoff/after-login returns 200 with AfterLoginResponse; use httptest with a mock RedisClient interface; validate response schemas match contracts/login-backoff-openapi.yaml
- [X] T016 [P] Create integration test file internal/webhooks/kratos-login-backoff/service_test.go: test CheckAndIncrement lockout threshold for identifier (10 attempts → blocked), test CheckAndIncrement lockout threshold for IP (20 attempts → blocked), test lockout precedence D-005 (both locked → longer TTL wins), test ResetCounters clears both keys, test fail-open on Redis error (returns allowed=true), test partial data (identifier only, IP only, neither); use mock Redis client

**Checkpoint**: Contract and integration tests compile and define expected behaviour — handlers can now be implemented to satisfy them

---

## Phase 3: User Story 1 - Brute Force Attempts Are Blocked After Threshold (Priority: P1) MVP

**Goal**: After exceeding the allowed number of failed login attempts, subsequent attempts are rejected with a 403 response including lockout details and retry-after duration

**Independent Test**: Submit multiple POST requests to the before-login endpoint for the same identifier until the threshold is exceeded; verify the response transitions from 200/allowed to 403/blocked with correct retry_after_seconds

### Implementation for User Story 1

- [X] T005 [US1] Create Service struct (`redisClient *clients.RedisClient`, `cfg *config.Config`, `logger *zap.Logger`) with NewService constructor, and implement CheckAndIncrement method in internal/webhooks/kratos-login-backoff/service.go: accept BeforeLoginRequest, call IncrementLoginAttempts with both identifier and IP, compare returned counts against config thresholds (MaxIdentifierAttempts, MaxIPAttempts), apply lockout precedence rule D-005 (when both exceed threshold, report the one with longer remaining TTL), return BeforeLoginAllowedResponse on success or BeforeLoginBlockedResponse on lockout with human-readable message including remaining minutes, fail-open on Redis errors (return allowed=true with zero counts and log warning)
- [X] T006 [US1] Create Handler struct (`service *Service`, `logger *zap.Logger`) with NewHandler constructor, and implement HandleBeforeLogin method in internal/webhooks/kratos-login-backoff/handler.go: extract correlation ID from context, decode JSON request body into BeforeLoginRequest, call service.CheckAndIncrement, marshal response as JSON, write HTTP 200 for allowed or HTTP 403 for blocked, fail-open on JSON decode errors (return 200 with allowed=true and log warning), set Content-Type application/json header
- [X] T007 [US1] Wire login-backoff dependencies in cmd/server/main.go: create loginBackoffService using loginbackoff.NewService(redisClient, cfg, logger) and loginBackoffHandler using loginbackoff.NewHandler(loginBackoffService, logger), register POST /api/v1/webhooks/kratos/login-backoff/before-login route on the existing mux with the existing middleware chain

**Checkpoint**: Before-login endpoint is functional - can verify lockout after threshold attempts via curl per quickstart.md

---

## Phase 4: User Story 2 - Successful Login Resets Backoff Counter (Priority: P1)

**Goal**: After a successful login, the failed attempt counters for that identifier and IP are cleared so previous failures do not accumulate toward future lockouts

**Independent Test**: Submit several before-login requests (below threshold), then submit an after-login reset request; submit more before-login requests and verify the counter starts from 1 (not continuing from previous count)

### Implementation for User Story 2

- [X] T008 [US2] Add ResetCounters method to Service in internal/webhooks/kratos-login-backoff/service.go: accept AfterLoginRequest, call ResetLoginAttempts with email and client_ip to delete both Redis keys, return AfterLoginResponse with status "success" and message "counters reset", fail-open on Redis errors (return success and log warning)
- [X] T009 [US2] Add HandleAfterLogin method to Handler in internal/webhooks/kratos-login-backoff/handler.go: extract correlation ID from context, decode JSON request body into AfterLoginRequest, call service.ResetCounters, marshal AfterLoginResponse as JSON, write HTTP 200, fail-open on JSON decode errors (return 200 with status "success" and log warning), set Content-Type application/json header
- [X] T010 [US2] Register POST /api/v1/webhooks/kratos/login-backoff/after-login route on the existing mux in cmd/server/main.go

**Checkpoint**: Both endpoints are functional - can verify full lockout-reset cycle via curl per quickstart.md

---

## Phase 5: User Story 3 - System Degrades Gracefully on Internal Errors (Priority: P2)

**Goal**: When inputs are missing or malformed, the system allows login attempts to proceed (fail-open) and logs warnings for operational visibility rather than blocking users

**Independent Test**: Submit before-login requests with only identifier (no IP), only IP (no identifier), and neither; verify all return 200/allowed. Submit after-login requests with missing email or IP; verify all return 200/success or 200/skipped.

### Implementation for User Story 3

- [X] T011 [US3] Add input validation and partial data support to CheckAndIncrement and ResetCounters in internal/webhooks/kratos-login-backoff/service.go: in CheckAndIncrement, normalize identifier to lowercase, route to IncrementIdentifierAttempt when only identifier is provided, route to IncrementIPAttempt when only IP is provided, return allowed=true with zero counts when neither is provided with warning log; in ResetCounters, build key list from available fields (email and/or client_ip), skip Redis call and return status "skipped" with message "no identifier or IP provided" when neither is present with warning log

**Checkpoint**: All fail-open and partial data scenarios work correctly

---

## Phase 6: User Story 4 - Operations Team Has Visibility Into Backoff Events (Priority: P2)

**Goal**: All backoff-related events are visible in structured logs with fields sufficient for security monitoring, incident response, and threshold tuning

**Independent Test**: Trigger a lockout and a counter reset; verify structured log output contains anonymized identifier, IP, attempt counts, remaining TTL, and correlation ID

### Implementation for User Story 4

- [X] T012 [US4] Enrich structured log entries in service methods in internal/webhooks/kratos-login-backoff/service.go: add zap fields for all log statements in CheckAndIncrement (zap.String("correlation_id", ...), zap.String("identifier_hash", sha256_prefix_of_email), zap.String("client_ip", ip), zap.Int64("identifier_attempts", count), zap.Int64("ip_attempts", count), zap.Int("identifier_threshold", cfg.MaxIdentifierAttempts), zap.Int("ip_threshold", cfg.MaxIPAttempts), zap.Int64("retry_after_seconds", remaining), zap.String("lockout_reason", reason)); add zap fields for ResetCounters log statements (correlation_id, identifier_hash, client_ip, event "counters_reset"); use first 8 chars of SHA-256 hex digest for identifier anonymization

**Checkpoint**: All backoff events produce structured log entries with complete operational context

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Contract documentation and final validation

- [X] T013 [P] Merge login-backoff endpoint definitions from specs/002-login-backoff/contracts/login-backoff-openapi.yaml into contracts/openapi.yaml: add both paths (/api/v1/webhooks/kratos/login-backoff/before-login and /api/v1/webhooks/kratos/login-backoff/after-login) with all request/response schemas and examples
- [X] T014 Run make lint and make test to verify all code compiles, passes golangci-lint, and existing tests still pass

---

## Phase 8: Reverse Proxy Integration (Post-Spec)

**Purpose**: Implement the before-login integration layer as a built-in reverse proxy per research.md R-001 Option A

- [X] T017 [US1] Create reverse proxy handler in internal/webhooks/kratos-login-backoff/proxy.go: implement NewLoginProxy(kratosURL, service, logger) using httputil.NewSingleHostReverseProxy, intercept POST requests to extract identifier from JSON/form body and client IP from headers (True-Client-Ip, X-Forwarded-For, X-Real-Ip, RemoteAddr), check backoff via service.CheckAndIncrement, return 429 JSON for API clients or 303 redirect to /login?lockout=true&retry_after=N for browser clients (Accept: text/html), proxy allowed requests and all GET requests to Kratos
- [X] T018 [US1] Add KratosInternalURL config field (env: KRATOS_INTERNAL_URL, default: http://kratos:4433) to internal/config/config.go and KRATOS_INTERNAL_URL to configs/.env.example
- [X] T019 [US1] Wire login proxy routes in cmd/server/main.go: register /self-service/login and /self-service/login/ handlers using NewLoginProxy with cfg.KratosInternalURL
- [X] T020 [US1] Add webhook-gateway service and kratos-login-backoff router (priority 200) to server/.build/traefik/http.yml, add KRATOS_INTERNAL_URL env to webhook-gateway in server/quickstart-services.yml

---

## Phase 9: Client-Web Lockout Display (Post-Spec)

**Purpose**: Display user-friendly lockout message in the login UI when blocked by the proxy

- [X] T021 [US1] Add lockout query param handling to client-web/src/core/auth/authentication/pages/LoginPage.tsx: read lockout and retry_after query params, inject error message (id: 9000429, type: error) into flow ui.messages via existing produce() pattern for KratosMessages rendering
- [X] T022 [US1] Add authentication.lockout translation key to client-web/src/core/i18n/en/translation.en.json

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup (T001 for config fields used in Redis TTLs) - BLOCKS all user stories
- **Contract Tests (Phase 2.5)**: Depends on Foundational (T004 for model types used in test assertions) - BLOCKS handler implementation
- **User Story 1 (Phase 3)**: Depends on Foundational (T003, T004) and Contract Tests (T015, T016)
- **User Story 2 (Phase 4)**: Depends on Foundational (T003, T004) and US1 (T005 for Service struct, T006 for Handler struct, T007 for main.go wiring)
- **User Story 3 (Phase 5)**: Depends on US2 completion (T008 for ResetCounters method to enhance)
- **User Story 4 (Phase 6)**: Depends on US3 completion (T011 modifies service.go, T012 adds to same file)
- **Polish (Phase 7)**: T013 can start after Phase 2; T014 depends on all code phases complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - creates service.go, handler.go, modifies main.go
- **User Story 2 (P1)**: Depends on US1 - adds methods to service.go and handler.go created in US1, adds route to main.go modified in US1
- **User Story 3 (P2)**: Depends on US2 - modifies service.go methods from US1 and US2
- **User Story 4 (P2)**: Depends on US3 - enriches log entries in service.go after US3 validation logic is in place

### Within Each Phase

- Models (T004) before services (T005, T008)
- Services before handlers (T005→T006, T008→T009)
- Handlers before route registration (T006→T007, T009→T010)
- Core implementation before enhancement (US1/US2 before US3/US4)

### Parallel Opportunities

- T001 and T002 can run in parallel (Phase 1: different files)
- T003 and T004 can run in parallel (Phase 2: different files)
- T013 can run in parallel with any code phase (Phase 7: different file, no code dependencies)

---

## Parallel Example: Foundational Phase

```text
# Launch both foundational tasks together (different files, no dependencies):
Task T003: "Add login backoff Redis operations to internal/clients/redis.go"
Task T004: "Create login backoff model types in internal/webhooks/kratos-login-backoff/models.go"
```

## Parallel Example: Setup Phase

```text
# Launch both setup tasks together (different files, no dependencies):
Task T001: "Add LoginBackoff config fields to internal/config/config.go"
Task T002: "Add LOGIN_BACKOFF_* env vars to configs/.env.example"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (config fields + env example)
2. Complete Phase 2: Foundational (Redis operations + models) - CRITICAL, blocks all stories
3. Complete Phase 3: User Story 1 (before-login endpoint)
4. **STOP and VALIDATE**: Test before-login endpoint with curl per quickstart.md
5. At this point, brute force protection is functional (blocking works)

### Incremental Delivery

1. Complete Setup + Foundational -> Foundation ready
2. Add User Story 1 -> Test lockout behavior -> Deploy (MVP - blocking works!)
3. Add User Story 2 -> Test reset behavior -> Deploy (counter reset on success)
4. Add User Story 3 -> Test edge cases -> Deploy (resilient to bad input)
5. Add User Story 4 -> Verify logs -> Deploy (full observability)
6. Polish -> Merge contracts, run full validation

### Sequential Execution (Single Developer)

This feature is best executed sequentially due to shared files:
1. Phase 1 + Phase 2 (foundation)
2. Phase 3: US1 (creates service.go, handler.go)
3. Phase 4: US2 (adds methods to same files)
4. Phase 5: US3 (modifies same service.go)
5. Phase 6: US4 (enriches same service.go)
6. Phase 7: Polish (contracts + validation)

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- US2 depends on US1 because both write to the same files (service.go, handler.go, main.go)
- US3 and US4 are enhancement phases that modify code written in US1/US2
- Fail-open behavior is built into US1/US2 from the start (matching existing kratos-verification pattern)
- No new dependencies required - uses existing go-redis and zap
- Lua scripts from research.md R-003 are pre-designed and tested
- Key patterns follow data-model.md: login_backoff:id:{email}, login_backoff:ip:{ip}
- Lockout precedence rule (D-005): when both dimensions locked, report the one with longer remaining TTL
- Commit after each task or logical group
- Stop at any checkpoint to validate independently
