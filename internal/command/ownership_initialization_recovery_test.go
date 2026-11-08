package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipInitializationRecovery(
	t *testing.T,
) {
	tests := []struct {
		name          string
		journal       []byte
		writeJournal  bool
		marker        []byte
		writeMarker   bool
		wantErr       error
		wantRevision  uint64
		wantApplied   bool
		wantNewMarker bool
	}{
		{
			name:          "interrupted before journal creation",
			wantNewMarker: true,
		},
		{
			name:          "interrupted after empty journal creation",
			writeJournal:  true,
			journal:       nil,
			wantNewMarker: true,
		},
		{
			name:         "interrupted during marker write",
			writeJournal: true,
			writeMarker:  true,
			marker:       []byte("tyravex-owner"),
			wantErr:      ErrOwnershipInitializationCorrupt,
		},
		{
			name:          "completed marker with empty journal",
			writeJournal:  true,
			writeMarker:   true,
			marker:        []byte(ownershipInitializationMarker),
			wantNewMarker: true,
		},
		{
			name:        "completed marker but missing journal",
			writeMarker: true,
			marker:      []byte(ownershipInitializationMarker),
			wantErr:     ErrOwnershipJournalMissing,
		},
		{
			name:         "interrupted JSON record",
			writeJournal: true,
			journal: []byte(
				`{"node_id":"node-1","revision":10}`,
			),
			wantErr: ErrOwnershipStoreCorrupt,
		},
		{
			name:         "valid journal with interrupted marker",
			writeJournal: true,
			journal: []byte(
				"{\"node_id\":\"node-1\",\"revision\":10}\n",
			),
			writeMarker: true,
			marker:      nil,
			wantErr:     ErrOwnershipInitializationCorrupt,
		},
		{
			name:         "valid legacy journal",
			writeJournal: true,
			journal: []byte(
				"{\"node_id\":\"node-1\",\"revision\":10}\n",
			),
			wantRevision:  10,
			wantApplied:   true,
			wantNewMarker: true,
		},
		{
			name:         "completed journal and marker",
			writeJournal: true,
			journal: []byte(
				"{\"node_id\":\"node-1\",\"revision\":10}\n",
			),
			writeMarker: true,
			marker: []byte(
				ownershipInitializationMarker,
			),
			wantRevision: 10,
			wantApplied:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(
				t.TempDir(),
				"revisions.jsonl",
			)

			if tt.writeJournal {
				if err := os.WriteFile(
					path,
					tt.journal,
					0600,
				); err != nil {
					t.Fatalf(
						"write journal: %v",
						err,
					)
				}
			}

			if tt.writeMarker {
				if err := os.WriteFile(
					path+".initialized",
					tt.marker,
					0600,
				); err != nil {
					t.Fatalf(
						"write marker: %v",
						err,
					)
				}
			}

			store, err := OpenPersistentOwnershipStore(
				"node-1",
				path,
			)

			if tt.wantErr != nil {
				if store != nil {
					_ = store.Close()
					t.Fatal(
						"unexpectedly opened invalid state",
					)
				}

				if !errors.Is(err, tt.wantErr) {
					t.Fatalf(
						"expected %v, got %v",
						tt.wantErr,
						err,
					)
				}

				return
			}

			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer store.Close()

			revision, applied := store.SnapshotRevision()

			if revision != tt.wantRevision ||
				applied != tt.wantApplied {
				t.Fatalf(
					"revision=%d applied=%v; want %d %v",
					revision,
					applied,
					tt.wantRevision,
					tt.wantApplied,
				)
			}

			if tt.wantNewMarker {
				marker, err := os.ReadFile(
					path + ".initialized",
				)
				if err != nil {
					t.Fatalf(
						"read marker: %v",
						err,
					)
				}

				if string(marker) !=
					ownershipInitializationMarker {
					t.Fatalf(
						"invalid marker: %q",
						marker,
					)
				}
			}

			// Restarted stores must never restore
			// service assignments from the journal.
			_, err = store.Lookup(
				context.Background(),
				"service-1",
			)

			if !errors.Is(err, ErrOwnershipNotFound) {
				t.Fatalf(
					"unexpected service authorization: %v",
					err,
				)
			}
		})
	}
}

func TestOwnershipInitializationRecoveryAfterPartialRevision(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := store.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Simulate an interrupted append after a valid
	// committed revision.
	file, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_APPEND,
		0600,
	)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}

	if _, err := file.WriteString(
		`{"node_id":"node-1","revision":11`,
	); err != nil {
		_ = file.Close()
		t.Fatalf("write partial revision: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}

	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("partial revision unexpectedly accepted")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected corrupt journal, got %v",
			err,
		)
	}
}

func TestOwnershipInitializationRecoveryDoesNotResetRevision(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := store.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("apply revision 10: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	revision, applied := reopened.SnapshotRevision()

	if !applied || revision != 10 {
		t.Fatalf(
			"revision lost: %d applied=%v",
			revision,
			applied,
		)
	}

	// An identical revision cannot restore access.
	err = reopened.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(10, "service-1"),
	)

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf(
			"expected stale revision rejection, got %v",
			err,
		)
	}

	_, err = reopened.Lookup(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"service unexpectedly authorized: %v",
			err,
		)
	}
}
