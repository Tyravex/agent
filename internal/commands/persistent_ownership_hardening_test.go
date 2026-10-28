package commands

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPersistentOwnershipHardeningWriteFailurePoisonsStore(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)
	defer store.Close()

	if err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	store.writeRecord = func(data []byte) (int, error) {
		return 0, io.ErrClosedPipe
	}

	err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(2, "service-2"),
	)

	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected write error, got %v", err)
	}

	_, err = store.Lookup(ctx, "service-1")

	if !errors.Is(err, ErrOwnershipStorePoisoned) {
		t.Fatalf(
			"lookup after failed write: %v",
			err,
		)
	}

	err = store.WithAuthorizedService(
		ctx,
		"node-1",
		"service-1",
		func() error {
			t.Fatal("poisoned store executed callback")
			return nil
		},
	)

	if !errors.Is(err, ErrOwnershipStorePoisoned) {
		t.Fatalf(
			"lease after failed write: %v",
			err,
		)
	}

	err = store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(3, "service-3"),
	)

	if !errors.Is(err, ErrOwnershipStorePoisoned) {
		t.Fatalf(
			"snapshot after failed write: %v",
			err,
		)
	}
}

func TestPersistentOwnershipHardeningShortWritePoisonsStore(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)
	defer store.Close()

	store.writeRecord = func(data []byte) (int, error) {
		return len(data) / 2, nil
	}

	err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	)

	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf(
			"expected short write, got %v",
			err,
		)
	}

	_, err = store.Lookup(ctx, "service-1")

	if !errors.Is(err, ErrOwnershipStorePoisoned) {
		t.Fatalf(
			"expected poisoned store, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipHardeningSyncFailurePoisonsStore(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)
	defer store.Close()

	syncFailure := errors.New("injected sync failure")

	store.syncRecord = func() error {
		return syncFailure
	}

	err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	)

	if !errors.Is(err, syncFailure) {
		t.Fatalf(
			"expected sync failure, got %v",
			err,
		)
	}

	_, err = store.Lookup(ctx, "service-1")

	if !errors.Is(err, ErrOwnershipStorePoisoned) {
		t.Fatalf(
			"expected poisoned store, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipHardeningCloseWaitsForLease(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	if err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	leaseDone := make(chan error, 1)

	go func() {
		leaseDone <- store.WithAuthorizedService(
			ctx,
			"node-1",
			"service-1",
			func() error {
				close(started)
				<-release
				return nil
			},
		)
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("lease did not start")
	}

	closeDone := make(chan error, 1)

	go func() {
		closeDone <- store.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf(
			"Close finished during lease: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-leaseDone:
		if err != nil {
			t.Fatalf("lease: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("lease did not finish")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Close did not finish")
	}

	_, err := store.Lookup(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrOwnershipStoreClosed) {
		t.Fatalf(
			"expected closed store, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipHardeningConcurrentLifecycle(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	if err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	leaseDone := make(chan error, 1)

	go func() {
		leaseDone <- store.WithAuthorizedService(
			ctx,
			"node-1",
			"service-1",
			func() error {
				close(started)
				<-release
				return nil
			},
		)
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("lease did not start")
	}

	var wg sync.WaitGroup
	wg.Add(2)

	applyDone := make(chan error, 1)
	closeDone := make(chan error, 1)

	go func() {
		defer wg.Done()
		applyDone <- store.ApplySnapshot(
			ctx,
			testOwnershipSnapshot(2),
		)
	}()

	go func() {
		defer wg.Done()
		closeDone <- store.Close()
	}()

	// Neither operation may finish while the lease runs.
	select {
	case err := <-applyDone:
		t.Fatalf(
			"ApplySnapshot finished during lease: %v",
			err,
		)
	case err := <-closeDone:
		t.Fatalf(
			"Close finished during lease: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-leaseDone:
		if err != nil {
			t.Fatalf("lease: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("lease did not finish")
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("concurrent operations deadlocked")
	}

	applyErr := <-applyDone
	closeErr := <-closeDone

	// Depending on lock acquisition order, the snapshot
	// either commits before Close or sees a closed store.
	if applyErr != nil &&
		!errors.Is(applyErr, ErrOwnershipStoreClosed) {
		t.Fatalf(
			"unexpected ApplySnapshot error: %v",
			applyErr,
		)
	}

	if closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
}

func TestPersistentOwnershipHardeningCorruptPartialWrite(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	// Simulate an interrupted append on disk.
	if _, err := store.file.Write(
		[]byte(`{"node_id":"node-1","revision":`),
	); err != nil {
		t.Fatalf("partial write: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("corrupt log unexpectedly opened")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected corrupt log, got %v",
			err,
		)
	}

	// A failed open must release the system lock,
	// even though the lock file remains on disk.
	lockFile, err := os.OpenFile(
		path+".lock",
		os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := lockOwnershipFile(lockFile); err != nil {
		t.Fatalf("lock not released after failed open: %v", err)
	}

	if err := unlockOwnershipFile(lockFile); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}
