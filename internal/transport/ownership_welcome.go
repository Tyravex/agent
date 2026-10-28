package transport

import (
	"errors"
	"strings"
)

var ErrOwnershipWelcomeSessionRequired = errors.New(
	"ownership requires a valid sessionId in node.welcome",
)

// validOwnershipSessionID validates the format of a
// Panel-issued connection identifier.
//
// The identifier is not a credential. The Panel must
// generate it independently for each authenticated
// WebSocket connection.
func validOwnershipSessionID(value string) bool {
	if len(value) < 16 || len(value) > 128 {
		return false
	}

	if strings.TrimSpace(value) != value {
		return false
	}

	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '-', ch == '_':
		default:
			return false
		}
	}

	return true
}
