# Configuration

The webhook gateway is configured via environment variables. See `.env.example` for a complete list.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `LOG_FORMAT` | Log format (json, console) | `json` |
| `REDIS_URL` | Redis connection URL | `redis://localhost:6379/0` |
| `RABBITMQ_URL` | RabbitMQ connection URL | `amqp://guest:guest@localhost:5672/` |
| `PLATFORM_URL` | Alkemio platform base URL | `https://alkem.io` |
| `MAINTENANCE_MODE` | Enable maintenance mode (true/false) | `false` |
| `MAINTENANCE_MESSAGE` | Message shown during maintenance | `Service under maintenance` |
| `CORRELATION_ID_HEADER` | HTTP header for correlation ID | `X-Request-ID` |

## Local Development

Copy `.env.example` to `.env` and adjust values for your local environment:

```bash
cp configs/.env.example .env
```
