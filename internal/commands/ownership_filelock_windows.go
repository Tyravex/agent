package commands

import (
	"os"

	"github.com/Tyravex/agent/internal/ownership"
)

func lockOwnershipFile(file *os.File) error {
	return ownership.LockFile(file)
}

func unlockOwnershipFile(file *os.File) error {
	return ownership.UnlockFile(file)
}
