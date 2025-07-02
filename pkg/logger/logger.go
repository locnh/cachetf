package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// InitLogger initializes the logger with the specified log level
// Supported levels: trace, debug, info, warn, error, fatal, panic
// Defaults to info if an invalid level is provided
func InitLogger(level string) {
	// Log as JSON instead of the default ASCII formatter
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// Output to stdout instead of the default stderr
	logrus.SetOutput(os.Stdout)

	switch strings.ToLower(level) {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	case "panic":
		logrus.SetLevel(logrus.PanicLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
		logrus.Warnf("Invalid log level '%s', defaulting to 'info'", level)
	}

	logrus.WithField("log_level", logrus.GetLevel().String()).Info("Logger initialized")
}

// GetLogger returns a logger with the specified context
func GetLogger(context string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"context": context,
	})
}
