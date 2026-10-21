package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
	"github.com/Tyravex/agent/internal/version"
)

type Server struct {
	cfg     config.Config
	started time.Time
	handler http.Handler
}

func NewServer(cfg config.Config) *Server {
	s := &Server{
		cfg:     cfg,
		started: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.Handle("GET /api/v1/status", s.authorize(http.HandlerFunc(s.status)))
	mux.Handle("GET /api/v1/docker", s.authorize(http.HandlerFunc(s.dockerInfo)))

	s.handler = mux
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "

		if !strings.HasPrefix(header, prefix) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}

		provided := strings.TrimPrefix(header, prefix)

		expectedHash := sha256.Sum256([]byte(s.cfg.NodeToken))
		providedHash := sha256.Sum256([]byte(provided))

		if provided == "" ||
			subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status        string       `json:"status"`
		NodeID        string       `json:"nodeId"`
		UptimeSeconds int64        `json:"uptimeSeconds"`
		Version       version.Info `json:"version"`
		DockerEnabled bool         `json:"dockerEnabled"`
	}{
		Status:        "online",
		NodeID:        s.cfg.NodeID,
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
		Version:       version.Current(),
		DockerEnabled: s.cfg.DockerEnabled,
	})
}

func (s *Server) dockerInfo(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DockerEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "docker_disabled",
		})
		return
	}

	c, err := docker.NewClientWithHost(s.cfg.DockerHost)
	if err != nil {
		log.Printf("Docker API client error: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "docker_unavailable",
		})
		return
	}
	defer c.Close()

	info, err := c.Inspect(r.Context())
	if err != nil {
		log.Printf("Docker API inspection error: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "docker_unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("HTTP response encoding error: %v", err)
	}
}

func ListenAddress(value string) (string, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return "", &net.AddrError{Err: "invalid listen address", Addr: value}
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", &net.AddrError{
			Err:  "API must listen on a loopback IP address",
			Addr: value,
		}
	}

	return value, nil
}
