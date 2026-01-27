# Data Model: Ory Kratos Post-Verification Webhook

**Feature**: 001-kratos-verification-webhook
**Date**: 2026-01-26

## Entities

### 1. KratosVerificationPayload (Input)

The incoming webhook payload from Ory Kratos after successful email verification.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| identity_id | string (UUID) | Yes | Kratos identity UUID |
| email | string | Yes | Verified email address |
| display_name | string | Yes | User's display name from traits |
| first_name | string | Yes | User's first name from traits |

**Go Type Definition**:

```go
// KratosVerificationPayload represents the webhook payload from Kratos
// after successful email verification.
type KratosVerificationPayload struct {
    IdentityID  string `json:"identity_id"`
    Email       string `json:"email"`
    DisplayName string `json:"display_name"`
    FirstName   string `json:"first_name"`
}
```

**Validation Rules**:
- `identity_id`: Must be non-empty, valid UUID format
- `email`: Must be non-empty, valid email format
- `display_name`: Must be non-empty
- `first_name`: Must be non-empty

**Source**: Constructed by Kratos via Jsonnet template from `ctx.identity` object.

---

### 2. UserPayload (Internal)

User information structure matching Alkemio notifications-lib format.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | string (UUID) | Yes | User/identity ID |
| firstName | string | Yes | User's first name |
| lastName | string | Yes | User's last name (empty string if not available) |
| email | string | Yes | User's email address |
| profile.displayName | string | Yes | Display name |
| profile.url | string | Yes | Full profile URL |
| type | string | Yes | Contributor type ("user") |

**Go Type Definition**:

```go
// UserPayload represents user information in notification events.
type UserPayload struct {
    ID        string      `json:"id"`
    FirstName string      `json:"firstName"`
    LastName  string      `json:"lastName"`
    Email     string      `json:"email"`
    Profile   ProfileInfo `json:"profile"`
    Type      string      `json:"type"`
}

// ProfileInfo contains profile display information.
type ProfileInfo struct {
    DisplayName string `json:"displayName"`
    URL         string `json:"url"`
}
```

---

### 3. UserSignupWelcomeEvent (Output)

The notification event published to RabbitMQ for the notifications service.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| eventType | string | Yes | Fixed value: "USER_SIGN_UP_WELCOME" |
| triggeredBy | UserPayload | Yes | The user who triggered (same as recipient) |
| recipients | []UserPayload | Yes | Array containing the new user |
| platform.url | string | Yes | Platform base URL |

**Go Type Definition**:

```go
// UserSignupWelcomeEvent is the notification event for new user welcome emails.
type UserSignupWelcomeEvent struct {
    EventType   string        `json:"eventType"`
    TriggeredBy UserPayload   `json:"triggeredBy"`
    Recipients  []UserPayload `json:"recipients"`
    Platform    PlatformInfo  `json:"platform"`
}

// PlatformInfo contains platform configuration.
type PlatformInfo struct {
    URL string `json:"url"`
}
```

**Event Type Constant**:

```go
const EventTypeUserSignUpWelcome = "USER_SIGN_UP_WELCOME"
```

---

### 4. WebhookResponse (Output)

HTTP response returned to Kratos.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| status | string | Yes | Processing status: "success", "skipped", "error" |
| message | string | No | Optional message explaining the status |

**Go Type Definition**:

```go
// WebhookResponse is the HTTP response to Kratos.
type WebhookResponse struct {
    Status  string `json:"status"`
    Message string `json:"message,omitempty"`
}
```

**Status Values**:
- `success`: Notification event published successfully
- `skipped`: Notification skipped (duplicate, missing traits, etc.)
- `error`: Internal error (still returns HTTP 200)

---

### 5. WelcomeSentRecord (Redis)

Persistent tracking of which identities have received welcome emails.

| Field | Type | Description |
|-------|------|-------------|
| Key | string | Format: `welcome_sent:{identity_id}` |
| Value | string | ISO 8601 timestamp when welcome was sent |
| TTL | duration | 90 days |

**Operations**:
- `SetNX`: Set only if key doesn't exist (atomic idempotency check)
- `Exists`: Check if key exists (for logging/debugging)

---

## Entity Relationships

```
┌─────────────────────────────────┐
│   Ory Kratos                    │
│   (Post-Verification Webhook)   │
└──────────────┬──────────────────┘
               │
               ▼
┌─────────────────────────────────┐
│   KratosVerificationPayload     │
│   - identity_id                 │
│   - email                       │
│   - display_name                │
│   - first_name                  │
└──────────────┬──────────────────┘
               │
               │ Transform
               ▼
┌─────────────────────────────────┐
│   UserPayload                   │
│   - id (from identity_id)       │
│   - email                       │
│   - firstName                   │
│   - profile.displayName         │
│   - profile.url (constructed)   │
└──────────────┬──────────────────┘
               │
               │ Compose
               ▼
┌─────────────────────────────────┐
│   UserSignupWelcomeEvent        │
│   - eventType: USER_SIGN_UP_... │
│   - triggeredBy: UserPayload    │
│   - recipients: [UserPayload]   │
│   - platform.url                │
└──────────────┬──────────────────┘
               │
               │ Publish
               ▼
┌─────────────────────────────────┐
│   RabbitMQ                      │
│   Queue: alkemio-notifications  │
└─────────────────────────────────┘

┌─────────────────────────────────┐
│   Redis                         │
│   welcome_sent:{identity_id}    │
│   (Idempotency tracking)        │
└─────────────────────────────────┘
```

---

## Transformation Logic

### KratosVerificationPayload → UserSignupWelcomeEvent

```go
func TransformToNotificationEvent(payload KratosVerificationPayload, platformURL string) UserSignupWelcomeEvent {
    user := UserPayload{
        ID:        payload.IdentityID,
        FirstName: payload.FirstName,
        LastName:  "", // Not provided by Kratos traits
        Email:     payload.Email,
        Profile: ProfileInfo{
            DisplayName: payload.DisplayName,
            URL:         fmt.Sprintf("%s/user/%s", platformURL, payload.IdentityID),
        },
        Type: "user",
    }

    return UserSignupWelcomeEvent{
        EventType:   EventTypeUserSignUpWelcome,
        TriggeredBy: user,
        Recipients:  []UserPayload{user},
        Platform: PlatformInfo{
            URL: platformURL,
        },
    }
}
```
