package ytsync

import (
	"errors"
	"ytsync/retry"
	"ytsync/storage"
	"ytsync/youtube"
)

// Error handling types exported for library users.
//
// All error types support the standard error handling patterns:
//
// Using errors.Is() for sentinel errors:
//
//	if errors.Is(err, youtube.ErrChannelNotFound) {
//		fmt.Println("Channel not found")
//	}
//
// Using errors.As() for wrapped errors:
//
//	var listerErr *youtube.ListerError
//	if errors.As(err, &listerErr) {
//		fmt.Printf("Listing failed for %s: %v\n", listerErr.Channel, listerErr.Err)
//	}

// Exported error types from sub-packages:
//
// From youtube package:
//   - youtube.ErrChannelNotFound: Channel does not exist
//   - youtube.ErrRateLimited: Rate limit exceeded
//   - youtube.ErrNetworkTimeout: Network timeout occurred
//   - youtube.ErrInvalidURL: Invalid YouTube URL
//   - youtube.ErrYtdlpNotInstalled: yt-dlp binary not found
//   - youtube.VideoLister: Interface for video listing
//   - youtube.ListerError: Error during video listing
//   - youtube.TranscriptError: Error during transcript extraction
//
// From retry package:
//   - retry.ErrChannelNotFound: Channel not found (permanent error)
//   - retry.ErrInvalidURL: Invalid URL (permanent error)
//   - retry.RetryableError: Error after max retries exceeded
//
// From storage package:
//   - storage.ErrNotFound: Entity not found in storage
//   - storage.ErrAlreadyExists: Entity already exists
//   - storage.ErrInvalidInput: Invalid input provided
//   - storage.ErrStorageCorrupt: Data corruption detected
//   - storage.ErrLockTimeout: File lock timeout
//   - storage.StorageError: General storage operation error

// Type aliases for convenient error handling.
type (
	// ListerError wraps errors during video listing.
	ListerError = youtube.ListerError
	// TranscriptError wraps errors during transcript extraction.
	TranscriptError = youtube.TranscriptError
	// RetryableError wraps errors that occurred after retries were exhausted.
	RetryableError = retry.RetryableError
	// StorageError wraps errors during storage operations.
	StorageError = storage.StorageError
)

// Sentinel errors exported from sub-packages.
var (
	// ErrChannelNotFound indicates the YouTube channel does not exist.
	ErrChannelNotFound = youtube.ErrChannelNotFound
	// ErrRateLimited indicates the operation was rate limited.
	ErrRateLimited = youtube.ErrRateLimited
	// ErrNetworkTimeout indicates a network timeout occurred.
	ErrNetworkTimeout = youtube.ErrNetworkTimeout
	// ErrInvalidURL indicates the provided URL is invalid.
	ErrInvalidURL = youtube.ErrInvalidURL
	// ErrYtdlpNotInstalled indicates yt-dlp binary was not found.
	ErrYtdlpNotInstalled = youtube.ErrYtdlpNotInstalled

	// Storage errors
	// ErrNotFound indicates an entity was not found in storage.
	ErrNotFound = storage.ErrNotFound
	// ErrAlreadyExists indicates an entity already exists in storage.
	ErrAlreadyExists = storage.ErrAlreadyExists
	// ErrInvalidInput indicates invalid input was provided.
	ErrInvalidInput = storage.ErrInvalidInput
	// ErrStorageCorrupt indicates data corruption was detected.
	ErrStorageCorrupt = storage.ErrStorageCorrupt
	// ErrLockTimeout indicates a timeout acquiring a file lock.
	ErrLockTimeout = storage.ErrLockTimeout
)

// IsRetryable determines if an error should be retried.
// It returns false for permanent errors like ErrChannelNotFound.
func IsRetryable(err error) bool {
	return retry.IsRetryable(err)
}
