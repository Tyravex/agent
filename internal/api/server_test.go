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
		NodeToken:     "test-secret-token",
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
	for _, token := range []string{"", "wrong-token"} {
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
	req.Header.Set("Authorization", "Bearer test-secret-token")

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

func TestDockerDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docker", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")

	rec := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
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

func TestListenAddress(t *testing.T) {
	valid := []string{
		"127.0.0.1:8080",
		"[::1]:8080",
	}

	for _, address := range valid {
		if _, err := ListenAddress(address); err != nil {
			t.Errorf("expected valid address %q: %v", address, err)
		}
	}

	invalid := []string{
		"0.0.0.0:8080",
		":8080",
		"192.168.1.10:8080",
		"localhost:8080",
		"invalid",
	}

	for _, address := range invalid {
		if _, err := ListenAddress(address); err == nil {
			t.Errorf("expected invalid address: %q", address)
		}
	}
}
