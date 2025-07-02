package storage

import (
	"context"
	"io"
)

// Storage defines the interface for storage backends
type Storage interface {
	// Get retrieves a file from storage
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Put saves a file to storage
	Put(ctx context.Context, key string, data io.Reader) error
	// Exists checks if a file exists in storage
	Exists(ctx context.Context, key string) (bool, error)
}
