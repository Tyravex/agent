package commands

import (
	"errors"
	"os"
	"syscall"
)

func lockOwnershipFile(file *os.File) error {
	err := syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_EX|syscall.LOCK_NB,
	)
	if err == nil {
		return nil
	}

	if errors.Is(err, syscall.EWOULDBLOCK) ||
		errors.Is(err, syscall.EAGAIN) {
		return ErrOwnershipStoreLocked
	}

	return err
}

func unlockOwnershipFile(file *os.File) error {
	return syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_UN,
	)
}
