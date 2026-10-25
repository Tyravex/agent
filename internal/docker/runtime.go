package docker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Runtime manages Tyravex-owned Linux containers through the Docker CLI.
// This is an isolated MVP adapter; the existing Docker SDK client is unchanged.
type Runtime struct {
	binary  string
	timeout time.Duration
}

type ServerSpec struct {
	ID            string
	Image         string
	MemoryMB      int64
	CPUPercent    int64
	Port          uint16
	ContainerPort uint16
	Env           map[string]string
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (s ServerSpec) Validate() error {
	if !safeID.MatchString(s.ID) {
		return errors.New("invalid server ID")
	}
	if strings.TrimSpace(s.Image) == "" || len(s.Image) > 512 || strings.ContainsAny(s.Image, " \t\r\n") || strings.HasPrefix(s.Image, "-") {
		return errors.New("invalid image")
	}
	if s.MemoryMB < 128 || s.MemoryMB > 1048576 {
		return errors.New("memoryMB must be 128..1048576")
	}
	if s.CPUPercent < 1 || s.CPUPercent > 100 {
		return errors.New("cpuPercent must be 1..100")
	}
	if s.Port == 0 || s.ContainerPort == 0 {
		return errors.New("ports must be nonzero")
	}
	for k, v := range s.Env {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(k) || strings.ContainsRune(v, '\x00') {
			return fmt.Errorf("invalid environment variable %q", k)
		}
	}
	return nil
}

func NewRuntime() *Runtime { return &Runtime{binary: "docker", timeout: 5 * time.Minute} }

func (r *Runtime) run(ctx context.Context, args ...string) (string, error) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary := r.binary
	if binary == "" {
		binary = "docker"
	}
	out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func containerName(id string) string { return "tyravex-" + id }
func volumeName(id string) string    { return "tyravex-data-" + id }

// Install creates a persistent, labeled container and starts it.
// Existing containers are never replaced, preventing accidental data loss.
func (r *Runtime) Install(ctx context.Context, s ServerSpec) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	name := containerName(s.ID)
	if _, err := r.run(ctx, "image", "pull", s.Image); err != nil {
		return "", err
	}
	if _, err := r.run(ctx, "volume", "create", "--label", "dev.tyravex.managed=true", "--label", "dev.tyravex.service="+s.ID, volumeName(s.ID)); err != nil {
		return "", err
	}
	args := []string{"container", "create", "--name", name, "--label", "dev.tyravex.managed=true", "--label", "dev.tyravex.service=" + s.ID, "--memory", strconv.FormatInt(s.MemoryMB, 10) + "m", "--cpus", strconv.FormatFloat(float64(s.CPUPercent)/100, 'f', 2, 64), "--restart", "unless-stopped", "--publish", fmt.Sprintf("%d:%d/tcp", s.Port, s.ContainerPort), "--mount", "type=volume,source=" + volumeName(s.ID) + ",target=/data", "--security-opt", "no-new-privileges:true", "--cap-drop", "ALL"}
	// Deterministic environment order is useful for audit and tests.
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		args = append(args, "--env", k+"="+s.Env[k])
	}
	args = append(args, s.Image)
	id, err := r.run(ctx, args...)
	if err != nil {
		return "", err
	}
	if _, err = r.run(ctx, "container", "start", name); err != nil {
		return id, err
	}
	return id, nil
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (r *Runtime) ensureOwned(ctx context.Context, id string) error {
	if !safeID.MatchString(id) {
		return errors.New("invalid server ID")
	}
	value, err := r.run(ctx, "container", "inspect", "--format", "{{index .Config.Labels \"dev.tyravex.service\"}}", containerName(id))
	if err != nil {
		return err
	}
	if value != id {
		return errors.New("container is not owned by Tyravex")
	}
	return nil
}

func (r *Runtime) Start(ctx context.Context, id string) error {
	if err := r.ensureOwned(ctx, id); err != nil {
		return err
	}
	_, err := r.run(ctx, "container", "start", containerName(id))
	return err
}
func (r *Runtime) Stop(ctx context.Context, id string) error {
	if err := r.ensureOwned(ctx, id); err != nil {
		return err
	}
	_, err := r.run(ctx, "container", "stop", "--time", "30", containerName(id))
	return err
}
func (r *Runtime) Restart(ctx context.Context, id string) error {
	if err := r.ensureOwned(ctx, id); err != nil {
		return err
	}
	_, err := r.run(ctx, "container", "restart", "--time", "30", containerName(id))
	return err
}
func (r *Runtime) Status(ctx context.Context, id string) (string, error) {
	if err := r.ensureOwned(ctx, id); err != nil {
		return "", err
	}
	return r.run(ctx, "container", "inspect", "--format", "{{.State.Status}}", containerName(id))
}
func (r *Runtime) Logs(ctx context.Context, id string, lines int) (string, error) {
	if err := r.ensureOwned(ctx, id); err != nil {
		return "", err
	}
	if lines < 1 || lines > 1000 {
		return "", errors.New("lines must be 1..1000")
	}
	return r.run(ctx, "container", "logs", "--tail", strconv.Itoa(lines), containerName(id))
}
