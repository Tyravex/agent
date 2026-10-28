package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/commands"
	"github.com/Tyravex/agent/internal/config"
)

// TestPanelInteropInitialSnapshot is run by the Panel Jest interop test.
func TestPanelInteropInitialSnapshot(t *testing.T) {
	panelURL := os.Getenv("TYRAVEX_INTEROP_PANEL_URL")
	if panelURL == "" {
		t.Skip("interop server URL not provided")
	}
	certFile := os.Getenv("TYRAVEX_INTEROP_CERT_FILE")
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("invalid interop certificate")
	}

	client := NewClient(config.Config{
		PanelURL: panelURL, NodeID: "node-1", NodeToken: "interop-token",
	})
	client.httpClient = &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}}
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatalf("connect to real Panel transport: %v", err)
	}
	defer session.Close()
	if session.Welcome().SessionID == "" {
		t.Fatal("welcome missing session ID")
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := manager.Status()
		if status.State == commands.OwnershipReady {
			if status.Revision != 1 {
				t.Fatalf("unexpected revision: %d", status.Revision)
			}
			fmt.Println("INTEROP_READY revision=1")
			session.Close()
			if manager.Status().State != commands.OwnershipNotReady {
				t.Fatal("ownership retained after disconnect")
			}
			return
		}
		select {
		case <-session.Done():
			t.Fatalf("connection closed before READY: %v", session.Err())
		case <-ctx.Done():
			t.Fatal("timeout waiting for ownership snapshot")
		case <-ticker.C:
		}
	}
}

// TestPanelInteropRefresh waits for the real Panel's periodic snapshot.
func TestPanelInteropRefresh(t *testing.T) {
	panelURL := os.Getenv("TYRAVEX_INTEROP_PANEL_URL")
	if panelURL == "" {
		t.Skip("interop server URL not provided")
	}
	client := interopClient(t, panelURL)
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	interopWaitRevision(t, ctx, session, manager, 1)
	interopWaitRevision(t, ctx, session, manager, 2)
	fmt.Println("INTEROP_REFRESH revision=2")
}

// TestPanelInteropReconnect verifies a new Panel session and fresh revision sequence.
func TestPanelInteropReconnect(t *testing.T) {
	panelURL := os.Getenv("TYRAVEX_INTEROP_PANEL_URL")
	if panelURL == "" {
		t.Skip("interop server URL not provided")
	}
	client := interopClient(t, panelURL)
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	interopWaitRevision(t, ctx, first, manager, 1)
	oldID := first.Welcome().SessionID
	first.Close()
	if manager.Status().State != commands.OwnershipNotReady {
		t.Fatal("ownership retained after first disconnect")
	}
	second, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Welcome().SessionID == oldID {
		t.Fatal("reconnect reused session ID")
	}
	interopWaitRevision(t, ctx, second, manager, 1)
	fmt.Println("INTEROP_RECONNECT new_session=true revision=1")
}

func interopClient(t *testing.T, panelURL string) *Client {
	t.Helper()
	certPEM, err := os.ReadFile(os.Getenv("TYRAVEX_INTEROP_CERT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("invalid interop certificate")
	}
	client := NewClient(config.Config{PanelURL: panelURL, NodeID: "node-1", NodeToken: "interop-token"})
	client.httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	return client
}

func interopWaitRevision(t *testing.T, ctx context.Context, session *Session, manager *commands.OwnershipConnectionManager, expected uint64) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := manager.Status()
		if status.State == commands.OwnershipReady && status.Revision >= expected {
			if status.Revision != expected {
				t.Fatalf("expected revision %d, got %d", expected, status.Revision)
			}
			return
		}
		select {
		case <-session.Done():
			t.Fatalf("session closed before revision %d: %v", expected, session.Err())
		case <-ctx.Done():
			t.Fatalf("timeout waiting for revision %d: %v", expected, ctx.Err())
		case <-ticker.C:
		}
	}
}
