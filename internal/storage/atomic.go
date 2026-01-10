package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriter provides atomic file write operations using temp file + rename.
// This ensures that the target file is never left in a partially-written state.
type AtomicWriter struct {
	path    string
	tmpPath string
	file    *os.File
}

// NewAtomicWriter creates a writer for atomic file updates.
// The writer creates a temporary file in the same directory as the target,
// and on Commit(), atomically renames it to replace the target.
func NewAtomicWriter(path string) (*AtomicWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".ytsync-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	return &AtomicWriter{
		path:    path,
		tmpPath: tmpFile.Name(),
		file:    tmpFile,
	}, nil
}

// Write writes data to the temporary file.
func (w *AtomicWriter) Write(p []byte) (n int, err error) {
	return w.file.Write(p)
}

// Commit atomically replaces the target file with the temporary file.
// This syncs the file to disk before renaming to ensure durability.
func (w *AtomicWriter) Commit() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(w.tmpPath, w.path); err != nil {
		os.Remove(w.tmpPath) // Best effort cleanup
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Abort discards the temporary file without committing.
func (w *AtomicWriter) Abort() error {
	w.file.Close()
	return os.Remove(w.tmpPath)
}
