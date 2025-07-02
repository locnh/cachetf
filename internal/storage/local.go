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
)

type LocalStorage struct {
	baseDir string
	logger  *logrus.Logger
	// mutexes provides per-key locking to prevent concurrent writes to the same file
	mutexes sync.Map
}

// getMutex returns a mutex for the given key, creating it if it doesn't exist
// This ensures that concurrent operations on the same key are serialized
func (s *LocalStorage) getMutex(key string) *sync.Mutex {
	mutex, _ := s.mutexes.LoadOrStore(key, &sync.Mutex{})
	return mutex.(*sync.Mutex)
}

func NewLocalStorage(baseDir string, logger *logrus.Logger) *LocalStorage {
	return &LocalStorage{
		baseDir: baseDir,
		logger:  logger,
	}
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.baseDir, key)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.WithFields(logrus.Fields{
				"key":  key,
				"path": path,
			}).Debug("Cache miss: file not found")
		} else {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"key":  key,
				"path": path,
			}).Error("Cache access error")
		}
		return nil, err
	}

	s.logger.WithFields(logrus.Fields{
		"key":  key,
		"path": path,
	}).Debug("Cache hit: file found")

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

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		s.logger.WithError(err).WithField("path", filepath.Dir(path)).Error("Failed to create directory")
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create the file with exclusive creation flag to prevent race conditions
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			// File was created by another goroutine after our initial check
			s.logger.WithField("path", path).Debug("File was created by another goroutine")
			return nil
		}
		s.logger.WithError(err).WithField("path", path).Error("Failed to create file")
		return fmt.Errorf("failed to create file: %w", err)
	}

	// Copy the content
	defer f.Close()
	if _, err = io.Copy(f, r); err != nil {
		s.logger.WithError(err).WithField("path", path).Error("Failed to write file content")
		// Try to clean up the file if writing failed
		_ = os.Remove(path)
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Ensure the file is synced to disk
	if err := f.Sync(); err != nil {
		s.logger.WithError(err).WithField("path", path).Error("Failed to sync file to disk")
		return fmt.Errorf("failed to sync file: %w", err)
	}

	s.logger.WithField("path", path).Debug("Successfully wrote file")
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(s.baseDir, key)
	_, err := os.Stat(path)

	if err == nil {
		s.logger.WithFields(logrus.Fields{
			"key":  key,
			"path": path,
		}).Debug("Cache hit: file exists")
		return true, nil
	}

	if os.IsNotExist(err) {
		s.logger.WithFields(logrus.Fields{
			"key":  key,
			"path": path,
		}).Debug("Cache miss: file does not exist")
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
	_, err := os.Stat(searchPath)
	if os.IsNotExist(err) {
		return 0, nil // No files to delete
	}
	if err != nil {
		return 0, fmt.Errorf("error checking path %s: %w", searchPath, err)
	}

	// Walk the directory and delete matching files
	var deletedCount int
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == searchPath {
			return nil
		}

		// Delete the file or directory
		if err := os.RemoveAll(path); err != nil {
			s.logger.WithError(err).WithField("path", path).Error("Failed to delete path")
			return err
		}

		// If it's a directory, we'll count each file in it
		if info.IsDir() {
			return filepath.SkipDir
		}

		deletedCount++
		s.logger.WithField("path", path).Debug("Deleted file")
		return nil
	})

	if err != nil {
		return deletedCount, fmt.Errorf("error walking path %s: %w", searchPath, err)
	}

	s.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"count":  deletedCount,
	}).Info("Finished deleting files by prefix")

	return deletedCount, nil
}
