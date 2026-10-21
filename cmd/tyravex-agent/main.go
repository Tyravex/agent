package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tyravex/agent/internal/agent"
	"github.com/Tyravex/agent/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Configuration error: %v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	runner := agent.NewRunner(cfg)
	runner.Run(ctx)
}
