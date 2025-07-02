package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"cachetf/internal/config"
	routes "cachetf/internal/routes"
	"cachetf/internal/storage"
	"cachetf/pkg/logger"
)

func main() {
	// Create context that listens for the interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Initialize logger
	logger.InitLogger(cfg.LogLevel)

	// Create router
	r := gin.New()
	r.Use(gin.Recovery())

	// Initialize storage
	var store storage.Storage
	if cfg.StorageType == "s3" {
		s3Config := &storage.S3Config{
			Bucket: cfg.S3.Bucket,
			Region: cfg.S3.Region,
		}
		store, err = storage.NewS3Storage(s3Config, logrus.StandardLogger())
		if err != nil {
			logrus.Fatalf("Failed to initialize S3 storage: %v", err)
		}
	} else {
		// Default to local filesystem storage
		store = storage.NewLocalStorage(cfg.CacheDir, logrus.StandardLogger())
	}

	// Setup routes
	routes.SetupRoutes(r, &routes.Config{
		URIPrefix: cfg.URIPrefix,
		Storage:   store,
	})

	// Initialize server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServerPort),
		Handler: r,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling
	go func() {
		logrus.Infof("Server is running on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Server error: %v", err)
		}
	}()

	// Listen for the interrupt signal
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown
	stop()
	logrus.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logrus.Fatal("Server forced to shutdown: ", err)
	}

	logrus.Info("Server exiting")
}
