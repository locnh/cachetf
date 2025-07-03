package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoggerMiddleware tests the LoggerMiddleware function
func TestLoggerMiddleware(t *testing.T) {
	// Test cases for different status codes and their expected log levels
	tests := []struct {
		name           string
		handler        gin.HandlerFunc
		expectedStatus int
		expectedLevel  logrus.Level
		expectedMsg    string
	}{
		{
			name: "successful request",
			handler: func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			},
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedMsg:    "Request processed",
		},
		{
			name: "client error",
			handler: func(c *gin.Context) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
			},
			expectedStatus: http.StatusBadRequest,
			expectedLevel:  logrus.WarnLevel,
			expectedMsg:    "Client error",
		},
		{
			name: "server error",
			handler: func(c *gin.Context) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			},
			expectedStatus: http.StatusInternalServerError,
			expectedLevel:  logrus.ErrorLevel,
			expectedMsg:    "Server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect log output to a buffer
			var buf bytes.Buffer
			logrus.SetOutput(&buf)
			logrus.SetFormatter(&logrus.JSONFormatter{})

			// Create a test router with the middleware
			router := gin.New()
			router.Use(LoggerMiddleware())
			router.GET("/test", tt.handler)

			// Create a test request
			req, err := http.NewRequest("GET", "/test", nil)
			require.NoError(t, err)

			// Record the response
			w := httptest.NewRecorder()

			// Serve the request
			router.ServeHTTP(w, req)

			// Verify the response status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Parse the log entry
			var logEntry map[string]interface{}
			err = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry)
			require.NoError(t, err, "Log output should be valid JSON")

			// Verify log fields
			assert.Equal(t, "GET", logEntry["method"])
			assert.Equal(t, "/test", logEntry["path"])
			assert.Equal(t, float64(tt.expectedStatus), logEntry["status"])
			assert.Contains(t, logEntry, "latency")
			assert.Contains(t, logEntry, "clientIP")

			// Verify log level and message
			assert.Equal(t, tt.expectedMsg, logEntry["msg"])
			assert.Equal(t, tt.expectedLevel.String(), logEntry["level"])
		})
	}
}

// TestLoggerMiddlewareLatency tests that the latency is logged correctly
func TestLoggerMiddlewareLatency(t *testing.T) {
	// Redirect log output to a buffer
	var buf bytes.Buffer
	logrus.SetOutput(&buf)
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Create a test router with the middleware and a handler that sleeps
	router := gin.New()
	router.Use(LoggerMiddleware())
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(100 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	// Record the response
	w := httptest.NewRecorder()

	// Serve the request
	req := httptest.NewRequest("GET", "/slow", nil)
	start := time.Now()
	router.ServeHTTP(w, req)
	elapsed := time.Since(start)

	// Verify the response status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the log entry
	var logEntry map[string]interface{}
	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry)
	require.NoError(t, err, "Log output should be valid JSON")

	// Verify latency is logged and is a number (nanoseconds)
	latencyNs, ok := logEntry["latency"].(float64)
	assert.True(t, ok, "Latency should be a number")
	
	// Convert nanoseconds to time.Duration
	loggedLatency := time.Duration(int64(latencyNs))

	// The logged latency should be close to the actual elapsed time
	tolerance := 50 * time.Millisecond
	assert.InDelta(t, elapsed.Milliseconds(), loggedLatency.Milliseconds(), float64(tolerance.Milliseconds()))
}

// TestLoggerMiddlewareClientIP tests that the client IP is logged correctly
func TestLoggerMiddlewareClientIP(t *testing.T) {
	// Test cases for different X-Forwarded-For headers
	tests := []struct {
		name       string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "no x-forwarded-for",
			headers:    map[string]string{},
			expectedIP: "192.0.2.1", // Default from httptest
		},
		{
			name: "with x-forwarded-for",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195, 198.51.100.1",
			},
			expectedIP: "203.0.113.195",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect log output to a buffer
			var buf bytes.Buffer
			logrus.SetOutput(&buf)
			logrus.SetFormatter(&logrus.JSONFormatter{})

			// Create a test router with the middleware
			router := gin.New()
			router.Use(LoggerMiddleware())
			router.GET("/ip", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			// Create a test request
			req := httptest.NewRequest("GET", "/ip", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// Record the response
			w := httptest.NewRecorder()

			// Serve the request
			router.ServeHTTP(w, req)

			// Parse the log entry
			var logEntry map[string]interface{}
			err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry)
			require.NoError(t, err, "Log output should be valid JSON")

			// Verify client IP is logged correctly
			assert.Equal(t, tt.expectedIP, logEntry["clientIP"])
		})
	}
}

// TestMain provides setup and teardown for all tests
func TestMain(m *testing.M) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Run tests
	code := m.Run()

	// Exit with the appropriate code
	os.Exit(code)
}
