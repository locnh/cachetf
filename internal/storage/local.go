// internal/storage/local.go
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	"cachetf/internal/metrics"
)

// LocalStorage implements Storage interface using local filesystem
type LocalStorage struct {
	baseDir string
	logger  *logrus.Logger
	// mutexes provides per-key locking to prevent concurrent writes to the same file
	mutexes sync.Map
	metrics *metrics.CacheMetrics
}

// getMutex returns a mutex for the given key, creating it if it doesn't exist
// This ensures that concurrent operations on the same key are serialized
func (s *LocalStorage) getMutex(key string) *sync.Mutex {
	mutex, _ := s.mutexes.LoadOrStore(key, &sync.Mutex{})
	return mutex.(*sync.Mutex)
}

// NewLocalStorage creates a new LocalStorage instance
func NewLocalStorage(baseDir string, logger *logrus.Logger) *LocalStorage {
	return &LocalStorage{
		baseDir: baseDir,
		logger:  logger,
		metrics: metrics.NewCacheMetrics(),
	}
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.baseDir, key)

	// Check if file exists first
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return nil, err
	}

	if !exists {
		s.metrics.RecordMiss()
		s.logger.WithFields(logrus.Fields{
			"key":  key,
			"path": path,
		}).Debug("Cache miss: file not found")
		return nil, os.ErrNotExist
	}

	// Open the file
	file, err := os.Open(path)
	if err != nil {
		// Don't record another miss here, as the Exists check already recorded it
		s.logger.WithError(err).WithField("path", path).Error("Failed to open file")
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"key":  key,
		"path": path,
	}).Debug("Cache hit: file found")

	// Record the hit and update file size in metrics
	s.metrics.RecordHit()
	if info, err := os.Stat(path); err == nil {
		s.metrics.UpdateSize(int64(info.Size()))
	}

	return file, nil
}

func (s *LocalStorage) Put(ctx context.Context, key string, r io.Reader) error {
	// Get a mutex for this specific key to prevent concurrent writes
	mutex := s.getMutex(key)
	mutex.Lock()
	defer mutex.Unlock()

	path := filepath.Join(s.baseDir, key)

	// Check if file already exists (quick check before creating directories)
	if _, err := os.Stat(path); err == nil {
		// File already exists, no need to write it again
		s.logger.WithField("path", path).Debug("File already exists, skipping write")
		return nil
	}

	// Check if file exists to update metrics
	if info, err := os.Stat(path); err == nil {
		// File exists, subtract its size from metrics
		s.metrics.UpdateSize(-info.Size())
	}

	// Create all directories in the path if they don't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create or truncate the file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}

	// Copy the content
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		s.logger.WithError(err).WithField("path", path).Error("Failed to write file content")
		// Try to clean up the file if writing failed
		_ = os.Remove(path)
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Update file size in metrics
	s.metrics.UpdateSize(n)

	// Ensure the file is synced to disk
	if err := f.Sync(); err != nil {
		s.logger.WithError(err).WithField("path", path).Error("Failed to sync file to disk")
	}

	s.logger.WithFields(logrus.Fields{
		"path": path,
		"size": n,
	}).Debug("Successfully stored file in cache")

	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(s.baseDir, key)

	_, err := os.Stat(path)
	if err == nil {
		s.logger.WithField("path", path).Debug("Cache hit: file exists")
		return true, nil
	}

	if os.IsNotExist(err) {
		s.logger.WithField("path", path).Debug("Cache miss: file does not exist")
		return false, nil
	}

	s.logger.WithError(err).WithFields(logrus.Fields{
		"key":  key,
		"path": path,
	}).Error("Error checking if file exists in cache")

	return false, err
}

// DeleteByPrefix deletes all files with the given prefix
func (s *LocalStorage) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	s.logger.WithField("prefix", prefix).Info("Deleting files by prefix")
	
	// Ensure the prefix is a directory
	searchPath := filepath.Join(s.baseDir, prefix)
	
	// Check if the path exists first
	fileInfo, err := os.Stat(searchPath)
	if os.IsNotExist(err) {
		s.logger.WithField("path", searchPath).Debug("Path does not exist, nothing to delete")
		return 0, nil // No files to delete
	}
	if err != nil {
		return 0, fmt.Errorf("error checking path %s: %w", searchPath, err)
	}

	// If it's a file, just delete it and return count 1
	if !fileInfo.IsDir() {
		if err := os.Remove(searchPath); err != nil {
			return 0, fmt.Errorf("error deleting file %s: %w", searchPath, err)
		}
		s.metrics.UpdateSize(-fileInfo.Size())
		s.logger.WithField("path", searchPath).Debug("Deleted file")
		return 1, nil
	}

	// For directories, walk and count all files
	var deletedCount int
	var totalSize int64

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == searchPath {
			return nil
		}

		// Only count files, not directories
		if !info.IsDir() {
			deletedCount++
			totalSize += info.Size()
			s.logger.WithField("path", path).Debug("Marked file for deletion")
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("error walking directory %s: %w", searchPath, err)
	}

	// Now actually delete the directory and all its contents
	if err := os.RemoveAll(searchPath); err != nil {
		return 0, fmt.Errorf("error deleting directory %s: %w", searchPath, err)
	}

	// Update metrics with total size and count of deleted files
	s.metrics.UpdateSize(-totalSize)
	s.metrics.RecordDeletion(deletedCount)

	s.logger.WithFields(logrus.Fields{
		"path":  searchPath,
		"count": deletedCount,
		"size":  totalSize,
	}).Info("Deleted directory and its contents")

	s.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  deletedCount,
	}).Info("Finished deleting files by prefix")

	return deletedCount, nil
}
