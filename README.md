# Alkemio Kratos Webhooks

A Go HTTP service that receives webhooks from external systems and bridges them into the Alkemio platform's notification infrastructure. Currently handles Ory Kratos post-verification webhooks, publishing welcome notification events to RabbitMQ for downstream consumption.

## Architecture

```
Ory Kratos ──POST──▶ Kratos Webhooks ──publish──▶ RabbitMQ (alkemio-notifications)
                           │
                           ├── Redis (idempotency tracking)
                           └── Structured logging (Zap)
```

The gateway follows **fail-open semantics**: all webhook responses return HTTP 200 to avoid blocking the Kratos verification flow, regardless of downstream failures.

## Project Structure

```
cmd/
  server/                          # Application entrypoint
configs/
  .env.example                     # Example environment variables
contracts/
  openapi.yaml                     # OpenAPI 3.0 specification
internal/
  clients/                         # Redis and RabbitMQ client wrappers
  config/                          # Configuration loading and logger setup
  health/                          # Kubernetes liveness and readiness probes
  middleware/                       # Correlation ID, logging, maintenance mode
  webhooks/
    kratos-verification/           # Kratos post-verification webhook handler
```

## Prerequisites

- Go 1.24+
- Redis
- RabbitMQ
- Docker (optional)

## Getting Started

1. Clone the repository and install dependencies:

```bash
git clone https://github.com/alkem-io/kratos-webhooks.git
cd kratos-webhooks
go mod download
```

2. Copy the example config and adjust values:

```bash
cp configs/.env.example .env
```

3. Run the server:

```bash
make run
```

## Configuration

All configuration is driven by environment variables. See `configs/.env.example` for defaults.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | `json` | Log format (`json` or `console`) |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection string |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection string |
| `PLATFORM_URL` | `https://alkem.io` | Base Alkemio platform URL |
| `MAINTENANCE_MODE` | `false` | Enable maintenance mode (returns 503) |
| `MAINTENANCE_MESSAGE` | `Service under maintenance` | Maintenance response message |
| `CORRELATION_ID_HEADER` | `X-Request-ID` | HTTP header used for request tracing |

## API Endpoints

### Webhooks

**POST** `/api/v1/webhooks/kratos/verification`

Receives Kratos post-verification payloads. Always returns HTTP 200.

Request body:

```json
{
  "identity_id": "uuid",
  "email": "user@example.com",
  "display_name": "Jane Doe",
  "first_name": "Jane"
}
```

Response:

```json
{
  "status": "success|skipped|error",
  "message": "optional explanation"
}
```

### Health

| Endpoint | Method | Purpose |
|---|---|---|
| `/health/live` | GET | Kubernetes liveness probe (always 200) |
| `/health/ready` | GET | Kubernetes readiness probe (checks Redis and RabbitMQ) |

## Make Targets

```bash
make build          # Compile binary to ./bin/kratos-webhooks
make test           # Run tests with race detector
make lint           # Run golangci-lint
make run            # Run the server locally
make clean          # Remove build artifacts
make tidy           # Run go mod tidy
make docker-build   # Build Docker image
make docker-run     # Run Docker container
```

## Docker

The project uses a multi-stage build with a distroless runtime image for minimal attack surface:

```bash
make docker-build
make docker-run
```

The container runs as a non-root user and exposes port 8080.

## Processing Flow

1. Receive and decode the Kratos webhook payload
2. Validate required fields (skip with warning if invalid)
3. Check Redis for duplicate delivery (skip if welcome already sent)
4. Transform payload into a `UserSignupWelcomeEvent`
5. Publish event to the `alkemio-notifications` RabbitMQ queue
6. Mark identity as welcomed in Redis (90-day TTL)
7. Return HTTP 200

## Middleware Stack

Applied in order:

1. **Correlation ID** - Extracts or generates a request trace ID
2. **Maintenance** - Returns 503 when maintenance mode is enabled (health endpoints are exempt)
3. **Logging** - Records request method, path, status code, and duration

## License

See [LICENSE](LICENSE) for details.
