# Quickstart: Ory Kratos Post-Verification Webhook

**Feature**: 001-kratos-verification-webhook
**Date**: 2026-01-26

## Prerequisites

- Go 1.24+
- Docker (for local Redis and RabbitMQ)
- Access to Ory Kratos configuration

## Local Development Setup

### 1. Start Dependencies

```bash
# Start Redis and RabbitMQ using Docker
docker run -d --name redis -p 6379:6379 redis:7-alpine
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```

### 2. Configure Environment

Create `.env` file in project root:

```env
# Server
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=console

# Redis
REDIS_URL=redis://localhost:6379/0

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/

# Platform
PLATFORM_URL=https://alkem.io

# Maintenance
MAINTENANCE_MODE=false
MAINTENANCE_MESSAGE=Service under maintenance

# Correlation ID
CORRELATION_ID_HEADER=X-Request-ID
```

### 3. Run the Service

```bash
# Install dependencies
go mod download

# Run the server
go run ./cmd/server

# Or with hot reload (if using air)
air
```

### 4. Verify Health

```bash
# Liveness probe
curl http://localhost:8080/health/live

# Readiness probe
curl http://localhost:8080/health/ready
```

## Testing the Webhook

### Manual Test

```bash
curl -X POST http://localhost:8080/api/v1/webhooks/kratos/verification \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: test-correlation-123" \
  -d '{
    "identity_id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john@example.com",
    "display_name": "John Doe",
    "first_name": "John"
  }'
```

Expected response:

```json
{
  "status": "success"
}
```

### Verify RabbitMQ Message

1. Open RabbitMQ Management UI: http://localhost:15672 (guest/guest)
2. Navigate to Queues → `alkemio-notifications`
3. Click "Get messages" to view the published event

### Verify Redis Idempotency

```bash
# Check if key was set
redis-cli GET "welcome_sent:550e8400-e29b-41d4-a716-446655440000"
```

## Kratos Configuration

Configure Ory Kratos to send post-verification webhooks:

```yaml
# kratos.yml
selfservice:
  flows:
    verification:
      enabled: true
      use: code
      after:
        hooks:
          - hook: web_hook
            config:
              url: http://webhook-gateway:8080/api/v1/webhooks/kratos/verification
              method: POST
              body: base64://ZnVuY3Rpb24oY3R4KSB7CiAgaWRlbnRpdHlfaWQ6IGN0eC5pZGVudGl0eS5pZCwKICBlbWFpbDogY3R4LmlkZW50aXR5LnRyYWl0cy5lbWFpbCwKICBkaXNwbGF5X25hbWU6IGN0eC5pZGVudGl0eS50cmFpdHMuZGlzcGxheV9uYW1lLAogIGZpcnN0X25hbWU6IGN0eC5pZGVudGl0eS50cmFpdHMuZmlyc3RfbmFtZQp9
              response:
                ignore: true
```

The base64 Jsonnet template decodes to:

```jsonnet
function(ctx) {
  identity_id: ctx.identity.id,
  email: ctx.identity.traits.email,
  display_name: ctx.identity.traits.display_name,
  first_name: ctx.identity.traits.first_name
}
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run contract tests only
go test ./test/contract/...

# Run integration tests only
go test ./test/integration/...

# Run with verbose output
go test -v ./...
```

## Docker Build

```bash
# Build the image
docker build -t webhook-gateway:latest .

# Run the container
docker run -p 8080:8080 \
  -e REDIS_URL=redis://host.docker.internal:6379/0 \
  -e RABBITMQ_URL=amqp://guest:guest@host.docker.internal:5672/ \
  -e PLATFORM_URL=https://alkem.io \
  webhook-gateway:latest
```

## Observability

### Log Output

Structured JSON logs with correlation IDs:

```json
{
  "level": "info",
  "ts": "2026-01-26T10:30:45.123Z",
  "caller": "handler.go:42",
  "msg": "webhook received",
  "correlation_id": "test-correlation-123",
  "identity_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "success"
}
```

### Key Log Fields

| Field | Description |
|-------|-------------|
| correlation_id | Request tracking ID (from X-Request-ID header or generated) |
| identity_id | Kratos identity UUID |
| status | Processing result: success, skipped, error |
| error | Error details if processing failed |

## Troubleshooting

### Common Issues

**Redis connection refused**:
```bash
# Check Redis is running
docker ps | grep redis
# Verify connection
redis-cli ping
```

**RabbitMQ connection failed**:
```bash
# Check RabbitMQ is running
docker ps | grep rabbitmq
# Verify management UI is accessible
curl http://localhost:15672/api/overview
```

**Webhook returns 503**:
```bash
# Check maintenance mode is disabled
echo $MAINTENANCE_MODE
# Should be empty or "false"
```
