package kratosverification

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/alkem-io/webhook-gateway/internal/clients"
)

// Service handles the business logic for Kratos verification webhooks.
type Service struct {
	redisClient    *clients.RedisClient
	rabbitMQClient *clients.RabbitMQClient
	platformURL    string
	logger         *zap.Logger
}

// NewService creates a new webhook service.
func NewService(redis *clients.RedisClient, rabbitmq *clients.RabbitMQClient, platformURL string, logger *zap.Logger) *Service {
	return &Service{
		redisClient:    redis,
		rabbitMQClient: rabbitmq,
		platformURL:    platformURL,
		logger:         logger,
	}
}

// ValidationError represents a payload validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidatePayload validates the Kratos verification payload.
func (s *Service) ValidatePayload(payload *KratosVerificationPayload) []ValidationError {
	var errors []ValidationError

	if strings.TrimSpace(payload.IdentityID) == "" {
		errors = append(errors, ValidationError{Field: "identity_id", Message: "required"})
	}
	if strings.TrimSpace(payload.Email) == "" {
		errors = append(errors, ValidationError{Field: "email", Message: "required"})
	}
	if strings.TrimSpace(payload.DisplayName) == "" {
		errors = append(errors, ValidationError{Field: "display_name", Message: "required"})
	}
	if strings.TrimSpace(payload.FirstName) == "" {
		errors = append(errors, ValidationError{Field: "first_name", Message: "required"})
	}

	return errors
}

// TransformToNotificationEvent converts a Kratos payload to a notification event.
func (s *Service) TransformToNotificationEvent(payload *KratosVerificationPayload) UserSignupWelcomeEvent {
	user := UserPayload{
		ID:        payload.IdentityID,
		FirstName: payload.FirstName,
		LastName:  "", // Not provided by Kratos traits
		Email:     payload.Email,
		Profile: ProfileInfo{
			DisplayName: payload.DisplayName,
			URL:         fmt.Sprintf("%s/user/%s", s.platformURL, payload.IdentityID),
		},
		Type: UserType,
	}

	return UserSignupWelcomeEvent{
		EventType:   EventTypeUserSignUpWelcome,
		TriggeredBy: user,
		Recipients:  []UserPayload{user},
		Platform: PlatformInfo{
			URL: s.platformURL,
		},
	}
}

// CheckAndMarkWelcomeSent checks if welcome email was already sent and marks it if not.
// Returns true if welcome should be sent (first time), false if already sent.
// On Redis error, returns true (fail-open).
func (s *Service) CheckAndMarkWelcomeSent(ctx context.Context, identityID string, correlationID string) bool {
	isNew, err := s.redisClient.MarkWelcomeSentIfNew(ctx, identityID)
	if err != nil {
		s.logger.Warn("redis unavailable for idempotency check, proceeding with notification",
			zap.Error(err),
			zap.String("correlation_id", correlationID),
			zap.String("identity_id", identityID),
		)
		return true // Fail-open: allow potential duplicate rather than blocking
	}
	return isNew
}

// PublishNotificationEvent publishes the notification event to RabbitMQ.
// Returns nil even on failure (fail-open semantics).
func (s *Service) PublishNotificationEvent(ctx context.Context, event UserSignupWelcomeEvent, correlationID string) error {
	err := s.rabbitMQClient.Publish(ctx, event)
	if err != nil {
		s.logger.Warn("failed to publish notification event",
			zap.Error(err),
			zap.String("correlation_id", correlationID),
			zap.String("identity_id", event.TriggeredBy.ID),
		)
		// Return error to indicate failure but caller should still return HTTP 200
		return err
	}

	s.logger.Info("notification event published",
		zap.String("correlation_id", correlationID),
		zap.String("identity_id", event.TriggeredBy.ID),
		zap.String("event_type", event.EventType),
	)
	return nil
}
