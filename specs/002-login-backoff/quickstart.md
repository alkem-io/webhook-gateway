# Quickstart: Login Backoff / Brute Force Protection

**Feature**: 002-login-backoff

## Prerequisites

- Go 1.24+
- Redis running locally (default: `redis://localhost:6379/0`)
- The webhook-gateway builds and tests pass (`make build && make test`)

## Configuration

Add these environment variables to your `.env` or export them:

```bash
# Login Backoff Configuration (all optional, shown with defaults)
LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS=10
LOGIN_BACKOFF_MAX_IP_ATTEMPTS=20
LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS=120
LOGIN_BACKOFF_IP_LOCKOUT_SECONDS=120
```

## Running Locally

```bash
make run
```

## Testing the Endpoints

### Before-Login Check (allowed)

```bash
curl -s -X POST http://localhost:8080/api/v1/webhooks/kratos/login-backoff/before-login \
  -H "Content-Type: application/json" \
  -d '{"identifier": "test@example.com", "client_ip": "127.0.0.1"}' | jq .
```

Expected response (first attempt):
```json
{"allowed": true, "identifier_attempts": 1, "ip_attempts": 1}
```

### Trigger Lockout

Run the before-login request 11 times (default threshold is 10). The 11th request returns:

```json
{"allowed": false, "reason": "identifier", "message": "Account temporarily locked due to too many failed attempts. Try again in 2 minutes.", "retry_after_seconds": 118}
```

### Reset Counters (successful login)

```bash
curl -s -X POST http://localhost:8080/api/v1/webhooks/kratos/login-backoff/after-login \
  -H "Content-Type: application/json" \
  -d '{"identity_id": "some-uuid", "email": "test@example.com", "client_ip": "127.0.0.1"}' | jq .
```

Expected response:
```json
{"status": "success", "message": "counters reset"}
```

### Verify Redis State

```bash
# Check current counter value
redis-cli GET "login_backoff:id:test@example.com"

# Check remaining TTL
redis-cli TTL "login_backoff:id:test@example.com"

# Manually clear for testing
redis-cli DEL "login_backoff:id:test@example.com" "login_backoff:ip:127.0.0.1"
```

## Integration Notes

### Kratos After-Login Hook

The after-login endpoint is called by a Kratos webhook. Add this to the Kratos config under `selfservice.flows.login.after.password.hooks`:

```yaml
- hook: web_hook
  config:
    url: http://alkemio-webhook-gateway-service:8080/api/v1/webhooks/kratos/login-backoff/after-login
    method: POST
    body: file:///etc/config/login-backoff-after.jsonnet
    response:
      ignore: true
```

With Jsonnet template:
```jsonnet
function(ctx) {
  identity_id: ctx.identity.id,
  email: ctx.identity.traits.email,
  client_ip: if "True-Client-Ip" in ctx.request_headers then ctx.request_headers["True-Client-Ip"][0] else "",
}
```

### Before-Login Integration

The before-login endpoint must be called at **credential submission time** (not at flow creation). See `research.md` R-001 for details on why Kratos `before` hooks cannot be used for this purpose. The calling mechanism (reverse proxy, Alkemio server, etc.) is an infrastructure concern.
