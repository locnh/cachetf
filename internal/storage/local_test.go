package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLocalStorage(t *testing.T) (*LocalStorage, string) {
	t.Helper()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "local-storage-test-*")
	require.NoError(t, err, "Failed to create temp directory")

	// Create a test logger
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	// Create a new LocalStorage instance
	storage := NewLocalStorage(tempDir, logger)

	// Cleanup function will be called when the test completes
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return storage, tempDir
}

func TestLocalStorage_PutAndGet(t *testing.T) {
	storage, tempDir := setupLocalStorage(t)
	ctx := context.Background()

	// Test data
	key := "test-file.txt"
	content := []byte("test content")

	// Test Put
	err := storage.Put(ctx, key, bytes.NewReader(content))
	require.NoError(t, err, "Put should not return an error")

	// Verify file was created
	filePath := filepath.Join(tempDir, key)
	_, err = os.Stat(filePath)
	require.NoError(t, err, "File should exist after Put")

	// Test Get
	reader, err := storage.Get(ctx, key)
	require.NoError(t, err, "Get should not return an error")
	defer reader.Close()

	// Read the content
	got, err := io.ReadAll(reader)
	require.NoError(t, err, "Failed to read from Get result")

	// Verify content
	assert.Equal(t, content, got, "Content should match what was written")
}

func TestLocalStorage_Exists(t *testing.T) {
	storage, tempDir := setupLocalStorage(t)
	ctx := context.Background()

	// Test non-existent file
	exists, err := storage.Exists(ctx, "non-existent")
	require.NoError(t, err, "Exists should not return an error for non-existent file")
	assert.False(t, exists, "Non-existent file should not exist")

	// Create a test file
	key := "test-file.txt"
	filePath := filepath.Join(tempDir, key)
	err = os.WriteFile(filePath, []byte("test"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Test case: Object exists
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err, "Exists should not return an error for existing file")
	assert.True(t, exists, "Existing file should exist")

	// Test case: Object exists with io.Reader
	reader := bytes.NewReader([]byte("test"))
	err = storage.Put(ctx, key, reader)
	require.NoError(t, err, "Put should not return an error")
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err, "Exists should not return an error for existing file")
	assert.True(t, exists, "Existing file should exist")
}

func TestLocalStorage_DeleteByPrefix(t *testing.T) {
	storage, tempDir := setupLocalStorage(t)
	ctx := context.Background()

	// Create test files
	files := []struct {
		key     string
		content []byte
	}{
		{"prefix1/file1.txt", []byte("file1")},
		{"prefix1/file2.txt", []byte("file2")},
		{"prefix2/file3.txt", []byte("file3")},
	}

	for _, file := range files {
		filePath := filepath.Join(tempDir, file.key)
		err := os.MkdirAll(filepath.Dir(filePath), 0755)
		require.NoError(t, err, "Failed to create directory")
		err = os.WriteFile(filePath, file.content, 0644)
		require.NoError(t, err, "Failed to create test file")
	}

	// Delete files with prefix1
	deleted, err := storage.DeleteByPrefix(ctx, "prefix1")
	require.NoError(t, err, "DeleteByPrefix should not return an error")
	assert.Equal(t, 2, deleted, "Should have deleted 2 files")

	// Verify files were deleted
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.key)
		_, err := os.Stat(filePath)
		if file.key == "prefix2/file3.txt" {
			// This file should still exist
			assert.NoError(t, err, "File with different prefix should still exist")
		} else {
			// These files should have been deleted
			assert.True(t, os.IsNotExist(err), "File should have been deleted")
		}
	}
}

func TestLocalStorage_ConcurrentAccess(t *testing.T) {
	storage, _ := setupLocalStorage(t)
	ctx := context.Background()
	key := "concurrent.txt"

	// Number of concurrent operations
	const numOps = 10

	// Channel to collect results
	errs := make(chan error, numOps)

	// Run concurrent writes
	for i := 0; i < numOps; i++ {
		go func(i int) {
			content := []byte{byte(i)}
			errs <- storage.Put(ctx, key, bytes.NewReader(content))
		}(i)
	}

	// Wait for all writes to complete
	for i := 0; i < numOps; i++ {
		err := <-errs
		assert.NoError(t, err, "Concurrent Put should not return an error")
	}

	// Read the final content
	reader, err := storage.Get(ctx, key)
	require.NoError(t, err, "Get should not return an error")
	defer reader.Close()

	got, err := io.ReadAll(reader)
	require.NoError(t, err, "Failed to read from Get result")

	// The final content should be one of the written bytes
	assert.Len(t, got, 1, "Content should be 1 byte")
}

func TestLocalStorage_GetNonExistent(t *testing.T) {
	storage, _ := setupLocalStorage(t)
	ctx := context.Background()

	// Test getting a non-existent file
	reader, err := storage.Get(ctx, "non-existent")
	assert.Error(t, err, "Get should return an error for non-existent file")
	assert.Nil(t, reader, "Reader should be nil when file doesn't exist")
}

func TestLocalStorage_InvalidPath(t *testing.T) {
	storage, _ := setupLocalStorage(t)
	ctx := context.Background()

	// Test with an invalid path
	err := storage.Put(ctx, "../invalid/path.txt", bytes.NewReader([]byte("test")))
	assert.Error(t, err, "Put should return an error for invalid path")

	// Test Exists with invalid path
	_, err = storage.Exists(ctx, "../invalid/path.txt")
	assert.Error(t, err, "Exists should return an error for invalid path")

	// Test DeleteByPrefix with invalid path
	_, err = storage.DeleteByPrefix(ctx, "../invalid/prefix")
	assert.Error(t, err, "DeleteByPrefix should return an error for invalid path")
}
