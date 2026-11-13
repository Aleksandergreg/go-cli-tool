//go:build !windows

package dockerlab

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid names a live process. Signal 0 performs
// only the existence and permission check; EPERM still proves existence.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
