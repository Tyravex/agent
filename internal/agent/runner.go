package agent

import (
	"context"
	"log"
	"time"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/heartbeat"
)

type Runner struct {
	config config.Config
	client *heartbeat.Client
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{
		config: cfg,
		client: heartbeat.NewClient(cfg),
	}
}

func (r *Runner) Run(ctx context.Context) {
	log.Printf("Tyravex Agent started, node: %s", r.config.NodeID)

	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	r.sendHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Tyravex Agent stopped")
			return

		case <-ticker.C:
			r.sendHeartbeat(ctx)
		}
	}
}

func (r *Runner) sendHeartbeat(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	if err := r.client.Send(ctx); err != nil {
		if ctx.Err() == nil {
			log.Printf("Heartbeat failed: %v", err)
		}
		return
	}

	log.Println("Heartbeat OK")
}
