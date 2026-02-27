# ADR-001: Reverse Proxy for Login Credential Submission Interception

**Feature**: 002-login-backoff
**Date**: 2026-02-26
**Status**: Accepted

## Context

The login backoff feature requires counting every failed login attempt and blocking requests once a threshold is exceeded. This means intercepting Kratos login POST submissions (where the user submits email + password) — not just page loads or successful logins.

Ory Kratos v1.3.1 provides two hook timing points for login flows:

| Hook | When it fires | Has identifier? | Fires on failure? |
|---|---|---|---|
| `before` | Flow creation (GET /self-service/login/browser) | No — user hasn't typed yet | N/A |
| `after` (password) | Successful authentication only | Yes (identity context) | **No** |

Neither hook fires at credential submission time with the submitted identifier. Kratos GitHub issue [#3580](https://github.com/ory/kratos/issues/3580) explicitly identifies this gap (requesting an "after login attempt" action).

The core problem: **there is no native Kratos hook for failed login attempts**, and the before-hook fires too early (at page load) without access to the identifier.

See [research.md R-001](../research.md) for the detailed investigation.

## Decision

Build a reverse proxy into the webhook-gateway (`proxy.go`) using Go's `httputil.ReverseProxy`. Traefik routes all login traffic (`/ory/kratos/public/self-service/login*`) to the webhook-gateway at priority 200. The proxy:

1. Intercepts POST requests (credential submissions)
2. Extracts the identifier (email) from the request body and the client IP from headers
3. Checks Redis counters against configurable thresholds
4. If allowed: proxies the request to Kratos via `KratosInternalURL`
5. If blocked: returns 303 redirect (browser) or 429 JSON (API)
6. Passes through all non-POST requests (GET flow creation, etc.) unchanged

The after-login Kratos webhook (native, fires on success) resets counters when authentication succeeds.

## Alternatives Considered

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| **A. Kratos before-hook** | Native Kratos integration, no proxy needed | Fires at flow creation (page load), not credential submission. No access to identifier. Only IP-based rate limiting possible. | Cannot count per-identifier failed attempts — the fundamental requirement |
| **B. Kratos after-hook only** | Works natively for counter reset | Only fires on successful auth. Cannot count or block failed attempts at all. | No mechanism for counting failures |
| **C. Alkemio server-side call** | No infrastructure routing changes | Requires modifying the Alkemio server (separate repo/team). Couples webhook-gateway to server release cycle. | Breaks service boundary; webhook-gateway should be self-contained |
| **D. Traefik custom middleware** | Would intercept at the right point | Requires writing/deploying a custom Traefik plugin. More complex than a Go HTTP handler. | Over-engineered for the use case |
| **E. Wait for Kratos #3580** | Clean native integration | Unknown timeline. Feature blocked indefinitely. | Unacceptable delivery risk |

## Consequences

**Positive:**
- Full access to both identifier and client IP at credential submission time
- Self-contained within webhook-gateway — no changes to Kratos, Alkemio server, or other services
- Transparent to Kratos — allowed requests are proxied unchanged
- Supports both browser (form POST) and API (JSON POST) clients
- Counter reset on success works via native Kratos after-login webhook
- If Kratos adds issue #3580, the proxy can be replaced with a native hook without changing the webhook-gateway's core logic (service.go, Redis operations remain the same)

**Negative:**
- Adds a network hop (Traefik → webhook-gateway → Kratos instead of Traefik → Kratos)
- Traefik routing configuration required (priority-based router for login paths)
- The webhook-gateway must know Kratos's internal URL (`KratosInternalURL` config)
- IP counter reset on successful login depends on `True-Client-Ip` header availability in Kratos webhook context (see known limitation below)

**Known limitation:** The Kratos after-login webhook's `ctx.request_headers` only includes headers in Kratos's default allowlist. `True-Client-Ip` is allowed but `X-Forwarded-For` is not. In environments without Cloudflare (which sets `True-Client-Ip`), the IP counter is not reset on successful login — only the identifier counter is reset. The IP counter still expires naturally via TTL.

## Empirical Validation

Validated end-to-end on 2026-02-26 against a running Kratos v1.3.1 instance:

1. **Failed attempts are counted by the proxy:**
   - 5 wrong-password login attempts via proxy → Redis `login_backoff:id:admin@alkem.io = 5`, `login_backoff:ip:172.19.0.1 = 5`

2. **Blocking works at threshold:**
   - 11th attempt → proxy returns 429/303 (blocked), Kratos never receives the request
   - Confirmed via webhook-gateway logs: `"login attempt blocked"`, `identifier_attempts: 11`

3. **After-login webhook fires on success and resets identifier counter:**
   - Successful login with correct password → Kratos calls `POST /api/v1/webhooks/kratos/login-backoff/after-login`
   - Redis `login_backoff:id:admin@alkem.io` key deleted (counter reset)
   - Confirmed via webhook-gateway logs: `"login backoff counters reset"`

4. **Post-reset, new failures start from zero:**
   - 5 more wrong-password attempts after reset → Redis `login_backoff:id:admin@alkem.io = 5` (fresh counter)

5. **Non-POST requests pass through unchanged:**
   - `GET /self-service/login/browser`, `GET /self-service/login/flows` → proxied to Kratos with 200 responses

6. **Kratos before-hook timing confirmed:**
   - Kratos `before` hooks are not configured for login and would fire at flow creation (GET), not credential submission (POST). Confirmed by: (a) Kratos source code analysis of `selfservice/flow/login/handler.go`, (b) absence of any identifier in the `before` hook Jsonnet context, (c) Kratos issue #3580 acknowledging this gap.

## References

- [Research R-001: Kratos Before-Login Hook Timing](../research.md) — detailed analysis of hook timing
- [Research R-004: Kratos Request Header Allowlist](../research.md) — True-Client-Ip vs X-Forwarded-For
- [Kratos login handler source (v1.3.1)](https://github.com/ory/kratos/blob/v1.3.1/selfservice/flow/login/handler.go) — `NewLoginFlow` calls `PreLoginHook`
- [Kratos GitHub Issue #3580](https://github.com/ory/kratos/issues/3580) — "New Ory Action: after login attempt"
- [proxy.go](../../../internal/webhooks/kratos-login-backoff/proxy.go) — implementation
- [Traefik routing config](../../../server/.build/traefik/http.yml) — `kratos-login-backoff` router
