package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/config"
	"github.com/coder/websocket"
)

func TestPersistentClientCannotCloseWithActiveSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(
				r.Context(),
				5*time.Second,
			)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			// Keep the connection alive until the
			// client closes its session.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	path := filepath.Join(t.TempDir(), "commands.jsonl")
	cfg := sessionTestClient(server).config

	client, err := NewClientWithPersistentRegistry(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.ClosePersistentRegistry()

	session, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := client.ClosePersistentRegistry(); !errors.Is(
		err,
		ErrClientBusy,
	) {
		t.Fatalf("expected busy client, got %v", err)
	}

	// The original client must still own the journal.
	_, err = NewClientWithPersistentRegistry(cfg, path)
	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf("expected locked journal, got %v", err)
	}

	session.Close()

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatalf("close after session shutdown: %v", err)
	}

	// Once the original owner closes, another client
	// must be able to open the same journal.
	second, err := NewClientWithPersistentRegistry(cfg, path)
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	defer second.ClosePersistentRegistry()
}

func TestPersistentClientRejectsConnectAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	client, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatal(err)
	}

	_, err = client.Connect(context.Background())
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected closed client, got %v", err)
	}

	_, err = client.ConnectWithExecutor(
		context.Background(),
		&recordingExecutor{},
	)
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected closed client, got %v", err)
	}
}

func TestPersistentClientCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	client, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := client.ClosePersistentRegistry(); err != nil {
			t.Fatalf("close #%d: %v", i+1, err)
		}
	}
}

func TestPersistentClientRejectsCloseDuringConnect(t *testing.T) {
	handshakeStarted := make(chan struct{})
	releaseHandshake := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			close(handshakeStarted)
			<-releaseHandshake
			http.Error(w, "test handshake rejected", http.StatusForbidden)
		},
	))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "commands.jsonl")
	cfg := sessionTestClient(server).config

	client, err := NewClientWithPersistentRegistry(cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.ClosePersistentRegistry()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	connectDone := make(chan error, 1)

	go func() {
		_, err := client.Connect(ctx)
		connectDone <- err
	}()

	select {
	case <-handshakeStarted:
	case <-ctx.Done():
		close(releaseHandshake)
		t.Fatal("connection did not begin")
	}

	if err := client.ClosePersistentRegistry(); !errors.Is(
		err,
		ErrClientBusy,
	) {
		close(releaseHandshake)
		t.Fatalf("expected busy client, got %v", err)
	}

	close(releaseHandshake)

	select {
	case err := <-connectDone:
		if err == nil {
			t.Fatal("expected rejected handshake")
		}
	case <-ctx.Done():
		t.Fatal("connection did not finish")
	}

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatalf("close after failed connect: %v", err)
	}
}
