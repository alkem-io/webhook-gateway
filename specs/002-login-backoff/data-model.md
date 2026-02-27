# Data Model: Login Backoff / Brute Force Protection

**Feature**: 002-login-backoff
**Date**: 2026-02-25

## Entities

### Failed Attempt Counter (Redis)

Tracks consecutive failed login attempts per dimension (identifier or IP). Stored as a Redis key with integer value and TTL-based expiry.

**Key patterns**:
- `login_backoff:id:{email}` - per-identifier counter (email, lowercased)
- `login_backoff:ip:{ip}` - per-IP counter (raw IP string)

**Value**: Integer (failed attempt count), managed atomically via Lua script.

**TTL**: Set on first increment, equals the configured lockout duration (default 120 seconds). The counter naturally expires when the lockout window elapses. No explicit "lockout state" entity is needed - lockout is derived from `count > threshold`.

**Lifecycle**:
1. Created (count=1, TTL set) on first login attempt for this dimension
2. Incremented on each subsequent attempt within the TTL window
3. Reset (key deleted) on successful login
4. Expires automatically when TTL elapses

### Before-Login Request

Incoming request to check and increment login attempt counters.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `flow_id` | string (UUID) | No | Kratos login flow ID (for correlation logging) |
| `identifier` | string (email) | No | Login identifier (email). If absent, only IP check is performed. |
| `client_ip` | string (IP) | No | Client IP address. If absent, only identifier check is performed. |

At least one of `identifier` or `client_ip` must be present. If neither is provided, the request is skipped (fail-open) with a warning log.

### Before-Login Response (Allowed)

| Field | Type | Description |
|-------|------|-------------|
| `allowed` | boolean | `true` - login attempt may proceed |
| `identifier_attempts` | integer | Current failed attempt count for identifier (0 if not tracked) |
| `ip_attempts` | integer | Current failed attempt count for IP (0 if not tracked) |

### Before-Login Response (Blocked)

| Field | Type | Description |
|-------|------|-------------|
| `allowed` | boolean | `false` - login attempt is blocked |
| `reason` | string | `"identifier"` or `"ip"` - which dimension triggered the lockout |
| `message` | string | Human-readable lockout message with remaining time |
| `retry_after_seconds` | integer | Seconds until lockout expires |

### After-Login Request

Incoming request to reset counters after successful authentication.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `identity_id` | string (UUID) | No | Kratos identity ID (for correlation logging) |
| `email` | string (email) | No | Login identifier to reset. |
| `client_ip` | string (IP) | No | Client IP to reset. |

### After-Login Response

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always `"success"` |
| `message` | string | `"counters reset"` |

## Configuration

Added to `internal/config/config.go`:

| Config Field | Env Variable | Type | Default | Description |
|-------------|-------------|------|---------|-------------|
| `LoginBackoffMaxIdentifierAttempts` | `LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS` | int | 10 | Max failed attempts per identifier before lockout |
| `LoginBackoffMaxIPAttempts` | `LOGIN_BACKOFF_MAX_IP_ATTEMPTS` | int | 20 | Max failed attempts per IP before lockout |
| `LoginBackoffIdentifierLockoutSeconds` | `LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS` | int | 120 | Lockout duration for identifier (seconds) |
| `LoginBackoffIPLockoutSeconds` | `LOGIN_BACKOFF_IP_LOCKOUT_SECONDS` | int | 120 | Lockout duration for IP (seconds) |
| `KratosInternalURL` | `KRATOS_INTERNAL_URL` | string | `http://kratos:4433` | Kratos backend URL for the reverse proxy |

## Reverse Proxy (Login Interception)

The reverse proxy (`proxy.go`) intercepts Kratos login POST submissions routed via Traefik. It extracts identifier and client IP directly from the HTTP request rather than from a webhook payload.

### Proxy Request Extraction

| Source | Field | Extraction Method |
|--------|-------|-------------------|
| Request body (JSON) | `identifier` | `json:"identifier"` field from Kratos login body |
| Request body (form) | `identifier` | `identifier` form field |
| Request headers | `client_ip` | True-Client-Ip → X-Forwarded-For (first) → X-Real-Ip → RemoteAddr |

### Proxy Blocked Response (API - Accept: application/json)

HTTP 429 with JSON body:

| Field | Type | Description |
|-------|------|-------------|
| `error.code` | integer | `429` |
| `error.status` | string | `"Too Many Requests"` |
| `error.reason` | string | `"identifier"` or `"ip"` |
| `error.message` | string | Human-readable lockout message |

### Proxy Blocked Response (Browser - Accept: text/html)

HTTP 303 redirect to `/login?lockout=true&retry_after=N` where N is remaining lockout seconds.

## Redis Operations

### IncrementAndCheck (Lua script, single round-trip)

**Input**: identifier key, IP key, identifier TTL, IP TTL
**Output**: identifier count, identifier TTL remaining, IP count, IP TTL remaining

Atomically increments both counters. Sets TTL only on first increment (fixed window). Returns current counts and remaining TTLs for threshold comparison in Go code.

### ResetCounters (DEL)

**Input**: identifier key, IP key
**Output**: none

Deletes both keys atomically. Used by the after-login handler on successful authentication.

## State Transitions

```text
                    ┌─────────────────────────────────┐
                    │     No Counter (key absent)      │
                    │     State: CLEAN                 │
                    └──────┬──────────────┬────────────┘
                           │              │
                   attempt │              │ TTL expires
                   (INCR)  │              │ (automatic)
                           ▼              │
                    ┌──────────────────┐  │
              ┌────►│  Counter Active   │──┘
              │     │  count < threshold│
              │     │  State: TRACKING  │
              │     └──────┬───────────┘
              │            │
     attempt  │   count >= │ threshold
     (INCR)   │            │ (INCR)
              │            ▼
              │     ┌──────────────────┐
              │     │  Counter Active   │
              └─────│  count >= threshold│──► TTL expires → CLEAN
                    │  State: LOCKED OUT │
                    └──────────────────┘
                           │
                    success│ login
                    (DEL)  │
                           ▼
                    ┌──────────────────┐
                    │     CLEAN         │
                    └──────────────────┘
```
