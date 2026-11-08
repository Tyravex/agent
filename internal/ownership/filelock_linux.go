package ownership

import (
	"errors"
	"os"
	"syscall"
)

func LockFile(file *os.File) error {
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

func UnlockFile(file *os.File) error {
	return syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_UN,
	)
}
