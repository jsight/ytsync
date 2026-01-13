//go:build windows

package storage

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// FileLock provides advisory file locking for cross-process synchronization.
// This uses LockFileEx on Windows.
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
		// Try to acquire exclusive lock without blocking
		err = lockFile(l.file)
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
	unlockFile(l.file)
	l.file.Close()
	os.Remove(l.path)
	l.file = nil
	return nil
}

// lockFile acquires an exclusive lock on the file using Windows API.
func lockFile(f *os.File) error {
	var overlapped windows.Overlapped
	// LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	return err
}

// unlockFile releases the lock on the file using Windows API.
func unlockFile(f *os.File) error {
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	return err
}
