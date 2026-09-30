//go:build windows

package dockerlab

import (
	"errors"
	"syscall"
)

const (
	processQueryLimitedInformation = 0x1000
	processStillActive             = 259
)

// processAlive reports whether pid names a live process. Uncertain results
// report alive so orphan cleanup stays conservative.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}
	return exitCode == processStillActive
}
