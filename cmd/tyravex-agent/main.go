package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tyravex/agent/internal/agent"
	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
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

	runner := agent.NewRunner(cfg)
	runner.Run(ctx)
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
