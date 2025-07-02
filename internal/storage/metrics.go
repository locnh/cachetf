package storage

import (
	"context"
	"io"
)

// metricsWrapper wraps a Storage implementation with metrics
type metricsWrapper struct {
	s Storage
}

// NewMetricsWrapper creates a new metrics wrapper around a Storage implementation
// Note: The underlying storage implementation is responsible for recording metrics
// This wrapper only exists to maintain backward compatibility with the Storage interface
func NewMetricsWrapper(s Storage) Storage {
	return &metricsWrapper{
		s: s,
	}
}

func (m *metricsWrapper) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Just pass through to the underlying storage, which handles metrics
	return m.s.Get(ctx, key)
}

func (m *metricsWrapper) Put(ctx context.Context, key string, r io.Reader) error {
	// Just pass through to the underlying storage, which handles metrics
	return m.s.Put(ctx, key, r)
}

func (m *metricsWrapper) Exists(ctx context.Context, key string) (bool, error) {
	// Just pass through to the underlying storage, which handles metrics
	return m.s.Exists(ctx, key)
}

func (m *metricsWrapper) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	// Just pass through to the underlying storage, which handles metrics
	return m.s.DeleteByPrefix(ctx, prefix)
}
