//go:build !windows

package ownerproc

import (
	"errors"
	"syscall"
)

// Alive reports whether pid names a live process. Signal 0 performs
// only the existence and permission check; EPERM still proves existence.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
