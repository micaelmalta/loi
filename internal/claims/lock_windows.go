//go:build windows

package claims

import (
	"errors"
	"os"
)

// errWouldBlock is a sentinel returned by tryLock on Windows when the lock
// file already exists (i.e., another process holds the lock).
var errWouldBlock = errors.New("lock file already exists")

// tryLock attempts to create lockPath with O_EXCL (exclusive create).
// On Windows, O_EXCL creation is the portable advisory-lock primitive:
// only one goroutine/process succeeds; others get os.ErrExist.
// Returns fd=-1 (unused on Windows), an unlock closure, and any error.
func tryLock(lockPath string) (fd int, unlock func(), err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return -1, nil, errWouldBlock
		}
		return -1, nil, err
	}
	f.Close()

	unlockFn := func() {
		os.Remove(lockPath) //nolint:errcheck
	}
	return -1, unlockFn, nil
}

// isWouldBlock reports whether err signals that the lock is already held.
func isWouldBlock(err error) bool {
	return err == errWouldBlock
}
