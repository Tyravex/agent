package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler:           testServer().Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	done := make(chan error, 1)

	go func() {
		done <- server.Serve(listener)
	}()

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	response, err := client.Get(
		"http://" + listener.Addr().String() + "/health",
	)
	if err != nil {
		t.Fatalf("HTTP server request failed: %v", err)
	}
	response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("HTTP shutdown failed: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("unexpected server result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not stop")
	}
}

func TestHTTPListenPortOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	_, err = net.Listen("tcp", listener.Addr().String())
	if err == nil {
		t.Fatal("expected occupied port error")
	}
}
