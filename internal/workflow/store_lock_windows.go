//go:build windows

package workflow

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	lockFileFailImmediately = 0x00000001
	lockFileExclusiveLock   = 0x00000002
)

var (
	kernel32ProcUnlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
	kernel32ProcLockFileEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
)

func lockFile(file *os.File, nonblocking bool) error {
	flags := uint32(lockFileExclusiveLock)
	if nonblocking {
		flags |= lockFileFailImmediately
	}
	var overlapped syscall.Overlapped
	result, _, callErr := kernel32ProcLockFileEx.Call(
		file.Fd(),
		uintptr(flags),
		0,
		uintptr(^uint32(0)),
		uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result == 0 {
		return callErr
	}
	return nil
}

func unlockFile(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := kernel32ProcUnlockFileEx.Call(
		file.Fd(),
		0,
		uintptr(^uint32(0)),
		uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result == 0 {
		return callErr
	}
	return nil
}
