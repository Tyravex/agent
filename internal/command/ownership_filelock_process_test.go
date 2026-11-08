package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	ownershipProcessHelperEnv  = "TYRAVEX_OWNERSHIP_TEST_HELPER"
	ownershipProcessHelperPath = "TYRAVEX_OWNERSHIP_TEST_PATH"
	ownershipProcessReady      = "OWNERSHIP_STORE_READY"
)

func TestOwnershipFileLockProcessHelper(t *testing.T) {
	if os.Getenv(ownershipProcessHelperEnv) != "1" {
		return
	}

	path := os.Getenv(ownershipProcessHelperPath)
	if path == "" {
		t.Fatal("missing helper store path")
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("helper open: %v", err)
	}

	if err := store.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("helper snapshot: %v", err)
	}

	// Signal readiness only after the revision has been
	// written and synchronized.
	fmt.Fprintln(os.Stdout, ownershipProcessReady)

	// Do not call Close(). The parent process will
	// terminate this process forcibly.
	for {
		time.Sleep(time.Hour)
	}
}

func TestOwnershipFileLockProcessCrashRecovery(t *testing.T) {
	if os.Getenv(ownershipProcessHelperEnv) == "1" {
		t.Skip("parent test disabled in helper process")
	}

	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestOwnershipFileLockProcessHelper$",
		"-test.v",
	)

	cmd.Env = append(
		os.Environ(),
		ownershipProcessHelperEnv+"=1",
		ownershipProcessHelperPath+"="+path,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	// Always clean up the child, including when a test
	// assertion fails before the explicit Kill().
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	ready := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(stdout)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				ready <- err
				return
			}

			if strings.TrimSpace(line) ==
				ownershipProcessReady {
				ready <- nil
				return
			}
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf(
				"helper exited before readiness: %v",
				err,
			)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for helper readiness")
	}

	// The child holds an OS-level exclusive lock.
	second, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if second != nil {
		_ = second.Close()
		t.Fatal("second process opened locked store")
	}

	if !errors.Is(err, ErrOwnershipStoreLocked) {
		t.Fatalf(
			"expected lock denial, got %v",
			err,
		)
	}

	// Force termination without Close().
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}

	// A killed process normally exits with a nonzero
	// status. That is expected here.
	_ = cmd.Wait()

	// The lock must be released by the operating system.
	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf(
			"reopen after forced termination: %v",
			err,
		)
	}
	defer reopened.Close()

	revision, applied := reopened.SnapshotRevision()

	if !applied || revision != 10 {
		t.Fatalf(
			"revision not recovered: %d, applied=%v",
			revision,
			applied,
		)
	}

	// The revision survives, but service permissions do not.
	_, err = reopened.Lookup(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"service authorized after crash: %v",
			err,
		)
	}

	// Neither an older nor an identical revision
	// may restore permissions.
	for _, staleRevision := range []uint64{9, 10} {
		err := reopened.ApplySnapshot(
			context.Background(),
			testOwnershipSnapshot(
				staleRevision,
				"service-1",
			),
		)

		if !errors.Is(
			err,
			ErrStaleOwnershipRevision,
		) {
			t.Fatalf(
				"revision %d: expected stale rejection, got %v",
				staleRevision,
				err,
			)
		}
	}

	// A strictly newer snapshot restores access.
	if err := reopened.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(11, "service-1"),
	); err != nil {
		t.Fatalf(
			"apply newer snapshot: %v",
			err,
		)
	}

	if _, err := reopened.Lookup(
		context.Background(),
		"service-1",
	); err != nil {
		t.Fatalf(
			"service not restored by newer snapshot: %v",
			err,
		)
	}

	// Confirm that the child did not remove the lock
	// file and that it remains a stable lock target.
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf(
			"lock file missing after crash: %v",
			err,
		)
	}

	// Verify the journal is still readable while open.
	journal, err := os.Open(path)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer journal.Close()

	content, err := io.ReadAll(journal)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}

	if !strings.Contains(
		string(content),
		`"revision":10`,
	) {
		t.Fatal("persisted revision missing from journal")
	}
}
