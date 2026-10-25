package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Spec is an engine-independent request for a managed service.
// The Docker adapter is responsible for translating it into Docker API calls.
type Spec struct {
	ID         string
	Image      string
	Startup    string
	MemoryMB   int64
	CPUPercent int64
	DiskMB     int64
}

var serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

const maxSafeInteger int64 = 9007199254740991

func (s Spec) Validate() error {
	if !serviceIDPattern.MatchString(s.ID) {
		return errors.New("invalid service ID")
	}
	if strings.TrimSpace(s.Image) != s.Image || s.Image == "" || len(s.Image) > 512 || strings.ContainsAny(s.Image, " \t\r\n") {
		return errors.New("invalid container image")
	}
	if strings.TrimSpace(s.Startup) == "" || len(s.Startup) > 2048 {
		return errors.New("invalid startup command")
	}
	if s.MemoryMB < 64 || s.MemoryMB > maxSafeInteger/1048576 {
		return errors.New("invalid memory limit")
	}
	if s.CPUPercent < 1 || s.CPUPercent > 100 {
		return errors.New("invalid CPU percentage")
	}
	if s.DiskMB < 1 || s.DiskMB > maxSafeInteger/1048576 {
		return errors.New("invalid disk limit")
	}
	return nil
}

// ContainerName provides a stable name for retry-safe container lookup.
func (s Spec) ContainerName() (string, error) {
	if err := s.Validate(); err != nil {
		return "", fmt.Errorf("container name: %w", err)
	}
	return "tyravex-" + s.ID, nil
}
