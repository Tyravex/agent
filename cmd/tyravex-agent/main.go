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
				log.Println("Usage: tyravex-agent version [--json]")
				os.Exit(1)
			}

			if len(os.Args) == 3 {
				data, err := version.JSON()
				if err != nil {
					log.Printf("Version error: %v", err)
					os.Exit(1)
				}

				fmt.Println(string(data))
				return
			}

			info := version.Current()

			fmt.Printf("Tyravex Agent v%s\n", info.Version)
			fmt.Printf("Commit: %s\n", info.Commit)
			fmt.Printf("Built: %s\n", info.BuildDate)
			fmt.Printf("Go: %s\n", info.GoVersion)
			fmt.Printf("Platform: %s/%s\n", info.OS, info.Arch)
			return

		case "docker-info":
			if err := printDockerInfo(ctx); err != nil {
				log.Printf("Docker error: %v", err)
				os.Exit(1)
			}
			return

		default:
			log.Printf("Unknown command: %s", os.Args[1])
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("Configuration error: %v", err)
		os.Exit(1)
	}

	address, err := config.ValidateAPIListen(cfg.APIListen)
	if err != nil {
		log.Printf("API configuration error: %v", err)
		os.Exit(1)
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
		log.Printf("HTTP API listen error: %v", err)
		os.Exit(1)
	}

	listenErr := make(chan error, 1)

	go func() {
		log.Printf("Tyravex Agent HTTP API listening on %s", listener.Addr())

		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
		}
	}()

	runnerCtx, cancelRunner := context.WithCancel(ctx)
	defer cancelRunner()

	runnerDone := make(chan struct{})

	go func() {
		defer close(runnerDone)
		agent.NewRunner(cfg).Run(runnerCtx)
	}()

	select {
	case <-ctx.Done():
	case err := <-listenErr:
		log.Printf("HTTP API error: %v", err)
	}

	cancelRunner()

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancelShutdown()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP API shutdown error: %v", err)
	}

	<-runnerDone
	log.Println("Tyravex Agent shutdown complete")
}

func printDockerInfo(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.DockerEnabled {
		return fmt.Errorf("Docker integration is disabled in config.yml")
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
