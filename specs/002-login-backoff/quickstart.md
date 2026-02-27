# Quickstart: Login Backoff / Brute Force Protection

**Feature**: 002-login-backoff

## Prerequisites

- Go 1.24+
- Redis running locally (default: `redis://localhost:6379/0`)
- The kratos-webhooks builds and tests pass (`make build && make test`)

## Configuration

Add these environment variables to your `.env` or export them:

```bash
# Login Backoff Configuration (all optional, shown with defaults)
LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS=10
LOGIN_BACKOFF_MAX_IP_ATTEMPTS=20
LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS=120
LOGIN_BACKOFF_IP_LOCKOUT_SECONDS=120

# Kratos internal URL for the reverse proxy (required for proxy mode)
KRATOS_INTERNAL_URL=http://kratos:4433
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
    url: http://alkemio-kratos-webhooks-service:8080/api/v1/webhooks/kratos/login-backoff/after-login
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

### Before-Login Integration (Reverse Proxy)

The before-login check is handled by a built-in reverse proxy (`proxy.go`). Traefik routes login traffic to the kratos-webhooks, which checks backoff on POST requests and proxies allowed requests to Kratos.

**Traefik routing** (in `server/.build/traefik/http.yml`):

```yaml
# Service
kratos-webhooks:
  loadBalancer:
    servers:
      - url: 'http://kratos-webhooks:8080/'

# Router (priority 200 — higher than the default kratos-public router)
kratos-login-backoff:
  rule: 'PathPrefix(`/ory/kratos/public/self-service/login`)'
  service: 'kratos-webhooks'
  middlewares:
    - strip-kratos-public-prefix
  entryPoints:
    - 'web'
  priority: 200
```

**Proxy routes** registered in `main.go`:
- `/self-service/login` — exact path (credential submission POST)
- `/self-service/login/` — sub-paths (flow creation GET, e.g. `/self-service/login/browser`)

**Behavior**:
- GET requests → proxied to Kratos without interception
- POST requests → identifier and IP extracted from body/headers, backoff checked, blocked with 429 (API) or 303 redirect (browser) if over threshold

### Client-Web Lockout Display

When the proxy blocks a browser request, it redirects to `/login?lockout=true&retry_after=N`. The `LoginPage.tsx` reads these query params and injects a lockout message into the Kratos UI messages array (id: `9000429`, type: `error`), which is rendered by the existing `KratosMessages` component.

Translation key: `authentication.lockout` in `translation.en.json`.

### Docker Compose Deployment

The kratos-webhooks is added to `quickstart-services.yml`:

```yaml
kratos-webhooks:
  container_name: alkemio_dev_kratos_webhooks
  hostname: kratos-webhooks
  image: alkemio/kratos-webhooks:latest
  depends_on:
    redis: { condition: service_started }
    rabbitmq: { condition: service_healthy }
  environment:
    - REDIS_URL=redis://redis:6379/0
    - KRATOS_INTERNAL_URL=http://kratos:4433
    - LOGIN_BACKOFF_MAX_IDENTIFIER_ATTEMPTS=10
    - LOGIN_BACKOFF_MAX_IP_ATTEMPTS=20
    - LOGIN_BACKOFF_IDENTIFIER_LOCKOUT_SECONDS=120
    - LOGIN_BACKOFF_IP_LOCKOUT_SECONDS=120
  networks:
    - alkemio_dev_net
```

Build and deploy:
```bash
make docker-build
cd ../server && docker compose --env-file .env.docker -f quickstart-services.yml -p alkemio-serverdev up -d kratos-webhooks
```
