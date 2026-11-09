package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Tyravex/agent/internal/config"
	api "github.com/Tyravex/agent/internal/transport/httpapi"
)

func Run(ctx context.Context, cfg config.Config) error {
	address, err := config.ValidateAPIListen(cfg.APIListen)
	if err != nil {
		return fmt.Errorf("API configuration: %w", err)
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           api.NewServer(cfg).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("HTTP API listen: %w", err)
	}
	defer listener.Close()

	listenErr := make(chan error, 1)

	go func() {
		log.Printf(
			"Tyravex Agent HTTP API listening on %s",
			listener.Addr(),
		)

		listenErr <- httpServer.Serve(listener)
	}()

	runnerCtx, cancelRunner := context.WithCancel(ctx)
	defer cancelRunner()

	runnerDone := make(chan struct{})

	go func() {
		defer close(runnerDone)
		NewRunner(cfg).Run(runnerCtx)
	}()

	var serverErr error

	select {
	case <-ctx.Done():
	case err := <-listenErr:
		if !errors.Is(err, http.ErrServerClosed) {
			serverErr = fmt.Errorf("HTTP API server: %w", err)
		}
	}

	cancelRunner()

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancelShutdown()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		serverErr = errors.Join(
			serverErr,
			fmt.Errorf("HTTP API shutdown: %w", err),
		)

		if closeErr := httpServer.Close(); closeErr != nil {
			serverErr = errors.Join(
				serverErr,
				fmt.Errorf("HTTP API close: %w", closeErr),
			)
		}
	}

	<-runnerDone

	log.Println("Tyravex Agent shutdown complete")

	return serverErr
}
