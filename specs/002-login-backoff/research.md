# Research: Login Backoff / Brute Force Protection

**Feature**: 002-login-backoff
**Date**: 2026-02-25

## R-001: Kratos Before-Login Hook Timing (CRITICAL)

**Decision**: The before-login endpoint cannot rely on Kratos `before` hooks for per-identifier rate limiting.

**Rationale**: Ory Kratos v1.3.1 `before` hooks for the `login` flow fire at **flow initialization** (when the login page loads), NOT at credential submission. This means:

1. `PreLoginHook` is called inside `NewLoginFlow` (flow creation handler), which runs on `GET /self-service/login/browser`.
2. When the user submits credentials (`POST /self-service/login?flow=<id>`), no before-hook fires. Only `PostLoginHook` fires after successful authentication.
3. The Jsonnet context for before-login hooks does NOT contain the submitted identifier (email) because the user hasn't entered anything yet.
4. There is no Kratos hook that fires after credential submission but before authentication. GitHub issue [ory/kratos#3580](https://github.com/ory/kratos/issues/3580) explicitly identifies this gap.

**Available in before-login Jsonnet context** (`ctx`):
- `ctx.flow.id` - login flow UUID
- `ctx.request_headers` - filtered through allowlist (includes `True-Client-Ip` but NOT `X-Forwarded-For`)
- `ctx.request_url`, `ctx.request_method`, `ctx.request_cookies`
- NOT available: `ctx.identity`, `ctx.session`, submitted form values

**Impact on spec assumptions**:
- Spec assumption "before-login hook that checks whether the identifier (email) or client IP is currently locked out" is **partially incorrect**: the hook cannot check per-identifier because the identifier is not available.
- Spec assumption "existing Jsonnet templates that extract identifier and client_ip" is **incorrect**: these templates do not exist and cannot extract the identifier at this hook point.
- The increment-on-check model (FR-012) assumes the before-hook fires per login attempt. In reality, it fires per flow creation (typically once per login page visit).

**Alternatives considered**:

| Approach | Pros | Cons |
|----------|------|------|
| A. Reverse proxy intercepting login POST | Full access to identifier + IP, transparent to Kratos | Requires infrastructure routing changes |
| B. Alkemio server calls check/failure endpoints | No infra changes if server already proxies login | Requires Alkemio server code changes |
| C. Before-hook for IP only + no identifier tracking | Works with current Kratos hooks | Doesn't protect against distributed attacks on single account |
| D. Wait for Kratos issue #3580 | Clean integration | Unknown timeline, blocks feature delivery |

**Recommended approach**: **Option A or B** - Build the webhook-gateway endpoints to accept identifier and IP from any caller. The after-login hook (success) works as designed via Kratos webhook. The before-login check requires an integration layer (reverse proxy or server-side call) to be called at credential submission time. This is an infrastructure integration concern outside the webhook-gateway's scope.

**Sources**:
- [Kratos login handler source (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/selfservice/flow/login/handler.go) - `NewLoginFlow` calls `PreLoginHook`
- [Kratos webhook hook source (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/selfservice/hook/web_hook.go) - `ExecuteLoginPreHook` context construction
- [GitHub Issue #3580: New Ory Action: after login attempt](https://github.com/ory/kratos/issues/3580)
- [Kratos config schema (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/embedx/config.schema.json) - `selfservice.flows.login.before.hooks`

---

## R-002: Kratos Webhook Error Response Format

**Decision**: Use the Kratos `messages` array format for webhook error responses, not the `{"error": {"message": "..."}}` format stated in the spec.

**Rationale**: When a Kratos webhook with `can_interrupt: true` (deprecated) or `response.parse: true` returns a non-2xx status, Kratos parses the response body using this structure:

```json
{
  "messages": [
    {
      "instance_ptr": "#/",
      "messages": [
        {
          "id": 4000001,
          "text": "Account temporarily locked due to too many failed attempts. Try again in 2 minutes.",
          "type": "error",
          "context": {}
        }
      ]
    }
  ]
}
```

If the response doesn't match this format, Kratos falls back to a generic HTTP 502 error: "A third-party upstream service responded improperly."

**Note**: The `can_interrupt` config option is deprecated in v1.3.x. The replacement is `response.parse: true`. Both trigger the same response parsing logic.

**Impact on spec**: FR-004 specifies `{"error": {"message": "..."}}` which will NOT be parsed by Kratos. The endpoint should return the Kratos messages format instead. However, since the before-login endpoint may be called by a proxy rather than a Kratos hook (see R-001), the response format should be simple and caller-agnostic. The integration layer is responsible for adapting the response to whatever the frontend/Kratos expects.

**Sources**:
- [Kratos webhook source (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/selfservice/hook/web_hook.go) - `parseWebhookResponse` function
- [Kratos config schema (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/embedx/config.schema.json) - `response.parse` deprecates `can_interrupt`

---

## R-003: Redis Atomic Counter Pattern

**Decision**: Use a Lua script via `redis.NewScript` for atomic increment-with-TTL operations.

**Rationale**: The naive `INCR` + `EXPIRE` two-command approach has a documented race condition: if the process crashes between commands, the key is created without a TTL, causing a permanent lockout. Redis official documentation explicitly recommends a Lua script for this pattern ([redis.io/commands/incr - Pattern: Rate limiter 2](https://redis.io/docs/latest/commands/incr/)).

**Lua script** (fixed-window, TTL set only on first increment):
```lua
local key = KEYS[1]
local ttl_seconds = tonumber(ARGV[1])
local count = redis.call('INCR', key)
if count == 1 then
    redis.call('EXPIRE', key, ttl_seconds)
end
local remaining = redis.call('TTL', key)
return {count, remaining}
```

**Fixed vs. sliding window**: Fixed window (TTL from first failure) chosen because:
- Matches the spec's description: "Failed attempt counters have a TTL matching the lockout window, so they naturally expire"
- Simpler mental model: lockout expires N minutes after first failure, regardless of continued attempts
- An active attacker who keeps trying still triggers a new lockout once the window expires and they hit the threshold again

**Two-key variant** for checking both identifier and IP in a single round-trip:
```lua
local id_key = KEYS[1]
local ip_key = KEYS[2]
local id_ttl = tonumber(ARGV[1])
local ip_ttl = tonumber(ARGV[2])

local id_count = redis.call('INCR', id_key)
if id_count == 1 then
    redis.call('EXPIRE', id_key, id_ttl)
end
local id_remaining = redis.call('TTL', id_key)

local ip_count = redis.call('INCR', ip_key)
if ip_count == 1 then
    redis.call('EXPIRE', ip_key, ip_ttl)
end
local ip_remaining = redis.call('TTL', ip_key)

return {id_count, id_remaining, ip_count, ip_remaining}
```

**`redis.NewScript` behavior in go-redis v9**:
1. Computes SHA1 hash at init time
2. First call tries `EVALSHA` (hash only, no script transfer)
3. If Redis returns `NOSCRIPT`, automatically retries with `EVAL` (full script)
4. After first `EVAL`, Redis caches the script for subsequent `EVALSHA` calls

**Reset operation**: Simple `DEL key1 key2` (atomic per Redis spec for multi-key `DEL`).

**Redis Cluster caveat**: The two-key Lua script requires both keys on the same Redis node. Since `login_backoff:id:{email}` and `login_backoff:ip:{ip}` hash to different slots, this only works with standalone Redis. The current codebase uses `redis.NewClient` (standalone), so this is fine. If Redis Cluster is adopted later, split into two separate script calls.

**Alternatives considered**:
- `MULTI/EXEC`: Cannot use intermediate values for conditional TTL. Would reset TTL on every increment (sliding window), which is a different semantic.
- `INCR` + `EXPIRE` (two commands): Race condition on crash between commands. Not acceptable for a security feature where key leak = permanent lockout.

**Sources**:
- [Redis INCR - Pattern: Rate limiter 2](https://redis.io/docs/latest/commands/incr/)
- [go-redis Lua scripting guide](https://redis.uptrace.dev/guide/lua-scripting.html)
- [go-redis Script.go source](https://github.com/redis/go-redis/blob/master/script.go)

---

## R-004: Kratos Request Header Allowlist

**Decision**: Use `True-Client-Ip` header for client IP extraction in Kratos webhook payloads.

**Rationale**: Kratos filters request headers through a strict allowlist before passing them to webhook Jsonnet templates. The allowlist includes `True-Client-Ip` but NOT `X-Forwarded-For` or `X-Real-Ip`.

Allowed headers relevant to IP detection:
- `True-Client-Ip` (Cloudflare-style, on the allowlist)

NOT allowed:
- `X-Forwarded-For` (stripped by Kratos)
- `X-Real-Ip` (stripped by Kratos)

**Impact**: The infrastructure (ingress controller / reverse proxy) must be configured to set the `True-Client-Ip` header for the Kratos after-login webhook to include client IP in its payload.

For the before-login check (called by proxy, not Kratos), the proxy has direct access to client IP and can include it in the request body regardless of Kratos header filtering.

**Source**: [Kratos webhook source (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/selfservice/hook/web_hook.go) - `RequestHeaderAllowList`

---

## R-005: Endpoint Response Design

**Decision**: Return simple, caller-agnostic JSON responses from the webhook-gateway endpoints.

**Rationale**: Given R-001 (before-login may be called by a proxy, not Kratos), and R-002 (Kratos expects a specific messages format), the webhook-gateway should return a simple response format. The integration layer (proxy, Kratos hook config) adapts the response to whatever the consumer expects.

**Before-login response (allowed)**:
```json
HTTP 200
{"allowed": true, "identifier_attempts": 3, "ip_attempts": 5}
```

**Before-login response (blocked)**:
```json
HTTP 403
{
  "allowed": false,
  "reason": "identifier",
  "message": "Account temporarily locked due to too many failed attempts. Try again in 2 minutes.",
  "retry_after_seconds": 95
}
```

**After-login response**:
```json
HTTP 200
{"status": "success", "message": "counters reset"}
```

This format is easy to adapt: a Kratos Jsonnet template or a proxy middleware can transform the 403 response into whatever error format the frontend expects.
