package storage

import (
	"context"
	"io"
)

// Storage defines the interface for storage backends
type Storage interface {
	// Get retrieves a file by key
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Put stores a file with the given key
	Put(ctx context.Context, key string, r io.Reader) error
	// Exists checks if a file exists
	Exists(ctx context.Context, key string) (bool, error)
	// DeleteByPrefix deletes all items with the given prefix
	DeleteByPrefix(ctx context.Context, prefix string) (int, error)
}
