package agent

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
	"github.com/Tyravex/agent/internal/transport"
)

const (
	reconnectInitialDelay = 2 * time.Second
	reconnectMaxDelay     = 30 * time.Second
)

type Runner struct {
	config    config.Config
	transport *transport.Client
	startedAt time.Time
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{
		config:    cfg,
		transport: transport.NewClient(cfg),
		startedAt: time.Now(),
	}
}

func (r *Runner) Run(ctx context.Context) {
	log.Printf("Tyravex Agent started, node: %s", r.config.NodeID)
	defer log.Println("Tyravex Agent stopped")

	reconnectDelay := reconnectInitialDelay

	for ctx.Err() == nil {
		session, err := r.transport.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			log.Printf("Panel WebSocket connection failed: %v", err)
		} else {
			welcome := session.Welcome()

			log.Printf(
				"Panel WebSocket connected, heartbeat interval: %ds",
				welcome.HeartbeatIntervalSeconds,
			)

			connectedAt := time.Now()

			r.runSession(ctx, session)
			session.Close()

			// Reset backoff only after a reasonably stable connection.
			// A successful handshake alone must not reset the retry delay.
			if time.Since(connectedAt) >= 30*time.Second {
				reconnectDelay = reconnectInitialDelay
			}

			if ctx.Err() != nil {
				return
			}

			log.Printf("Panel WebSocket disconnected: %v", session.Err())
		}

		if !waitForReconnect(ctx, reconnectDelay) {
			return
		}

		reconnectDelay *= 2
		if reconnectDelay > reconnectMaxDelay {
			reconnectDelay = reconnectMaxDelay
		}
	}
}

func (r *Runner) runSession(
	ctx context.Context,
	session *transport.Session,
) {
	interval := time.Duration(
		session.Welcome().HeartbeatIntervalSeconds,
	) * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Send the first heartbeat immediately after the handshake.
	if !r.sendHeartbeat(ctx, session) {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-session.Done():
			return

		case <-ticker.C:
			if !r.sendHeartbeat(ctx, session) {
				return
			}
		}
	}
}

func (r *Runner) sendHeartbeat(
	ctx context.Context,
	session *transport.Session,
) bool {
	if ctx.Err() != nil {
		return false
	}

	uptime := int64(time.Since(r.startedAt) / time.Second)
	if uptime < 0 {
		uptime = 0
	}

	heartbeat := transport.Heartbeat{
		UptimeSeconds: uptime,
		Docker:        r.dockerStatus(ctx),
	}

	if ctx.Err() != nil {
		return false
	}

	if err := session.SendHeartbeat(ctx, heartbeat); err != nil {
		if ctx.Err() == nil {
			log.Printf("WebSocket heartbeat failed: %v", err)
		}
		return false
	}

	log.Println("WebSocket heartbeat OK")
	return true
}

func (r *Runner) dockerStatus(
	ctx context.Context,
) transport.DockerStatus {
	unavailable := transport.DockerStatus{
		Available: false,
		Version:   nil,
	}

	if !r.config.DockerEnabled || ctx.Err() != nil {
		return unavailable
	}

	client, err := docker.NewClientWithHost(r.config.DockerHost)
	if err != nil {
		log.Printf("Docker client initialization failed: %v", err)
		return unavailable
	}
	defer client.Close()

	info, err := client.Inspect(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("Docker inspection failed: %v", err)
		}
		return unavailable
	}

	version := strings.TrimSpace(info.ServerVersion)
	if version == "" || len(version) > 128 {
		return unavailable
	}

	return transport.DockerStatus{
		Available: true,
		Version:   &version,
	}
}

func waitForReconnect(
	ctx context.Context,
	delay time.Duration,
) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}
