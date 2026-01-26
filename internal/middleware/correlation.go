// Package middleware provides HTTP middleware components.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// CorrelationIDKey is the context key for correlation ID.
type correlationIDKey struct{}

// CorrelationID creates middleware that extracts or generates correlation IDs.
func CorrelationID(headerName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := r.Header.Get(headerName)
			if correlationID == "" {
				correlationID = uuid.New().String()
			}

			// Add correlation ID to response header
			w.Header().Set(headerName, correlationID)

			// Store in context
			ctx := context.WithValue(r.Context(), correlationIDKey{}, correlationID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetCorrelationID retrieves the correlation ID from context.
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return id
	}
	return ""
}
