//go:build windows

package workspacelease

import (
	"errors"
	"os"
	"sync"
)

var (
	leaseMu    sync.Mutex
	leaseFiles = map[string]bool{}
)

func lockFile(file *os.File, _ bool) error {
	leaseMu.Lock()
	defer leaseMu.Unlock()
	name := file.Name()
	if leaseFiles[name] {
		return errors.New("workspace lease is held")
	}
	leaseFiles[name] = true
	return nil
}

func unlockFile(file *os.File) error {
	leaseMu.Lock()
	delete(leaseFiles, file.Name())
	leaseMu.Unlock()
	return nil
}
