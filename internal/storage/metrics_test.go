package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockStorage is a mock implementation of the Storage interface
type mockStorage struct {
	mock.Mock
}

func (m *mockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *mockStorage) Put(ctx context.Context, key string, r io.Reader) error {
	// Read the content to ensure it's not empty
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// Create a new reader for the mock to return
	args := m.Called(ctx, key, bytes.NewReader(content))
	return args.Error(0)
}

func (m *mockStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *mockStorage) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	args := m.Called(ctx, prefix)
	return args.Int(0), args.Error(1)
}

func TestMetricsWrapper_Get(t *testing.T) {
	// Create a mock storage
	mockStore := new(mockStorage)
	wrapper := NewMetricsWrapper(mockStore)

	// Set up expectations
	expectedContent := "test content"
	expectedReader := io.NopCloser(bytes.NewBufferString(expectedContent))
	mockStore.On("Get", mock.Anything, "test-key").Return(expectedReader, nil)

	// Call the method
	reader, err := wrapper.Get(context.Background(), "test-key")
	require.NoError(t, err)
	defer reader.Close()

	// Verify the result
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(content))

	// Verify the mock was called
	mockStore.AssertExpectations(t)
}

func TestMetricsWrapper_Put(t *testing.T) {
	// Create a mock storage
	mockStore := new(mockStorage)
	wrapper := NewMetricsWrapper(mockStore)

	// Set up test content
	content := "test content"
	mockStore.On("Put", mock.Anything, "test-key", mock.Anything).Return(nil)

	// Call the method
	err := wrapper.Put(context.Background(), "test-key", bytes.NewReader([]byte(content)))

	// Verify the result
	require.NoError(t, err)

	// Verify the mock was called
	mockStore.AssertExpectations(t)
}

func TestMetricsWrapper_Exists(t *testing.T) {
	// Create a mock storage
	mockStore := new(mockStorage)
	wrapper := NewMetricsWrapper(mockStore)

	// Set up expectations
	mockStore.On("Exists", mock.Anything, "test-key").Return(true, nil)

	// Call the method
	exists, err := wrapper.Exists(context.Background(), "test-key")

	// Verify the result
	require.NoError(t, err)
	assert.True(t, exists)

	// Verify the mock was called
	mockStore.AssertExpectations(t)
}

func TestMetricsWrapper_DeleteByPrefix(t *testing.T) {
	// Create a mock storage
	mockStore := new(mockStorage)
	wrapper := NewMetricsWrapper(mockStore)

	// Set up expectations
	mockStore.On("DeleteByPrefix", mock.Anything, "test-prefix").Return(2, nil)

	// Call the method
	deleted, err := wrapper.DeleteByPrefix(context.Background(), "test-prefix")

	// Verify the result
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	// Verify the mock was called
	mockStore.AssertExpectations(t)
}

func TestMetricsWrapper_ErrorHandling(t *testing.T) {
	// Create a mock storage
	mockStore := new(mockStorage)
	wrapper := NewMetricsWrapper(mockStore)

	// Test Get error
	expectedErr := errors.New("test error")
	mockStore.On("Get", mock.Anything, "error-key").Return(nil, expectedErr)
	_, err := wrapper.Get(context.Background(), "error-key")
	assert.ErrorIs(t, err, expectedErr)

	// Test Put error
	mockStore.On("Put", mock.Anything, "error-key", mock.Anything).Return(expectedErr)
	err = wrapper.Put(context.Background(), "error-key", bytes.NewReader([]byte("test")))
	assert.ErrorIs(t, err, expectedErr)

	// Test Exists error
	mockStore.On("Exists", mock.Anything, "error-key").Return(false, expectedErr)
	_, err = wrapper.Exists(context.Background(), "error-key")
	assert.ErrorIs(t, err, expectedErr)

	// Test DeleteByPrefix error
	mockStore.On("DeleteByPrefix", mock.Anything, "error-prefix").Return(0, expectedErr)
	_, err = wrapper.DeleteByPrefix(context.Background(), "error-prefix")
	assert.ErrorIs(t, err, expectedErr)

	// Verify all mocks were called
	mockStore.AssertExpectations(t)
}
