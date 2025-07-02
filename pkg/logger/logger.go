package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// InitLogger initializes the logger with default configuration
func InitLogger() {
	// Log as JSON instead of the default ASCII formatter
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// Output to stdout instead of the default stderr
	logrus.SetOutput(os.Stdout)

	// Set log level from environment variable, default to info
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch logLevel {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.WithField("level", logrus.GetLevel().String()).Info("Log level set")
}

// GetLogger returns a logger with the specified context
func GetLogger(context string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"context": context,
	})
}
