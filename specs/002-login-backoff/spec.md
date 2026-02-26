# Feature Specification: Login Backoff / Brute Force Protection

**Feature Branch**: `002-login-backoff`
**Created**: 2026-02-25
**Status**: Draft
**Input**: User description: "Add login backoff / brute force protection as a Kratos webhook in the webhook-gateway. The service receives two webhooks from Ory Kratos: (1) a before-login hook that checks whether the identifier (email) or client IP is currently locked out due to too many failed attempts, and if so interrupts the login flow; (2) an after-login hook on successful password authentication that resets the backoff counter. State (failed attempt counts, lockout expiry) is tracked in Redis. This replaces the planned standalone login-backoff-service from infra-ops with endpoints in the existing webhook-gateway, following the same domain-oriented package pattern (internal/webhooks/kratos-login-backoff/). The Kratos config already has Jsonnet templates that extract identifier and client_ip from the login flow context."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Brute Force Attempts Are Blocked After Threshold (Priority: P1)

An attacker repeatedly attempts to log in with incorrect passwords targeting a specific user account or from a single IP address. After exceeding the allowed number of failed attempts, subsequent login attempts are rejected before password verification occurs, protecting the account from brute force attacks.

**Why this priority**: This is the core security value - preventing unauthorized access through password guessing. Without this, accounts are vulnerable to automated brute force attacks.

**Independent Test**: Can be fully tested by submitting multiple failed login attempts for the same identifier and verifying that the system blocks further attempts after the configured threshold.

**Acceptance Scenarios**:

1. **Given** a user has made fewer failed login attempts than the configured threshold, **When** they attempt to log in, **Then** the login attempt proceeds normally (the before-login hook does not interrupt the flow).

2. **Given** a user has reached the configured threshold of failed login attempts for their identifier, **When** they attempt to log in again, **Then** the before-login hook interrupts the login flow and Kratos returns an error indicating the account is temporarily locked.

3. **Given** an IP address has reached the configured threshold of failed login attempts, **When** any login attempt is made from that IP, **Then** the before-login hook interrupts the login flow regardless of which account is being targeted.

4. **Given** a user is currently locked out, **When** the lockout period expires, **Then** the user can attempt to log in again normally.

---

### User Story 2 - Successful Login Resets Backoff Counter (Priority: P1)

A legitimate user who has made a few failed login attempts (below the lockout threshold) successfully logs in with the correct password. The system resets their failed attempt counter so that previous mistakes do not accumulate toward a future lockout.

**Why this priority**: Equally critical to blocking - without resetting on success, legitimate users would eventually lock themselves out through normal usage over time.

**Independent Test**: Can be tested by making several failed attempts (below threshold), then logging in successfully, and verifying the counter is reset by making fresh failed attempts without hitting lockout prematurely.

**Acceptance Scenarios**:

1. **Given** a user has accumulated some failed login attempts below the lockout threshold, **When** they log in successfully with the correct password, **Then** their failed attempt counter for that identifier is reset to zero.

2. **Given** a user logs in successfully from an IP that has accumulated failed attempts, **When** the after-login hook fires, **Then** the failed attempt counter for that IP is also reset.

---

### User Story 3 - System Degrades Gracefully on Internal Errors (Priority: P2)

When the backoff tracking system is temporarily unavailable (e.g., storage outage), the login flow is not blocked. Users can still log in, albeit without brute force protection, ensuring that an infrastructure issue does not cause a platform-wide login outage.

**Why this priority**: Availability is critical for a login system. Brute force protection is a defense-in-depth measure and must not become a single point of failure for authentication.

**Independent Test**: Can be tested by simulating storage unavailability and verifying that login attempts still proceed normally.

**Acceptance Scenarios**:

1. **Given** the backoff tracking storage is unavailable, **When** the before-login hook is called, **Then** the hook allows the login attempt to proceed (fail-open) and logs a warning for operational visibility.

