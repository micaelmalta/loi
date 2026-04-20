//go:build !windows

package claims

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryLock attempts a non-blocking exclusive flock on lockPath.
// Returns the open file descriptor, an unlock closure, and any error.
// On success, the caller owns the fd; the closure closes it on unlock.
// On EWOULDBLOCK the fd is closed before returning and the error is returned.
func tryLock(lockPath string) (fd int, unlock func(), err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return -1, nil, err
	}

	flockErr := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if flockErr != nil {
		f.Close()
		return -1, nil, flockErr
	}

	unlockFn := func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:errcheck
		f.Close()
	}
	return int(f.Fd()), unlockFn, nil
}

// isWouldBlock reports whether err is EWOULDBLOCK or EAGAIN,
// which flock returns when the lock is already held.
func isWouldBlock(err error) bool {
	return err == unix.EWOULDBLOCK || err == unix.EAGAIN
}
