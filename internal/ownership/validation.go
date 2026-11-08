package ownership

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var authorizedServiceIDPattern = regexp.MustCompile(
	`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`,
)

var ErrInvalidOwnership = errors.New(
	"invalid service ownership",
)

// ValidServiceID reports whether an ID is safe to use
// as a service identifier.
func ValidServiceID(id string) bool {
	return authorizedServiceIDPattern.MatchString(id)
}

// ValidateAssignment validates a trusted service assignment.
//
// It does not check whether the assignment is authorized
// for a particular node or whether it is enabled.
func ValidateAssignment(assignment ServiceOwnership) error {
	if !ValidServiceID(assignment.ServiceID) {
		return fmt.Errorf(
			"%w: invalid service ID",
			ErrInvalidOwnership,
		)
	}

	if assignment.NodeID == "" ||
		strings.TrimSpace(assignment.NodeID) != assignment.NodeID ||
		len(assignment.NodeID) > 128 ||
		strings.ContainsAny(assignment.NodeID, "\r\n\t") {
		return fmt.Errorf(
			"%w: invalid node ID",
			ErrInvalidOwnership,
		)
	}

	return nil
}
