package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tyravex/agent/internal/agent"
	"github.com/Tyravex/agent/internal/api"
	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
	"github.com/Tyravex/agent/internal/version"
)

func main() {
	if err := run(); err != nil {
		log.Printf("Tyravex Agent error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			if len(os.Args) > 3 ||
				(len(os.Args) == 3 && os.Args[2] != "--json") {
				return errors.New(
					"usage: tyravex-agent version [--json]",
				)
			}

			if len(os.Args) == 3 {
				data, err := version.JSON()
				if err != nil {
					return fmt.Errorf("version JSON: %w", err)
				}

				fmt.Println(string(data))
				return nil
			}

			info := version.Current()

			fmt.Printf("Tyravex Agent v%s\n", info.Version)
			fmt.Printf("Commit: %s\n", info.Commit)
			fmt.Printf("Built: %s\n", info.BuildDate)
			fmt.Printf("Go: %s\n", info.GoVersion)
			fmt.Printf("Platform: %s/%s\n", info.OS, info.Arch)

			return nil

		case "docker-info":
			return printDockerInfo(ctx)

		default:
			return fmt.Errorf(
				"unknown command: %s",
				os.Args[1],
			)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	return runAgent(ctx, cfg)
}

func runAgent(ctx context.Context, cfg config.Config) error {
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
		agent.NewRunner(cfg).Run(runnerCtx)
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

func printDockerInfo(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.DockerEnabled {
		return fmt.Errorf(
			"Docker integration is disabled in config.yml",
		)
	}

	dockerClient, err := docker.NewClientWithHost(cfg.DockerHost)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	info, err := dockerClient.Inspect(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Tyravex Agent - Docker diagnostics")
	fmt.Printf("Docker Engine: %s\n", info.ServerVersion)
	fmt.Printf("Containers: %d\n", len(info.Containers))

	for _, container := range info.Containers {
		fmt.Printf(
			"  %s | %s | %s\n",
			container.Name,
			container.Image,
			container.Status,
		)
	}

	return nil
}
