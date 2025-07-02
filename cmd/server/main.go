package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"cache-t-f/internal/config"
	"cache-t-f/internal/routes"
	"cache-t-f/pkg/logger"
)

func main() {
	// Initialize logger
	logger.InitLogger()
	logrus.Info("Starting application...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logrus.Fatalf("Error loading config: %v", err)
	}

	// Initialize Gin
	r := gin.New()

	// Setup routes
	routes.SetupRoutes(r, &routes.Config{
		URIPrefix: cfg.URIPrefix,
		CacheDir:  cfg.CacheDir,
	})

	// Start server
	serverAddr := ":" + cfg.ServerPort
	logrus.Infof("Server is running on %s", serverAddr)

	// Graceful shutdown
	go func() {
		if err := r.Run(serverAddr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logrus.Info("Shutting down server...")
}
