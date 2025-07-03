package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestInitLogger tests the InitLogger function with various log levels
func TestInitLogger(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedLevel logrus.Level
	}{
		{"TraceLevel", "trace", logrus.TraceLevel},
		{"DebugLevel", "debug", logrus.DebugLevel},
		{"InfoLevel", "info", logrus.InfoLevel},
		{"WarnLevel", "warn", logrus.WarnLevel},
		{"ErrorLevel", "error", logrus.ErrorLevel},
		{"FatalLevel", "fatal", logrus.FatalLevel},
		{"PanicLevel", "panic", logrus.PanicLevel},
		{"InvalidLevel", "invalid", logrus.InfoLevel}, // Default to InfoLevel for invalid levels
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test InitLogger
			InitLogger(tt.level)

			// Verify log level was set correctly
			assert.Equal(t, tt.expectedLevel, logrus.GetLevel(), "Log level should be set correctly")
		})
	}
}

// TestGetLogger tests the GetLogger function
func TestGetLogger(t *testing.T) {
	// Initialize logger with debug level to capture all logs
	InitLogger("debug")

	// Redirect log output to a buffer
	var buf bytes.Buffer
	logrus.SetOutput(&buf)

	// Test with a context
	context := "test-context"
	logger := GetLogger(context)

	// Log a test message
	testMessage := "test log message"
	logger.Info(testMessage)

	// Parse the JSON log entry
	var logEntry map[string]interface{}
	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to unmarshal log entry: %v", err)
	}

	// Verify the log entry contains the context and message
	assert.Equal(t, context, logEntry["context"], "Log entry should contain the context")
	assert.Equal(t, testMessage, logEntry["msg"], "Log entry should contain the message")
	assert.Equal(t, "info", logEntry["level"], "Log level should be info")
}

// TestLoggerOutputFormat tests that the logger outputs in JSON format
func TestLoggerOutputFormat(t *testing.T) {
	// Initialize logger
	InitLogger("info")

	// Redirect log output to a buffer
	var buf bytes.Buffer
	logrus.SetOutput(&buf)

	// Log a test message
	logrus.Info("test format")

	// The output should be valid JSON
	var logEntry map[string]interface{}
	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &logEntry)
	assert.NoError(t, err, "Log output should be valid JSON")

	// Should contain standard fields
	assert.Contains(t, logEntry, "level", "Log entry should contain 'level' field")
	assert.Contains(t, logEntry, "msg", "Log entry should contain 'msg' field")
	assert.Contains(t, logEntry, "time", "Log entry should contain 'time' field")
}

// TestLoggerInitialization tests the logger's basic functionality
func TestLoggerInitialization(t *testing.T) {
	// This test verifies that the logger can be initialized without panicking
	// and that the log level is set correctly
	InitLogger("info")
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel(), "Log level should be set to info")
}

// TestMain provides setup and teardown for all tests
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Exit with the appropriate code
	os.Exit(code)
}
