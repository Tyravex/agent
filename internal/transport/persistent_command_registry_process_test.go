package transport

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const registryHelperEnv = "TYRAVEX_REGISTRY_TEST_HELPER"

func TestPersistentRegistryProcessHelper(t *testing.T) {
	if os.Getenv(registryHelperEnv) != "1" {
		return
	}

	path := os.Getenv("TYRAVEX_REGISTRY_TEST_PATH")
	if path == "" {
		t.Fatal("missing registry path")
	}

	registry, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf("helper open registry: %v", err)
	}
	defer registry.Close()

	// The parent waits for this exact message before
	// attempting to open the journal itself.
	fmt.Println("REGISTRY_LOCK_ACQUIRED")

	// Keep the process alive until the parent terminates it.
	// stdin is connected to a pipe owned by the parent.
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
}

func TestPersistentRegistryLockAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test")
	}

	path := filepath.Join(t.TempDir(), "commands.jsonl")
	lockPath := path + ".lock"

	cmd := exec.Command(
		os.Args[0],
		"-test.run=^TestPersistentRegistryProcessHelper$",
		"-test.v",
	)

	cmd.Env = append(
		os.Environ(),
		registryHelperEnv+"=1",
		"TYRAVEX_REGISTRY_TEST_PATH="+path,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create helper stdin: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("create helper stdout: %v", err)
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	processFinished := false
	defer func() {
		_ = stdin.Close()

		if !processFinished {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Read helper output in a goroutine so a broken
	// subprocess cannot block this test indefinitely.
	ready := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) ==
				"REGISTRY_LOCK_ACQUIRED" {
				ready <- nil
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ready <- err
			return
		}

		ready <- errors.New(
			"helper exited before acquiring lock",
		)
	}()

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf(
				"helper readiness: %v; stderr: %s",
				err,
				stderr.String(),
			)
		}

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for helper lock")
	}

	// The child process currently owns the journal.
	other, err := openPersistentCommandRegistry(path)

	if err == nil {
		_ = other.Close()
		t.Fatal("second process acquired active lock")
	}

	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf(
			"expected ErrCommandRegistryLocked, got %v",
			err,
		)
	}

	// Simulate a crash: Kill bypasses registry.Close().
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}

	if err := cmd.Wait(); err == nil {
		t.Fatal("expected abnormal helper termination")
	}
	processFinished = true

	// A cooperative .lock file intentionally survives
	// abnormal process termination.
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected stale lock file: %v", err)
	}

	other, err = openPersistentCommandRegistry(path)

	if err == nil {
		_ = other.Close()
		t.Fatal("stale lock was bypassed")
	}

	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf(
			"expected stale lock rejection, got %v",
			err,
		)
	}

	// Simulate deliberate operator recovery.
	// The child has already been killed AND waited for.
	// Never remove a lock while its owner may be alive.
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove stale test lock: %v", err)
	}

	recovered, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf("open after recovery: %v", err)
	}

	if err := recovered.Close(); err != nil {
		t.Fatalf("close recovered registry: %v", err)
	}
}
