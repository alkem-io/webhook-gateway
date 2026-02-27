package kratosloginbackoff_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/alkem-io/webhook-gateway/internal/config"
	kratosloginbackoff "github.com/alkem-io/webhook-gateway/internal/webhooks/kratos-login-backoff"
)

// setupTestHandler creates a handler backed by a real service with the given Redis client.
func setupTestHandler(t *testing.T, redisClient *mockRedisHelper) *kratosloginbackoff.Handler {
	t.Helper()
	cfg := &config.Config{
		LoginBackoffMaxIdentifierAttempts:    10,
		LoginBackoffMaxIPAttempts:            20,
		LoginBackoffIdentifierLockoutSeconds: 120,
		LoginBackoffIPLockoutSeconds:         120,
	}
	logger := zap.NewNop()
	service := kratosloginbackoff.NewServiceWithRedis(redisClient, cfg, logger)
	return kratosloginbackoff.NewHandler(service, logger)
}

func TestBeforeLogin_AllowedUnderThreshold(t *testing.T) {
	mock := &mockRedisHelper{
		incrementBothResult: [4]int64{3, 118, 5, 117},
	}
	handler := setupTestHandler(t, mock)

	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com",
		"client_ip":  "192.168.1.1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/kratos/login-backoff/before-login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleBeforeLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp kratosloginbackoff.BeforeLoginAllowedResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Allowed {
		t.Error("expected allowed=true")
	}
	if resp.IdentifierAttempts != 3 {
		t.Errorf("expected identifier_attempts=3, got %d", resp.IdentifierAttempts)
	}
	if resp.IPAttempts != 5 {
		t.Errorf("expected ip_attempts=5, got %d", resp.IPAttempts)
	}
}

func TestBeforeLogin_BlockedOverThreshold(t *testing.T) {
	mock := &mockRedisHelper{
		incrementBothResult: [4]int64{11, 95, 5, 117},
	}
	handler := setupTestHandler(t, mock)

	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com",
		"client_ip":  "192.168.1.1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/kratos/login-backoff/before-login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleBeforeLogin(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}

	var resp kratosloginbackoff.BeforeLoginBlockedResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Allowed {
		t.Error("expected allowed=false")
	}
	if resp.Reason != "identifier" {
		t.Errorf("expected reason=identifier, got %s", resp.Reason)
	}
	if resp.RetryAfterSeconds != 95 {
		t.Errorf("expected retry_after_seconds=95, got %d", resp.RetryAfterSeconds)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestBeforeLogin_FailOpenOnMalformedInput(t *testing.T) {
	mock := &mockRedisHelper{}
	handler := setupTestHandler(t, mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/kratos/login-backoff/before-login", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleBeforeLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on malformed input, got %d", rec.Code)
	}

	var resp kratosloginbackoff.BeforeLoginAllowedResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Allowed {
		t.Error("expected allowed=true on malformed input (fail-open)")
	}
}

func TestAfterLogin_ReturnsSuccess(t *testing.T) {
	mock := &mockRedisHelper{}
	handler := setupTestHandler(t, mock)

	body, _ := json.Marshal(map[string]string{
		"identity_id": "some-uuid",
		"email":       "user@example.com",
		"client_ip":   "192.168.1.1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/kratos/login-backoff/after-login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleAfterLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp kratosloginbackoff.AfterLoginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status=success, got %s", resp.Status)
	}
	if resp.Message != "counters reset" {
		t.Errorf("expected message='counters reset', got %s", resp.Message)
	}
}

func TestBeforeLogin_ContentTypeJSON(t *testing.T) {
	mock := &mockRedisHelper{
		incrementBothResult: [4]int64{1, 120, 1, 120},
	}
	handler := setupTestHandler(t, mock)

	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com",
		"client_ip":  "192.168.1.1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/kratos/login-backoff/before-login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.HandleBeforeLogin(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
