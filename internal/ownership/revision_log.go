package ownership

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxRevisionRecordBytes = 4096

type ownershipRevisionRecord struct {
	NodeID   string `json:"node_id"`
	Revision uint64 `json:"revision"`
}

// ReadRevisionLog validates the complete ownership revision
// journal and returns the latest applied revision.
//
// A missing final newline, malformed JSON, incorrect node ID,
// zero revision, or non-increasing revision is rejected.
func ReadRevisionLog(
	file *os.File,
	expectedNodeID string,
) (uint64, bool, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, false, err
	}

	reader := bufio.NewReader(file)

	var revision uint64
	var applied bool

	for {
		line, err := reader.ReadBytes('\n')

		if len(line) == 0 && errors.Is(err, io.EOF) {
			break
		}

		if len(line) > maxRevisionRecordBytes ||
			len(line) == 0 ||
			line[len(line)-1] != '\n' {
			return 0, false, ErrOwnershipStoreCorrupt
		}

		var record ownershipRevisionRecord

		if jsonErr := json.Unmarshal(
			line,
			&record,
		); jsonErr != nil {
			return 0, false, fmt.Errorf(
				"%w: invalid JSON",
				ErrOwnershipStoreCorrupt,
			)
		}

		if record.NodeID != expectedNodeID ||
			record.Revision == 0 ||
			(applied && record.Revision <= revision) {
			return 0, false, ErrOwnershipStoreCorrupt
		}

		revision = record.Revision
		applied = true

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, false, err
		}
	}

	return revision, applied, nil
}
