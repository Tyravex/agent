package transport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrCommandRegistryCorrupt = errors.New(
		"command registry is corrupted",
	)

	ErrCommandRegistryLocked = errors.New(
		"command registry is already locked",
	)

	ErrCommandRegistryClosed = errors.New(
		"command registry is closed",
	)
)

// persistentCommandRegistry stores command IDs in an
// append-only JSONL journal.
//
// A successful reservation is synchronized to disk before
// the caller may execute the corresponding command.
//
// The lock file prevents cooperating Agent processes from
// opening the same journal concurrently.
//
// The lock is intentionally not removed automatically
// after a crash. Recovery requires operator intervention.
type persistentCommandRegistry struct {
	mu sync.Mutex

	file     *os.File
	lockFile *os.File
	lockPath string

	seen map[string]struct{}

	// Once persistence becomes uncertain, no further
	// reservations are allowed.
	failed bool
}

type commandRegistryRecord struct {
	CommandID string `json:"commandId"`
}

func openPersistentCommandRegistry(
	path string,
) (*persistentCommandRegistry, error) {
	if path == "" {
		return nil, errors.New(
			"command registry path is empty",
		)
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve command registry path: %w",
			err,
		)
	}

	if _, err := os.Stat(
		filepath.Dir(absolutePath),
	); err != nil {
		return nil, fmt.Errorf(
			"command registry directory: %w",
			err,
		)
	}

	lockPath := absolutePath + ".lock"

	lockFile, err := os.OpenFile(
		lockPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf(
				"%w: %s",
				ErrCommandRegistryLocked,
				lockPath,
			)
		}

		return nil, fmt.Errorf(
			"create command registry lock: %w",
			err,
		)
	}

	registry := &persistentCommandRegistry{
		lockFile: lockFile,
		lockPath: lockPath,
		seen:     make(map[string]struct{}),
	}

	// Release the lock if opening or validating the
	// journal fails.
	cleanup := func() {
		_ = registry.releaseLock()
	}

	file, err := os.OpenFile(
		absolutePath,
		os.O_RDWR|os.O_CREATE,
		0600,
	)
	if err != nil {
		cleanup()

		return nil, fmt.Errorf(
			"open command registry: %w",
			err,
		)
	}

	registry.file = file

	if err := registry.load(); err != nil {
		_ = registry.file.Close()
		registry.file = nil
		cleanup()

		return nil, err
	}

	if _, err := registry.file.Seek(
		0,
		io.SeekEnd,
	); err != nil {
		_ = registry.file.Close()
		registry.file = nil
		cleanup()

		return nil, fmt.Errorf(
			"seek command registry: %w",
			err,
		)
	}

	return registry, nil
}

// load requires every record to end with a newline.
//
// An unterminated final record is rejected rather than
// silently interpreted as a completed reservation.
//
// This is intentionally fail-closed: automatic truncation
// could erase evidence of an uncertain write.
func (r *persistentCommandRegistry) load() error {
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	const maxRecordBytes = 64 * 1024

	// ReadSlice never grows its buffer.
	// A record exceeding the buffer is rejected before
	// unbounded memory allocation can occur.
	reader := bufio.NewReaderSize(
		r.file,
		maxRecordBytes+1,
	)

	lineNumber := 0

	for {
		line, err := reader.ReadSlice('\n')

		if errors.Is(err, io.EOF) {
			if len(line) != 0 {
				return fmt.Errorf(
					"%w: unterminated record at line %d",
					ErrCommandRegistryCorrupt,
					lineNumber+1,
				)
			}

			break
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			return fmt.Errorf(
				"%w: oversized record at line %d",
				ErrCommandRegistryCorrupt,
				lineNumber+1,
			)
		}

		if err != nil {
			return fmt.Errorf(
				"read command registry: %w",
				err,
			)
		}

		lineNumber++

		if len(line) > maxRecordBytes {
			return fmt.Errorf(
				"%w: oversized record at line %d",
				ErrCommandRegistryCorrupt,
				lineNumber,
			)
		}

		var record commandRegistryRecord

		if err := decodeStrict(line, &record); err != nil {
			return fmt.Errorf(
				"%w: line %d: %v",
				ErrCommandRegistryCorrupt,
				lineNumber,
				err,
			)
		}

		if record.CommandID == "" ||
			len(record.CommandID) > 128 {
			return fmt.Errorf(
				"%w: invalid command ID at line %d",
				ErrCommandRegistryCorrupt,
				lineNumber,
			)
		}

		// The writer produces exactly one canonical JSON
		// object per line. Requiring the same representation
		// also rejects duplicate JSON keys, extra whitespace,
		// reordered fields and alternate escaping.
		canonical, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf(
				"%w: encode line %d: %v",
				ErrCommandRegistryCorrupt,
				lineNumber,
				err,
			)
		}

		canonical = append(canonical, '\n')

		if !bytes.Equal(line, canonical) {
			return fmt.Errorf(
				"%w: noncanonical record at line %d",
				ErrCommandRegistryCorrupt,
				lineNumber,
			)
		}

		if _, exists := r.seen[record.CommandID]; exists {
			return fmt.Errorf(
				"%w: duplicate command ID at line %d",
				ErrCommandRegistryCorrupt,
				lineNumber,
			)
		}

		if len(r.seen) >= maxRememberedCommands {
			return ErrCommandHistoryFull
		}

		r.seen[record.CommandID] = struct{}{}
	}

	return nil
}

// reserve persists an ID before returning success.
//
// On any write or sync failure, the registry becomes
// permanently unavailable for new reservations.
func (r *persistentCommandRegistry) reserve(
	id string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file == nil || r.failed {
		return ErrCommandRegistryClosed
	}

	if id == "" || len(id) > 128 {
		return errors.New(
			"invalid command ID",
		)
	}

	if _, exists := r.seen[id]; exists {
		return ErrDuplicateCommandID
	}

	if len(r.seen) >= maxRememberedCommands {
		return ErrCommandHistoryFull
	}

	data, err := json.Marshal(
		commandRegistryRecord{
			CommandID: id,
		},
	)
	if err != nil {
		return err
	}

	data = append(data, '\n')

	n, writeErr := r.file.Write(data)

	if writeErr != nil || n != len(data) {
		r.failed = true

		if writeErr == nil {
			writeErr = io.ErrShortWrite
		}

		return fmt.Errorf(
			"write command registry: %w",
			writeErr,
		)
	}

	if err := r.file.Sync(); err != nil {
		r.failed = true

		return fmt.Errorf(
			"sync command registry: %w",
			err,
		)
	}

	r.seen[id] = struct{}{}

	return nil
}

// Close releases the journal and its cooperative lock.
//
// A failed journal close does not make the journal
// safe for another process to use.
func (r *persistentCommandRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file == nil && r.lockFile == nil {
		return nil
	}

	if r.file != nil {
		if err := r.file.Close(); err != nil {
			r.failed = true

			return fmt.Errorf(
				"close command registry: %w",
				err,
			)
		}

		r.file = nil
	}

	return r.releaseLock()
}

// releaseLock is called only while the registry owns
// the lock, either during initialization or under mu.
func (r *persistentCommandRegistry) releaseLock() error {
	if r.lockFile == nil {
		return nil
	}

	if err := r.lockFile.Close(); err != nil {
		return fmt.Errorf(
			"close command registry lock: %w",
			err,
		)
	}

	r.lockFile = nil

	if err := os.Remove(r.lockPath); err != nil {
		return fmt.Errorf(
			"remove command registry lock: %w",
			err,
		)
	}

	return nil
}
