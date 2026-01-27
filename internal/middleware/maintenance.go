package middleware

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// MaintenanceResponse is the response body during maintenance mode.
type MaintenanceResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Maintenance creates middleware that returns 503 when maintenance mode is enabled.
func Maintenance(enabled bool, message string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow health endpoints
			if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
				next.ServeHTTP(w, r)
				return
			}

			if enabled {
				correlationID := GetCorrelationID(r.Context())
				logger.Info("request rejected due to maintenance mode",
					zap.String("correlation_id", correlationID),
					zap.String("path", r.URL.Path),
				)

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "300")
				w.WriteHeader(http.StatusServiceUnavailable)

				resp := MaintenanceResponse{
					Status:  "unavailable",
					Message: message,
				}
				json.NewEncoder(w).Encode(resp)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
