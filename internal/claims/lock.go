package claims

import (
	"fmt"
	"time"
)

// LockFile acquires an exclusive advisory lock on path+".lock".
// Returns an unlock function. Retries with exponential backoff up to timeout.
// The implementation is platform-specific: lock_unix.go and lock_windows.go
// provide the actual tryLock and doUnlock primitives used here.
func LockFile(path string, timeout time.Duration) (unlock func(), err error) {
	lockPath := path + ".lock"
	deadline := time.Now().Add(timeout)
	backoff := 10 * time.Millisecond

	for {
		fd, unlockFn, tryErr := tryLock(lockPath)
		if tryErr == nil {
			_ = fd // fd ownership transferred to unlockFn closure on unix
			return unlockFn, nil
		}

		if !isWouldBlock(tryErr) {
			return nil, fmt.Errorf("claims: lock %s: %w", lockPath, tryErr)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("claims: timed out acquiring lock %s after %s", lockPath, timeout)
		}

		time.Sleep(backoff)
		if backoff < 500*time.Millisecond {
			backoff *= 2
		}
	}
}
