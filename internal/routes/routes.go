package routes

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"cachetf/internal/handler"
	"cachetf/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SetupRoutes configures all the routes for the application
func SetupRoutes(router *gin.Engine, config *Config) {
	// Ensure storage is configured
	if config.Storage == nil {
		logrus.Fatal("Storage is not configured")
	}

	// Create handlers with logger and storage
	logger := logrus.StandardLogger()
	registryHandler := handler.NewRegistryHandler(logger, config.Storage)
	cacheHandler := handler.NewCacheHandler(config.Storage, logger)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// Base group with configurable URI prefix
	base := router.Group(config.URIPrefix)

	// Cache management endpoints
	{
		// DELETE /:registry/...
		base.DELETE("/:registry", cacheHandler.DeleteCache)
		base.DELETE("/:registry/:namespace", cacheHandler.DeleteCache)
		base.DELETE("/:registry/:namespace/:provider", cacheHandler.DeleteCache)
		base.DELETE("/:registry/:namespace/:provider/:version", cacheHandler.DeleteCache)
		base.DELETE("/:registry/:namespace/:provider/:version/:file", cacheHandler.DeleteCache)
	}

	// Terraform Registry API endpoints
	registry := base.Group("/:registry/:namespace/:provider")
	{
		// GET /:registry/:namespace/:provider/index.json
		registry.GET("/index.json", registryHandler.GetProviderIndex)

		// Handle both version and file requests
		re := regexp.MustCompile(`^(\d+\.\d+\.\d+)(?:-[\w-]+)?(?:\.json)?$`)
		registry.GET("/:fileOrVersion", func(c *gin.Context) {
			fileOrVersion := c.Param("fileOrVersion")

			// Check if it's a version request (e.g., 1.2.3 or 1.2.3.json)
			version := strings.TrimSuffix(fileOrVersion, ".json")
			if re.MatchString(version) {
				c.Set("version", version)
				registryHandler.GetProviderVersion(c)
				return
			}

			// Check if it's a .zip file (provider binary)
			if strings.HasSuffix(fileOrVersion, ".zip") {
				// Debug log the incoming filename
				logrus.WithField("filename", fileOrVersion).Debug("Processing provider binary request")

				// More permissive pattern to match the provider binary filename
				pattern := `^terraform-provider-([^_]+?)_(\d+\.\d+\.\d+(?:-[\w-]+)?)_([^_]+)_([^.]+)\.zip$`
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(fileOrVersion)

				logrus.WithFields(logrus.Fields{
					"filename": fileOrVersion,
					"pattern":  pattern,
					"matches":  matches,
				}).Debug("Regex match results")

				if len(matches) < 5 { // full match + 4 groups
					errMsg := fmt.Sprintf("invalid file format: %s (pattern: %s, matches: %v)", fileOrVersion, pattern, matches)
					logrus.WithField("filename", fileOrVersion).Error("Failed to match provider binary pattern")
					c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
					return
				}

				// Set the file parameter in the URL parameters
				c.Params = append(c.Params, gin.Param{
					Key:   "file",
					Value: fileOrVersion,
				})

				// Also set the individual components as context values
				c.Set("version", matches[2])
				c.Set("os", matches[3])
				c.Set("arch", matches[4])

				// Log the file download request
				registryHandler.Logger().WithFields(logrus.Fields{
					"file":    fileOrVersion,
					"version": matches[2],
					"os":      matches[3],
					"arch":    matches[4],
				}).Debug("Calling DownloadProvider")

				// Call the download handler
				registryHandler.DownloadProvider(c)
				return
			}

			// If we get here, it's an unsupported request
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported request"})
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
	Storage   storage.Storage
}
