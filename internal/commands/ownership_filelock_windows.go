package commands

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002

	windowsLockViolation syscall.Errno = 33
)

var (
	ownershipKernel32 = syscall.NewLazyDLL("kernel32.dll")

	ownershipLockFileEx = ownershipKernel32.NewProc(
		"LockFileEx",
	)

	ownershipUnlockFileEx = ownershipKernel32.NewProc(
		"UnlockFileEx",
	)
)

func lockOwnershipFile(file *os.File) error {
	var overlapped syscall.Overlapped

	result, _, callErr := ownershipLockFileEx.Call(
		file.Fd(),
		uintptr(
			lockfileExclusiveLock|
				lockfileFailImmediately,
		),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if result != 0 {
		return nil
	}

	if errors.Is(callErr, windowsLockViolation) {
		return ErrOwnershipStoreLocked
	}

	if callErr != nil &&
		!errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}

	return syscall.EINVAL
}

func unlockOwnershipFile(file *os.File) error {
	var overlapped syscall.Overlapped

	result, _, callErr := ownershipUnlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if result != 0 {
		return nil
	}

	if callErr != nil &&
		!errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}

	return syscall.EINVAL
}
