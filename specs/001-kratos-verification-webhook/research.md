# Research: Ory Kratos Post-Verification Webhook

**Feature**: 001-kratos-verification-webhook
**Date**: 2026-01-26

## Kratos Webhook Payload Structure

### Decision: Use Kratos Jsonnet-Templated Webhook Payload

**Rationale**: Ory Kratos webhooks use Jsonnet templates to customize the payload sent to webhook endpoints. The `ctx` object contains identity information after successful verification.

**Alternatives considered**:
- Raw Kratos internal payload format: Rejected because Kratos documentation recommends using Jsonnet templates for clean, customized payloads
- Session-based payload: Not applicable for verification flow (no session exists yet)

### Kratos Context Object (`ctx`)

The Jsonnet template receives a `ctx` object containing:

```jsonnet
{
  flow: {
    id: "uuid",
    type: "verification",
    expires_at: "ISO timestamp",
    issued_at: "ISO timestamp",
    request_url: "string",
    ui: { ... }
  },
  identity: {
    id: "uuid",
    created_at: "ISO timestamp",
    updated_at: "ISO timestamp",
    state: "active",
    schema_id: "string",
    traits: {
      email: "user@example.com",
      display_name: "John Doe",
      first_name: "John"
      // Additional custom traits
    },
    verifiable_addresses: [{
      id: "uuid",
      value: "user@example.com",
      via: "email",
      status: "completed",
      verified: true,
      created_at: "ISO timestamp",
      updated_at: "ISO timestamp"
    }],
    metadata_public: { ... },
    metadata_admin: { ... }
  },
  request_headers: { ... },
  request_method: "POST",
  request_url: "string"
}
```

### Recommended Kratos Configuration

```yaml
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

The base64-encoded Jsonnet template decodes to:

```jsonnet
function(ctx) {
  identity_id: ctx.identity.id,
  email: ctx.identity.traits.email,
  display_name: ctx.identity.traits.display_name,
  first_name: ctx.identity.traits.first_name
}
```

**Note**: `response.ignore: true` ensures non-blocking behavior - Kratos won't wait for webhook response.

---

## UserSignupWelcome Notification Event Schema

### Decision: Match Existing Alkemio Notification Event Format

**Rationale**: The notifications service already has a working implementation for `UserSignupWelcome` events. The webhook gateway must publish events in the exact format expected by the notifications service.

**Alternatives considered**:
- Custom event format: Rejected to maintain compatibility with existing notifications service
- Direct email sending: Rejected because the spec requires using existing RabbitMQ infrastructure

### Event Type

```typescript
enum NotificationEvent {
  UserSignUpWelcome = 'USER_SIGN_UP_WELCOME'
}
```

### Event Payload Schema

The notifications service expects `NotificationEventPayloadPlatformUserRegistration`:

```typescript
interface NotificationEventPayloadPlatformUserRegistration extends BaseEventPayload {
  user: UserPayload;
}

interface BaseEventPayload {
  eventType: string;           // 'USER_SIGN_UP_WELCOME'
  triggeredBy: UserPayload;    // The user who triggered (same as recipient for welcome)
  recipients: UserPayload[];   // Array containing the new user
  platform: {
    url: string;               // Platform base URL (e.g., 'https://alkem.io')
  };
}

interface UserPayload extends ContributorPayload {
  firstName: string;
  lastName: string;
  email: string;
}

interface ContributorPayload {
  id: string;
  profile: {
    displayName: string;
    url: string;               // Profile URL
  };
  type: string;                // 'user'
}
```

### Example Event Payload

```json
{
  "eventType": "USER_SIGN_UP_WELCOME",
  "triggeredBy": {
    "id": "kratos-identity-uuid",
    "firstName": "John",
    "lastName": "",
    "email": "john@example.com",
    "profile": {
      "displayName": "John Doe",
      "url": "https://alkem.io/user/kratos-identity-uuid"
    },
    "type": "user"
  },
  "recipients": [{
    "id": "kratos-identity-uuid",
    "firstName": "John",
    "lastName": "",
    "email": "john@example.com",
    "profile": {
      "displayName": "John Doe",
      "url": "https://alkem.io/user/kratos-identity-uuid"
    },
    "type": "user"
  }],
  "platform": {
    "url": "https://alkem.io"
  }
}
```

---

## RabbitMQ Integration

### Decision: Publish to `alkemio-notifications` Queue

**Rationale**: The notifications service consumes from a durable queue named `alkemio-notifications`. The webhook gateway should publish directly to this queue.

**Alternatives considered**:
- Using exchange with routing keys: Possible but unnecessary for single event type
- Direct HTTP call to notifications service: Rejected per spec (requires RabbitMQ)

### Connection Configuration

```go
// Environment variables
RABBITMQ_URL=amqp://user:password@rabbitmq:5672/?heartbeat=30

