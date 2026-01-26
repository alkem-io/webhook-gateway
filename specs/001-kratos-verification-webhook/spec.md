# Feature Specification: Ory Kratos Post-Verification Webhook

**Feature Branch**: `001-kratos-verification-webhook`
**Created**: 2026-01-26
**Status**: Draft
**Input**: User description: "add an ory kratos post-verification webhook that invokes the NotificationEvent.UserSignupWelcome. That event WILL NOT be deleted but we'll add a webhook that will be triggered in Ory Kratos after successful verification of user email triggering this email."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - New User Receives Welcome Email After Verification (Priority: P1)

A new user registers on the Alkemio platform and verifies their email address through the Ory Kratos verification flow. Upon successful email verification, they automatically receive a welcome email helping them get started with the platform.

**Why this priority**: This is the core value proposition - ensuring every verified user receives onboarding communication to improve their initial experience with the platform.

**Independent Test**: Can be fully tested by completing the Kratos email verification flow and observing that the welcome email is delivered to the user's inbox.

**Acceptance Scenarios**:

1. **Given** a user has just verified their email in Ory Kratos, **When** the verification succeeds, **Then** the webhook gateway receives the post-verification webhook and triggers the UserSignupWelcome notification event.

2. **Given** the webhook gateway receives a valid post-verification payload from Kratos, **When** processing completes, **Then** the notifications service receives the event containing user display name, first name, email, and profile URL.

3. **Given** the notifications service receives a UserSignupWelcome event, **When** the event is processed, **Then** the user receives a welcome email at their verified email address.

---

### User Story 2 - System Handles Verification Failures Gracefully (Priority: P2)

When the webhook gateway cannot successfully forward the notification event (e.g., notifications service unavailable), the system handles this gracefully without impacting the user's verification status.

**Why this priority**: Users should not have their verification blocked due to notification delivery issues - verification is critical path, welcome email is not.

**Independent Test**: Can be tested by simulating notifications service unavailability during webhook processing and verifying the webhook returns successfully to Kratos.

**Acceptance Scenarios**:

1. **Given** the notifications service is unavailable, **When** the webhook gateway attempts to forward the event, **Then** the gateway logs the failure and returns a success response to Kratos (verification should not be blocked).

2. **Given** the webhook payload is malformed or missing required fields, **When** the gateway receives the request, **Then** it logs the validation error with details and returns an appropriate response.

---

### Edge Cases

- **Re-verification scenario**: If Kratos sends a verification webhook for an already-verified user, the system checks if a welcome email was previously sent for that identity and skips notification if so.
- **Duplicate webhook deliveries**: Kratos retries are handled via once-per-identity tracking; duplicate webhooks for the same user do not trigger additional emails.
- **Incomplete profile data**: If required user traits (display name, first name, email) are missing from the Kratos payload, the system skips notification emission silently and logs a warning for operational visibility.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose an HTTP endpoint to receive Ory Kratos post-verification webhooks.
- **FR-002**: System MUST validate incoming webhook payloads according to the Kratos webhook schema.
- **FR-003**: System MUST extract user information (identity ID, email, traits) from the Kratos verification webhook payload.
- **FR-004**: System MUST construct and publish a UserSignupWelcome notification event to RabbitMQ (existing messaging infrastructure) for consumption by the notifications service.
- **FR-005**: System MUST include user display name, first name, email address, and profile URL in the notification event payload.
- **FR-006**: System MUST return HTTP 200 to Kratos upon receiving a valid webhook, regardless of downstream notification delivery status.
- **FR-007**: System MUST log all webhook invocations with correlation IDs for observability.
- **FR-008**: System MUST track which user identities have received a welcome email and skip notification for users who have already been sent one (once-per-identity idempotency).
- **FR-009**: System MUST skip notification emission and log a warning if required user traits (display name, first name, email) are missing from the webhook payload.

### Key Entities

- **Kratos Verification Webhook Payload**: The incoming payload from Ory Kratos containing identity information after successful email verification.
- **UserSignupWelcome Event**: The notification event sent to the notifications service containing user details for welcome email generation.
- **User Identity**: Kratos identity with traits including email, display name, and first name.
- **Welcome Email Sent Record**: Persistent tracking of which identity IDs have already received a welcome email, stored in Redis for durability and fast lookup, enabling once-per-identity idempotency. TTL of 90 days balances storage efficiency with covering edge cases where users might re-verify after extended periods; after 90 days, a duplicate welcome email is acceptable as a re-engagement touchpoint.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Verified users receive welcome emails after completing email verification.
- **SC-002**: Webhook processing does not block or delay the Kratos verification flow.
- **SC-003**: All webhook invocations are traceable through structured logs with correlation IDs.
- **SC-004**: Duplicate webhook deliveries result in at-most-once notification event emission.

## Assumptions

- Ory Kratos is configured to send post-verification webhooks to this gateway.
- The notifications service consumes UserSignupWelcome events from RabbitMQ (existing messaging infrastructure shared across Alkemio services).
- Kratos identity traits include the required user information (display name, first name, email).
- Profile URL can be constructed from the identity ID using a known pattern.
- The webhook endpoint is deployed on an internal network only accessible by Kratos; no authentication mechanism is required at the application layer.
- Redis is available as part of the existing infrastructure for persistent key-value storage.

## Clarifications

### Session 2026-01-26

- Q: How should the webhook endpoint authenticate incoming requests from Kratos? → A: No authentication (internal network only)
- Q: Should duplicate webhooks or re-verification trigger multiple welcome emails? → A: Send welcome email only once per user identity (track sent status)
- Q: How should the gateway communicate with the notifications service? → A: Publish to RabbitMQ (existing messaging infrastructure)
- Q: How should the gateway persist sent-status for idempotency? → A: Redis/key-value store
- Q: What should happen if user profile data is incomplete in the Kratos payload? → A: Skip notification silently, log warning
