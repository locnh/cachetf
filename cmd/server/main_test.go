package main_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cachetf/internal/config"
	"cachetf/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsServer(t *testing.T) {
	// Create a temporary directory for test cache
	tempDir, err := os.MkdirTemp("", "cachetf-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test configuration
	cfg := &config.Config{
		ServerPort:  8080,
		MetricsPort: 9100,
		URIPrefix:   "/providers",
		StorageType: config.StorageTypeLocal,
		CacheDir:    filepath.Join(tempDir, "cache"),
		LogLevel:    "debug",
	}

	// Create a test storage and wrap it with metrics
	localStore := storage.NewLocalStorage(cfg.CacheDir, logrus.StandardLogger())
	metricsWrapper := storage.NewMetricsWrapper(localStore)

	// Create a test context with timeout
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start metrics server
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler: metricsMux,
	}

	// Start main server
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Use the metrics-wrapped storage in the test
	r.Use(func(c *gin.Context) {
		c.Set("storage", metricsWrapper)
		c.Next()
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServerPort),
		Handler: r,
	}

	// Start servers in goroutines
	go func() {
		_ = metricsSrv.ListenAndServe()
	}()

	go func() {
		_ = srv.ListenAndServe()
	}()

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("Metrics endpoint returns 200", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", cfg.MetricsPort))
		assert.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Main server returns 404 for unknown routes", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/unknown", cfg.ServerPort))
		assert.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// Cleanup
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		_ = metricsSrv.Shutdown(context.Background())
	})
}
