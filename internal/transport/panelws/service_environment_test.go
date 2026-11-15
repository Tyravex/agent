package panelws

import (
	"context"
	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/transport/protocol"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEnvironmentClientRejectsHTTP(t *testing.T) {
	client := &Client{config: config.Config{PanelURL: "http://127.0.0.1:3000", NodeID: "node-1", NodeToken: "test"}}
	if _, err := client.FetchServiceEnvironment(context.Background(), protocol.EnvironmentBinding{NodeID: "node-1"}, nil); err == nil {
		t.Fatal("accepted insecure delivery")
	}
}
func TestEnvironmentClientNeverForwardsTokenAcrossRedirect(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := &Client{config: config.Config{PanelURL: source.URL, NodeID: "node-1", NodeToken: "test"}, httpClient: source.Client()}
	if _, err := client.FetchServiceEnvironment(context.Background(), protocol.EnvironmentBinding{NodeID: "node-1"}, nil); err == nil {
		t.Fatal("accepted redirect")
	}
	if targetHits.Load() != 0 {
		t.Fatal("followed redirect with Agent authentication")
	}
}
