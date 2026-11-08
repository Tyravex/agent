package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tyravex/agent/internal/ownership"
)

var (
	ErrOwnershipStoreLocked  = ownership.ErrOwnershipStoreLocked
	ErrOwnershipStoreCorrupt = ownership.ErrOwnershipStoreCorrupt
	ErrOwnershipStoreClosed  = errors.New(
		"persistent ownership store is closed",
	)
	ErrOwnershipStorePoisoned = errors.New(
		"persistent ownership store requires recovery",
	)
)

type ownershipRevisionRecord struct {
	NodeID   string `json:"node_id"`
	Revision uint64 `json:"revision"`
}

// PersistentOwnershipStore remembers revisions across
// restarts, but deliberately does not restore assignments.
//
// All operations are serialized through mu. An active
// execution lease holds mu until its callback returns.
//
// The store must not be called recursively from a lease
// callback: that would deadlock.
type PersistentOwnershipStore struct {
	mu sync.Mutex

	nodeID string
	memory *MemoryOwnershipStore

	file     *os.File
	lockFile *os.File
	lockPath string

	closed   bool
	poisoned bool

	// Test seams. Production uses os.File.Write and Sync.
	writeRecord func([]byte) (int, error)
	syncRecord  func() error
}

var _ ServiceOwnershipStore = (*PersistentOwnershipStore)(nil)
var _ ServiceLeaseStore = (*PersistentOwnershipStore)(nil)

func OpenPersistentOwnershipStore(
	nodeID string,
	path string,
) (_ *PersistentOwnershipStore, err error) {
	if err := validateOwnership(ServiceOwnership{
		ServiceID: "validation",
		NodeID:    nodeID,
	}); err != nil {
		return nil, err
	}

	if path == "" {
		return nil, fmt.Errorf(
			"%w: empty persistence path",
			ErrInvalidOwnership,
		)
	}

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return nil, err
	}

	lockPath := path + ".lock"

	// The lock file may already exist after a crash.
	// Its existence is not evidence of an active lock.
	lockFile, err := os.OpenFile(
		lockPath,
		os.O_RDWR|os.O_CREATE,
		0600,
	)
	if err != nil {
		return nil, err
	}

	if err := lockOwnershipFile(lockFile); err != nil {
		_ = lockFile.Close()
		return nil, err
	}

	success := false

	defer func() {
		if !success {
			_ = unlockOwnershipFile(lockFile)
			_ = lockFile.Close()
		}
	}()

	if err := ensureOwnershipJournalInitialized(
		path,
		nodeID,
	); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE,
		0600,
	)
	if err != nil {
		return nil, err
	}

	defer func() {
		if !success {
			_ = file.Close()
		}
	}()

	revision, applied, err := ownership.ReadRevisionLog(
		file,
		nodeID,
	)
	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}

	memory := NewMemoryOwnershipStore()

	// Restore only the revision, never assignments.
	memory.RestoreRevision(revision, applied)

	store := &PersistentOwnershipStore{
		nodeID:      nodeID,
		memory:      memory,
		file:        file,
		lockFile:    lockFile,
		lockPath:    lockPath,
		writeRecord: file.Write,
		syncRecord:  file.Sync,
	}

	success = true
	return store, nil
}

func (s *PersistentOwnershipStore) ApplySnapshot(
	ctx context.Context,
	snapshot OwnershipSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrAuthorizationUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrOwnershipStoreClosed
	}

	if s.poisoned {
		return ErrOwnershipStorePoisoned
	}

	replacement, err := validateOwnershipSnapshot(
		snapshot,
		s.nodeID,
	)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	revision, applied := s.memory.SnapshotRevision()

	if applied && snapshot.Revision <= revision {
		return ErrStaleOwnershipRevision
	}

	record, err := json.Marshal(ownershipRevisionRecord{
		NodeID:   s.nodeID,
		Revision: snapshot.Revision,
	})
	if err != nil {
		return err
	}

	record = append(record, '\n')

	// A failed write or sync makes disk state uncertain.
	// Poison the store before returning the error.
	n, err := s.writeRecord(record)

	if err != nil || n != len(record) {
		s.poisoned = true

		if err == nil {
			err = io.ErrShortWrite
		}

		return fmt.Errorf(
			"persist ownership revision: %w",
			err,
		)
	}

	if err := s.syncRecord(); err != nil {
		s.poisoned = true

		return fmt.Errorf(
			"sync ownership revision: %w",
			err,
		)
	}

	// Publish only after a successful durable write.
	// No lease can run while s.mu is held.
	s.memory.PublishPersistedSnapshot(
		snapshot.Revision,
		replacement,
	)

	return nil
}

func (s *PersistentOwnershipStore) Lookup(
	ctx context.Context,
	serviceID string,
) (ServiceOwnership, error) {
	if err := ctx.Err(); err != nil {
		return ServiceOwnership{}, err
	}

	if s == nil {
		return ServiceOwnership{}, ErrAuthorizationUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ServiceOwnership{}, ErrOwnershipStoreClosed
	}

	if s.poisoned {
		return ServiceOwnership{}, ErrOwnershipStorePoisoned
	}

	return s.memory.Lookup(ctx, serviceID)
}

func (s *PersistentOwnershipStore) WithAuthorizedService(
	ctx context.Context,
	nodeID string,
	serviceID string,
	run func() error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrLeaseUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrOwnershipStoreClosed
	}

	if s.poisoned {
		return ErrOwnershipStorePoisoned
	}

	if nodeID != s.nodeID {
		return ErrServiceNotAuthorized
	}

	// The outer lock remains held throughout run().
	// This prevents Close, ApplySnapshot, and another
	// persistent-store lease from overlapping.
	return s.memory.WithAuthorizedService(
		ctx,
		nodeID,
		serviceID,
		run,
	)
}

func (s *PersistentOwnershipStore) SnapshotRevision() (
	uint64,
	bool,
) {
	if s == nil {
		return 0, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.memory.SnapshotRevision()
}

func (s *PersistentOwnershipStore) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	// Active leases finish before Close acquires mu.
	s.closed = true

	fileErr := s.file.Close()
	unlockErr := unlockOwnershipFile(s.lockFile)
	lockCloseErr := s.lockFile.Close()

	// Do not remove the lock file.
	// Its inode/file identity must remain stable.
	return errors.Join(
		fileErr,
		unlockErr,
		lockCloseErr,
	)
}
