//go:build !windows

package workflow

import (
	"os"
	"syscall"
)

func lockFile(file *os.File, nonblocking bool) error {
	operation := syscall.LOCK_EX
	if nonblocking {
		operation |= syscall.LOCK_NB
	}
	return syscall.Flock(int(file.Fd()), operation)
}

func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
