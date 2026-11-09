package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	agent "github.com/Tyravex/agent/internal/app"
	version "github.com/Tyravex/agent/internal/buildinfo"
	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/runtime/docker"
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

	return agent.Run(ctx, cfg)
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
