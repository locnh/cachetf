package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of the storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorage) Put(ctx context.Context, key string, r io.Reader) error {
	args := m.Called(ctx, key, r)
	return args.Error(0)
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockStorage) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	args := m.Called(ctx, prefix)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func TestDeleteCache(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		setupMock      func(*MockStorage)
		expectedStatus int
		expectedBody   map[string]interface{}
		expectedLogs   []string
	}{
		{
			name: "delete by registry only",
			path: "/registry.terraform.io",
			setupMock: func(ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io").Return(5, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"message": "Cache cleared successfully",
				"deleted": float64(5),
			},
			expectedLogs: []string{"Deleting cache by prefix"},
		},
		{
			name: "delete by namespace",
			path: "/registry.terraform.io/hashicorp",
			setupMock: func(ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp").Return(3, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"message": "Cache cleared successfully",
				"deleted": float64(3),
			},
			expectedLogs: []string{"Deleting cache by prefix"},
		},
		{
			name: "delete by provider",
			path: "/registry.terraform.io/hashicorp/aws",
			setupMock: func(ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws").Return(2, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"message": "Cache cleared successfully",
				"deleted": float64(2),
			},
			expectedLogs: []string{"Deleting cache by prefix"},
		},
		{
			name: "delete by version",
			path: "/registry.terraform.io/hashicorp/aws/1.2.3",
			setupMock: func(ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws/1.2.3").Return(1, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"message": "Cache cleared successfully",
				"deleted": float64(1),
			},
			expectedLogs: []string{"Deleting cache by prefix"},
		},
		{
			name: "storage error",
			path: "/registry.terraform.io",
			setupMock: func(ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io").Return(0, errors.New("storage error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: map[string]interface{}{
				"error": "Failed to delete cache: storage error",
			},
			expectedLogs: []string{"Deleting cache by prefix", "Failed to delete cache"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock storage
			mockStorage := new(MockStorage)
			tc.setupMock(mockStorage)

			// Setup logger with test hook
			logger, hook := test.NewNullLogger()

			// Create handler
			handler := NewCacheHandler(mockStorage, logger)

			// Create router
			router := gin.New()
			api := router.Group("/")
			handler.RegisterCacheRoutes(api)

			// Create request
			req, _ := http.NewRequest("DELETE", tc.path, nil)
			w := httptest.NewRecorder()

			// Serve the request
			router.ServeHTTP(w, req)

			// Check response status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// Parse response body
			var responseBody map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &responseBody)
			assert.NoError(t, err)

			// Check response body
			assert.Equal(t, tc.expectedBody, responseBody)

			// Check logs
			for _, expectedLog := range tc.expectedLogs {
				found := false
				for _, entry := range hook.AllEntries() {
					if entry.Message == expectedLog {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", expectedLog)
			}

			// Verify all expectations were met
			mockStorage.AssertExpectations(t)
		})
	}
}

func TestRegisterCacheRoutes(t *testing.T) {
	// Setup
	mockStorage := new(MockStorage)
	logger := logrus.New()

	// Set up mock expectations for each route
	mockStorage.On("DeleteByPrefix", mock.Anything, "registry.terraform.io").Return(1, nil)
	mockStorage.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp").Return(1, nil)
	mockStorage.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws").Return(1, nil)
	mockStorage.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws/1.2.3").Return(1, nil)

	handler := NewCacheHandler(mockStorage, logger)

	// Create a test router
	router := gin.New()
	api := router.Group("/api")
	handler.RegisterCacheRoutes(api)

	// Test all registered routes
	testCases := []struct {
		name   string
		method string
		path   string
		setup  func(*testing.T, *MockStorage)
		status int
	}{
		{
			name:   "delete registry",
			method: "DELETE",
			path:   "/api/registry.terraform.io",
			setup: func(t *testing.T, ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io").Return(1, nil).Once()
			},
			status: http.StatusOK,
		},
		{
			name:   "delete namespace",
			method: "DELETE",
			path:   "/api/registry.terraform.io/hashicorp",
			setup: func(t *testing.T, ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp").Return(1, nil).Once()
			},
			status: http.StatusOK,
		},
		{
			name:   "delete provider",
			method: "DELETE",
			path:   "/api/registry.terraform.io/hashicorp/aws",
			setup: func(t *testing.T, ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws").Return(1, nil).Once()
			},
			status: http.StatusOK,
		},
		{
			name:   "delete version",
			method: "DELETE",
			path:   "/api/registry.terraform.io/hashicorp/aws/1.2.3",
			setup: func(t *testing.T, ms *MockStorage) {
				ms.On("DeleteByPrefix", mock.Anything, "registry.terraform.io/hashicorp/aws/1.2.3").Return(1, nil).Once()
			},
			status: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock and set up expectations for this test case
			mockStorage.ExpectedCalls = nil
			if tc.setup != nil {
				tc.setup(t, mockStorage)
			}

			req, _ := http.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Check the status code
			assert.Equal(t, tc.status, w.Code, "Unexpected status code for %s %s", tc.method, tc.path)

			// Verify all expectations were met
			mockStorage.AssertExpectations(t)
		})
	}
}