2. **Given** the backoff tracking storage is unavailable, **When** the after-login hook is called, **Then** the hook completes without error (best-effort) and logs a warning.

---

### User Story 4 - Operations Team Has Visibility Into Backoff Events (Priority: P2)

The security and operations team can observe login backoff activity through structured logs, including when accounts or IPs are locked out, when lockouts expire, and when counters are reset after successful logins.

**Why this priority**: Observability is essential for security monitoring, incident response, and tuning the backoff thresholds to balance security with user experience.

**Independent Test**: Can be tested by triggering lockout and reset scenarios and verifying that corresponding structured log entries are produced with correlation IDs.

**Acceptance Scenarios**:

1. **Given** a login attempt is blocked due to lockout, **When** the before-login hook rejects the request, **Then** a structured log entry is emitted containing the identifier (anonymized/hashed), the IP address, the remaining lockout duration, and a correlation ID.

2. **Given** a successful login resets backoff counters, **When** the after-login hook fires, **Then** a structured log entry is emitted recording the reset event.

---

### Edge Cases

- **Simultaneous lockout on both identifier and IP**: If both the identifier and the IP independently exceed their thresholds, the login is blocked. The lockout with the longer remaining duration takes precedence in the error response.
- **Distributed brute force (multiple IPs, same identifier)**: The per-identifier counter catches this pattern even if no single IP exceeds its threshold.
- **Credential stuffing (single IP, many identifiers)**: The per-IP counter catches this pattern even if no single identifier exceeds its threshold.
- **Missing identifier or IP in webhook payload**: If the Kratos payload is missing the identifier or client IP, the hook allows the login to proceed (fail-open) and logs a warning. Partial data (only identifier or only IP) is still used for the available dimension.
- **Counter overflow / stale data**: Failed attempt counters have a TTL matching the lockout window, so they naturally expire and do not accumulate indefinitely.
- **OIDC/passkey logins**: The before-login hook only applies to password-based login flows. OIDC and passkey flows are not subject to backoff because they delegate authentication to external providers.
- **First request after lockout expiry**: The lockout TTL in storage governs expiry. Once the key expires, the next attempt proceeds with a clean counter.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose a before-login endpoint that checks and increments failed attempt counters before password authentication proceeds. The endpoint is caller-agnostic: it may be invoked by a reverse proxy, application server, or Kratos webhook depending on the integration layer (see research.md R-001).
- **FR-002**: System MUST track the number of failed login attempts per identifier (email address) in persistent storage with a configurable TTL.
- **FR-003**: System MUST track the number of failed login attempts per client IP address in persistent storage with a configurable TTL.
- **FR-004**: System MUST block login attempts when the failed attempt count for the identifier OR the client IP exceeds the configured threshold, by returning HTTP 403 with a JSON body `{"allowed": false, "reason": "identifier_locked" | "ip_locked", "message": "Account temporarily locked due to too many failed attempts. Try again in X minutes.", "retry_after_seconds": N}` where X is the remaining lockout duration (rounded up to whole minutes) and N is the remaining seconds. The caller (reverse proxy or application server) is responsible for adapting this response to the UI format.
- **FR-005**: System MUST expose an after-login endpoint that Ory Kratos calls after successful password authentication.
- **FR-006**: System MUST reset the failed attempt counter for both the identifier and the client IP upon receiving a successful login notification.
- **FR-007**: System MUST allow login attempts to proceed (fail-open) when the backoff tracking storage is unavailable, and log a warning.
- **FR-008**: System MUST log all backoff-related events (lockout triggered, login allowed, counter reset, storage errors) with structured fields and correlation IDs.
- **FR-013**: Before-login hook MUST respond within 100ms under normal operating conditions (single Redis round-trip budget).
- **FR-009**: System MUST support configurable thresholds for maximum failed attempts (separately for identifier and IP). Defaults: 10 attempts per identifier, 20 attempts per IP.
- **FR-010**: System MUST support separately configurable lockout durations for identifier-based and IP-based lockouts. Defaults: 2 minutes for both identifier and IP lockouts.
- **FR-011**: System MUST validate incoming webhook payloads and skip processing (fail-open) for malformed or incomplete requests, logging a warning.
- **FR-012**: System MUST atomically increment the failed attempt counter for the identifier and/or client IP on each before-login invocation. The counter is reset only when the after-login endpoint reports a successful authentication. No explicit "login failed" signal is needed — an unreset counter naturally reflects a failed attempt.

