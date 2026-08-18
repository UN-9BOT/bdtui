package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// LockPath returns the singleton lock file path for a daemon bound to
// socketPath.
func LockPath(socketPath string) string {
	return socketPath + ".lock"
}

// AcquireLock takes an exclusive, non-blocking flock on path, creating the
// file if needed. The returned file must stay open for the daemon's lifetime;
// closing it (or process exit) releases the lock. A second live daemon fails
// to acquire the lock and gets an error.
func AcquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("daemon already running (lock %s): %w", path, err)
	}
	return f, nil
}

// ReleaseLock releases the flock and closes the lock file.
func ReleaseLock(f *os.File) error {
	if f == nil {
		return nil
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return f.Close()
}
