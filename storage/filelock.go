package storage

import (
	"os"
	"syscall"
	"time"
)

// FileLock provides advisory file locking for cross-process synchronization.
// This uses flock(2) system call which is available on Unix-like systems.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a file lock. The lock is not acquired until Lock() is called.
// The lock file will be created at path + ".lock".
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path + ".lock"}
}

// Lock acquires an exclusive lock with the specified timeout.
// Returns ErrLockTimeout if the lock cannot be acquired within the timeout.
func (l *FileLock) Lock(timeout time.Duration) error {
	var err error
	l.file, err = os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return &StorageError{Op: "lock", Entity: "file", ID: l.path, Err: err}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err = syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	l.file.Close()
	l.file = nil
	return ErrLockTimeout
}

// Unlock releases the lock.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}
	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
	os.Remove(l.path)
	l.file = nil
	return nil
}
