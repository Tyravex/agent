package main

import (
	"strings"
	"testing"
)

// These tests intentionally avoid invoking Docker Engine.
func TestUsageAndInvalidArguments(t *testing.T) {
	tests := [][]string{
		{},
		{"unknown"},
		{"start"},
		{"stop"},
		{"restart"},
		{"status"},
		{"logs"},
		{"logs", "-lines", "nope", "mc-test"},
		{"minecraft-install", "-port", "0"},
		{"minecraft-install", "-port", "65536"},
		{"minecraft-install", "-memory", "512"},
		{"minecraft-install", "extra"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if err := run(args); err == nil {
				t.Fatalf("expected error for %q", args)
			}
		})
	}
}
