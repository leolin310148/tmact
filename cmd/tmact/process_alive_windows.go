//go:build windows

package main

import "syscall"

const processQueryLimitedInformation = 0x1000

var (
	kernel32ProcOpenProcess = syscall.NewLazyDLL("kernel32.dll").NewProc("OpenProcess")
	kernel32ProcCloseHandle = syscall.NewLazyDLL("kernel32.dll").NewProc("CloseHandle")
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, _, _ := kernel32ProcOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if handle == 0 {
		return false
	}
	_, _, _ = kernel32ProcCloseHandle.Call(handle)
	return true
}
