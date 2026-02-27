// Package kratosloginbackoff handles login brute force protection webhooks.
package kratosloginbackoff

// BeforeLoginRequest is the incoming request to check and increment login attempt counters.
type BeforeLoginRequest struct {
	FlowID     string `json:"flow_id"`
	Identifier string `json:"identifier"`
	ClientIP   string `json:"client_ip"`
}

// BeforeLoginAllowedResponse is returned when a login attempt is allowed.
type BeforeLoginAllowedResponse struct {
	Allowed            bool  `json:"allowed"`
	IdentifierAttempts int64 `json:"identifier_attempts"`
	IPAttempts         int64 `json:"ip_attempts"`
}

// BeforeLoginBlockedResponse is returned when a login attempt is blocked due to lockout.
type BeforeLoginBlockedResponse struct {
	Allowed           bool   `json:"allowed"`
	Reason            string `json:"reason"`
	Message           string `json:"message"`
	RetryAfterSeconds int64  `json:"retry_after_seconds"`
}

// AfterLoginRequest is the incoming request to reset counters after successful authentication.
type AfterLoginRequest struct {
	IdentityID string `json:"identity_id"`
	Email      string `json:"email"`
	ClientIP   string `json:"client_ip"`
}

// AfterLoginResponse is returned after processing a successful login notification.
type AfterLoginResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Response status constants.
const (
	StatusSuccess = "success"
	StatusSkipped = "skipped"
)