// Queue settings (from notifications service)
queue: "alkemio-notifications"
queueOptions: { durable: true }
```

### Message Format

```go
// Message published to RabbitMQ
type RabbitMQMessage struct {
  ContentType string // "application/json"
  Body        []byte // JSON-encoded NotificationEventPayload
}
```

### Fail-Open Semantics

Per spec requirement FR-006: Return HTTP 200 to Kratos regardless of RabbitMQ availability.

```go
func (s *Service) PublishNotification(ctx context.Context, event NotificationEvent) error {
  err := s.rabbitClient.Publish(ctx, event)
  if err != nil {
    s.logger.Warn("failed to publish notification event",
      zap.Error(err),
      zap.String("identity_id", event.User.ID),
    )
    // Do not return error - fail-open
  }
  return nil
}
```

---

## Redis Idempotency Tracking

### Decision: Use Redis SET with NX for Once-Per-Identity Tracking

**Rationale**: Redis provides fast, atomic operations for checking and setting keys. The SET NX (set if not exists) operation ensures exactly-once semantics.

**Alternatives considered**:
- In-memory map: Rejected because it doesn't survive restarts and doesn't work with multiple instances
- Database table: Overhead for simple key-value tracking

### Key Schema

```
Key:   welcome_sent:{identity_id}
Value: {timestamp_iso}
TTL:   7776000 (90 days in seconds)
```

### Operations

```go
// Check and set in single atomic operation
func (c *RedisClient) MarkWelcomeSentIfNew(ctx context.Context, identityID string) (bool, error) {
  // Returns true if key was set (first time), false if already exists
  set, err := c.client.SetNX(ctx,
    fmt.Sprintf("welcome_sent:%s", identityID),
    time.Now().UTC().Format(time.RFC3339),
    90*24*time.Hour,
  ).Result()
  return set, err
}
```

### Fail-Open on Redis Unavailability

Per spec, if Redis is unavailable, proceed with notification (may result in duplicate, but verification shouldn't fail):

```go
func (s *Service) ShouldSendWelcome(ctx context.Context, identityID string) bool {
  isNew, err := s.redisClient.MarkWelcomeSentIfNew(ctx, identityID)
  if err != nil {
    s.logger.Warn("redis unavailable for idempotency check, proceeding with notification",
      zap.Error(err),
      zap.String("identity_id", identityID),
    )
    return true // Fail-open: allow potential duplicate rather than blocking
  }
  return isNew
}
```

---

## Profile URL Construction

### Decision: Construct from Platform URL and Identity ID

**Rationale**: Per spec assumption, profile URL can be constructed from the identity ID using a known pattern.

**Alternatives considered**:
- Fetch from Alkemio API: Adds latency and dependency; unnecessary given known URL pattern

### URL Pattern

```go
// Profile URL format
platformURL := "https://alkem.io"  // From config
profileURL := fmt.Sprintf("%s/user/%s", platformURL, identityID)

// Example: https://alkem.io/user/550e8400-e29b-41d4-a716-446655440000
```

---

## Go Dependencies (Latest Versions)

### Decision: Use Latest Stable Versions

**Rationale**: Per Constitution #11, always verify latest stable versions.

| Dependency | Version | Source |
|------------|---------|--------|
| go.uber.org/zap | v1.27.0 | [pkg.go.dev](https://pkg.go.dev/go.uber.org/zap) |
| github.com/redis/go-redis/v9 | v9.7.0 | [pkg.go.dev](https://pkg.go.dev/github.com/redis/go-redis/v9) |
| github.com/rabbitmq/amqp091-go | v1.10.0 | [pkg.go.dev](https://pkg.go.dev/github.com/rabbitmq/amqp091-go) |
| github.com/joho/godotenv | v1.5.1 | [pkg.go.dev](https://pkg.go.dev/github.com/joho/godotenv) |
| github.com/google/uuid | v1.6.0 | [pkg.go.dev](https://pkg.go.dev/github.com/google/uuid) |

---

## Error Handling Strategy

### Decision: Fail-Open with Structured Logging

**Rationale**: Per spec, webhook processing should not block verification flow. All errors should be logged but not returned to Kratos.

### Error Categories

| Error Type | Response to Kratos | Action |
|------------|-------------------|--------|
| Invalid JSON payload | HTTP 200 | Log warning, skip notification |
| Missing required traits | HTTP 200 | Log warning, skip notification |
| Redis unavailable | HTTP 200 | Log warning, proceed (may duplicate) |
| RabbitMQ unavailable | HTTP 200 | Log error, notification lost |
| Maintenance mode | HTTP 503 | Log info, Kratos will retry |

---

## Sources

- [Ory Webhooks Documentation](https://www.ory.com/docs/guides/integrate-with-ory-cloud-through-webhooks)
- [Ory Kratos Hooks Configuration](https://www.ory.com/docs/kratos/hooks/configure-hooks)
- [Alkemio Notifications Service](https://github.com/alkem-io/notifications)
- [@alkemio/notifications-lib](https://www.npmjs.com/package/@alkemio/notifications-lib)
