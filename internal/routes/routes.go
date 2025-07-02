package routes

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"cache-t-f/internal/handler"
)

// SetupRoutes configures all the routes for the application
func SetupRoutes(router *gin.Engine, config *Config) {
	// Ensure cache directory exists and is writable
	if config.CacheDir == "" {
		// This should not happen as we set a default in config
		logrus.Fatal("Cache directory is not configured")
	}

	// Create handler with logger and cache directory
	registryHandler := handler.NewRegistryHandler(logrus.StandardLogger(), config.CacheDir)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// Base group with configurable URI prefix
	base := router.Group(config.URIPrefix)

	// Terraform Registry API endpoints
	registry := base.Group("/:registry/:namespace/:provider")
	{
		// GET /:registry/:namespace/:provider/index.json
		registry.GET("/index.json", registryHandler.GetProviderIndex)

		// Handle both .zip and .json files with a single route
		registry.GET("/:file", func(c *gin.Context) {
			file := c.Param("file")

			// Check if it's a .json file (version info)
			if strings.HasSuffix(file, ".json") {
				// Extract version from filename (remove .json)
				version := strings.TrimSuffix(file, ".json")
				c.Set("version", version)
				registryHandler.GetProviderVersion(c)
				return
			}

			// Check if it's a .zip file (provider binary)
			if strings.HasSuffix(file, ".zip") {
				re := regexp.MustCompile(`^terraform-provider-([a-z0-9-]+)_(\d+\.\d+\.\d+)_([a-z]+)_([a-z0-9]+)\.zip$`)
				matches := re.FindStringSubmatch(file)
				
				if len(matches) != 5 { // full match + 4 groups
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file format"})
					return
				}

				// Log the request
				logrus.WithFields(logrus.Fields{
					"provider": matches[1],
					"version":  matches[2],
					"os":       matches[3],
					"arch":     matches[4],
				}).Info("Provider download requested")

				// Set the version in context for the handler
				c.Set("version", matches[2])
				registryHandler.DownloadProvider(c)
				return
			}

			// If not .json or .zip, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Not Found",
			})
		})
	}

	// Add 404 handler
	router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{
			"error": "Not Found",
		})
	})
}

// Config holds the configuration for routes
type Config struct {
	URIPrefix string
	CacheDir  string
}