### Key Entities

- **Failed Attempt Counter**: A per-identifier and per-IP count of consecutive failed login attempts, stored with a TTL equal to the lockout duration. Automatically expires when the lockout window elapses.
- **Lockout State**: Derived from the failed attempt counter - when the count exceeds the configured threshold, the identifier or IP is considered locked out until the counter's TTL expires.
- **Before-Login Payload**: The incoming request containing an optional login flow ID (for correlation), identifier (email), and client IP address. Fields are optional to support multiple callers (reverse proxy, application server, Kratos hook). At least one of identifier or client_ip should be present.
- **After-Login Payload**: The incoming webhook from Kratos containing the identity ID, email, client IP, and a success flag, sent only after successful password authentication.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After the configured number of failed login attempts, all subsequent login attempts for that identifier are blocked until the lockout period expires.
- **SC-002**: After the configured number of failed login attempts from a single IP, all subsequent login attempts from that IP are blocked until the lockout period expires.
- **SC-003**: A successful login resets the failed attempt counter, allowing the user to make fresh attempts without inheriting previous failures.
- **SC-004**: When the backoff tracking storage is unavailable, login attempts still proceed without interruption.
- **SC-005**: All lockout and reset events are visible in structured logs with correlation IDs for security monitoring.

## Assumptions

- The before-login endpoint is called by an integration layer (reverse proxy or application server) at credential submission time — NOT by a Kratos before-hook, which fires at flow initialization before the identifier is available (see research.md R-001).
- The after-login endpoint is called by Ory Kratos via a post-login webhook (`response.ignore: true`) after successful password authentication.
- Only password-based login flows trigger these hooks. OIDC and passkey flows are handled by external providers and are not subject to backoff.
- The webhook endpoints are deployed on an internal network; no authentication mechanism is required at the application layer.
- Failed attempt counting uses an increment-on-check model: the before-login endpoint increments the counter each time it is called (representing an attempt), and the after-login endpoint resets it on success. If a login attempt fails, no explicit "failure" webhook is needed — the counter simply remains incremented.
- Jsonnet templates for extracting identifier and client_ip do not currently exist for before-login hooks and cannot work at that hook point (see research.md R-001). A Jsonnet template for the after-login hook (which has identity context) is feasible and is an infrastructure integration concern outside this service's scope.
- The existing shared infrastructure (structured logging, health checks, correlation ID middleware) in the webhook gateway is reused.

## Clarifications

### Session 2026-02-25

- Q: What are the default values for max failed attempts and lockout duration? → A: 10 attempts per identifier, 20 per IP, 2-minute lockout duration.
- Q: What error message should blocked users see? → A: Explicit message with remaining time: "Account temporarily locked due to too many failed attempts. Try again in X minutes."
- Q: Should lockout duration be shared or separate for identifier vs IP? → A: Separate configurable lockout durations for identifier and IP (both default to 2 minutes).
- Q: What is the maximum acceptable latency for the before-login webhook response? → A: 100ms under normal operating conditions.
- Q: What HTTP response format should the before-login hook use to block a login? → A: HTTP 403 with JSON body `{"allowed": false, "reason": "...", "message": "...", "retry_after_seconds": N}`. The caller adapts this to the UI format (see research.md R-002 for Kratos-specific format considerations).
