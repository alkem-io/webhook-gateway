package kratosloginbackoff

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/alkem-io/webhook-gateway/internal/middleware"
)

// Handler handles HTTP requests for login backoff webhooks.
type Handler struct {
	service *Service
	logger  *zap.Logger
}

// NewHandler creates a new login backoff handler.
func NewHandler(service *Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// HandleBeforeLogin handles POST /api/v1/webhooks/kratos/login-backoff/before-login.
func (h *Handler) HandleBeforeLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := middleware.GetCorrelationID(ctx)

	var req BeforeLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("failed to decode before-login request, failing open",
			zap.Error(err),
			zap.String("correlation_id", correlationID),
		)
		h.respondJSON(w, http.StatusOK, BeforeLoginAllowedResponse{Allowed: true})
		return
	}

	result := h.service.CheckAndIncrement(ctx, req, correlationID)

	if result.Allowed {
		h.respondJSON(w, http.StatusOK, BeforeLoginAllowedResponse{
			Allowed:            true,
			IdentifierAttempts: result.IdentifierAttempts,
			IPAttempts:         result.IPAttempts,
		})
		return
	}

	h.respondJSON(w, http.StatusForbidden, BeforeLoginBlockedResponse{
		Allowed:           false,
		Reason:            result.Reason,
		Message:           result.Message,
		RetryAfterSeconds: result.RetryAfterSeconds,
	})
}

// HandleAfterLogin handles POST /api/v1/webhooks/kratos/login-backoff/after-login.
func (h *Handler) HandleAfterLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := middleware.GetCorrelationID(ctx)

	var req AfterLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("failed to decode after-login request, failing open",
			zap.Error(err),
			zap.String("correlation_id", correlationID),
		)
		h.respondJSON(w, http.StatusOK, AfterLoginResponse{
			Status:  StatusSkipped,
			Message: "invalid payload; reset skipped",
		})
		return
	}

	resp := h.service.ResetCounters(ctx, req, correlationID)
	h.respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
