package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

func testServer() *Server {
	return NewServer(config.Config{
		NodeID:        "test-node",
		NodeToken:     "heartbeat-secret-token",
		APIToken:      "test-api-secret-token",
		DockerEnabled: false,
	})
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	testServer().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if response["status"] != "ok" {
		t.Fatalf("unexpected response: %v", response)
	}
}

func TestStatusUnauthorized(t *testing.T) {
	for _, token := range []string{
		"",
		"wrong-token",
		"heartbeat-secret-token",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)

		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		rec := httptest.NewRecorder()
		testServer().Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	}
}

func TestStatusAuthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-api-secret-token")

	rec := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Status string `json:"status"`
		NodeID string `json:"nodeId"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if response.Status != "online" || response.NodeID != "test-node" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestStatusOwnershipState(t *testing.T) {
	state := "NOT_READY"

	cfg := config.Config{
		NodeID:        "test-node",
		APIToken:      "test-api-secret-token",
		DockerEnabled: false,
	}

	server := NewServer(cfg, func() string {
		return state
	})

	check := func(expected string) {
		t.Helper()

		req := httptest.NewRequest(
			http.MethodGet,
			"/api/v1/status",
			nil,
		)
		req.Header.Set(
			"Authorization",
			"Bearer test-api-secret-token",
		)

		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf(
				"expected 200, got %d: %s",
				rec.Code,
				rec.Body.String(),
			)
		}

		var response struct {
			Status         string `json:"status"`
			NodeID         string `json:"nodeId"`
			OwnershipState string `json:"ownershipState"`
		}

		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}

		if response.Status != "online" ||
			response.NodeID != "test-node" ||
			response.OwnershipState != expected {
			t.Fatalf(
				"unexpected response for %s: %+v",
				expected,
				response,
			)
		}
	}

	check("NOT_READY")

	state = "READY"
	check("READY")
}

func TestDockerDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker", nil)
	req.Header.Set("Authorization", "Bearer test-api-secret-token")

	rec := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected 503, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	testServer().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
