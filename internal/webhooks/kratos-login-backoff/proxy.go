package kratosloginbackoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/alkem-io/webhook-gateway/internal/middleware"
)

// kratosLoginBody represents the fields we need from a Kratos login POST.
type kratosLoginBody struct {
	Identifier string `json:"identifier"`
	Method     string `json:"method"`
}

// NewLoginProxy creates an http.Handler that checks login backoff before proxying to Kratos.
func NewLoginProxy(kratosURL string, service *Service, logger *zap.Logger) http.Handler {
	target, err := url.Parse(kratosURL)
	if err != nil {
		logger.Fatal("invalid kratos internal URL", zap.Error(err), zap.String("url", kratosURL))
	}

	proxy := httputil.NewSingleHostReverseProxy(target) //nolint:gosec // target is from trusted config

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := middleware.GetCorrelationID(r.Context())

		// Only intercept POST requests (credential submissions)
		if r.Method != http.MethodPost {
			proxy.ServeHTTP(w, r) //nolint:gosec // proxy target is from trusted config
			return
		}

		// Read body so we can inspect it and still forward it
		bodyBytes, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			logger.Warn("failed to read login request body, proxying without backoff check",
				zap.Error(err),
				zap.String("correlation_id", correlationID),
			)
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			proxy.ServeHTTP(w, r) //nolint:gosec // proxy target is from trusted config
			return
		}

		// Extract identifier from the body
		identifier := extractIdentifier(bodyBytes, r.Header.Get("Content-Type"))

		// Extract client IP
		clientIP := extractClientIP(r)

		// Check backoff
		result := service.CheckAndIncrement(r.Context(), BeforeLoginRequest{
			Identifier: identifier,
			ClientIP:   clientIP,
		}, correlationID)

		if !result.Allowed {
			logger.Warn("login proxy blocked request",
				zap.String("correlation_id", correlationID),
				zap.String("client_ip", clientIP),
				zap.String("reason", result.Reason),
				zap.Int64("retry_after_seconds", result.RetryAfterSeconds),
			)

			// Browser requests (native form POST): redirect to login page with lockout params
			if isBrowserRequest(r) {
				lockoutURL := fmt.Sprintf("/login?lockout=true&retry_after=%d", result.RetryAfterSeconds)
				http.Redirect(w, r, lockoutURL, http.StatusSeeOther)
				return
			}

			// API requests: return 429 JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    429,
					"status":  "Too Many Requests",
					"reason":  result.Reason,
					"message": result.Message,
				},
			})
			return
		}

		// Restore body and proxy to Kratos
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		proxy.ServeHTTP(w, r) //nolint:gosec // proxy target is from trusted config
	})
}

// extractIdentifier tries to parse the identifier from the request body.
func extractIdentifier(body []byte, contentType string) string {
	if strings.Contains(contentType, "application/json") {
		var parsed kratosLoginBody
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Identifier != "" {
			return parsed.Identifier
		}
	}

	// Try form-encoded
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(string(body))
		if err == nil {
			if id := values.Get("identifier"); id != "" {
				return id
			}
		}
	}

	// Fallback: try JSON even if content-type doesn't say so
	var parsed kratosLoginBody
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Identifier != "" {
		return parsed.Identifier
	}

	return ""
}

// isBrowserRequest returns true if the request is from a browser (native form POST)
// rather than an API client. Browser requests accept text/html; API clients accept application/json.
func isBrowserRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// extractClientIP gets the client IP from headers or RemoteAddr.
func extractClientIP(r *http.Request) string {
	// Check True-Client-Ip first (Cloudflare-style)
	if ip := r.Header.Get("True-Client-Ip"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-Ip
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}

	// Fall back to RemoteAddr (strip port)
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
