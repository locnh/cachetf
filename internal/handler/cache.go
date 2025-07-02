package handler

import (
	"fmt"
	"net/http"
	"strings"

	"cachetf/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CacheHandler handles cache-related operations
type CacheHandler struct {
	storage storage.Storage
	logger  *logrus.Logger
}

// NewCacheHandler creates a new CacheHandler
func NewCacheHandler(storage storage.Storage, logger *logrus.Logger) *CacheHandler {
	return &CacheHandler{
		storage: storage,
		logger:  logger,
	}
}

// DeleteCache handles DELETE requests to clear cache by prefix
func (h *CacheHandler) DeleteCache(c *gin.Context) {
	// Get path parameters
	params := []string{
		c.Param("registry"),
	}

	// Add optional parameters if they exist
	if namespace := c.Param("namespace"); namespace != "" {
		params = append(params, namespace)

		if provider := c.Param("provider"); provider != "" {
			params = append(params, provider)

			if version := c.Param("version"); version != "" {
				params = append(params, version)

				// Handle file deletion if specified
				if file := c.Param("file"); file != "" {
					params = append(params, file)
				}
			}
		}
	}

	// Join parameters to create the prefix
	prefix := strings.Join(params, "/")

	// Log the deletion attempt
	h.logger.WithFields(logrus.Fields{
		"prefix": prefix,
	}).Info("Deleting cache by prefix")

	// Delete objects with the given prefix
	count, err := h.storage.DeleteByPrefix(c.Request.Context(), prefix)
	if err != nil {
		h.logger.WithError(err).Error("Failed to delete cache")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to delete cache: %v", err),
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"message": "Cache cleared successfully",
		"deleted": count,
	})
}

// RegisterCacheRoutes registers cache-related routes
func (h *CacheHandler) RegisterCacheRoutes(router *gin.RouterGroup) {
	// DELETE /:registry/...
	router.DELETE("/:registry", h.DeleteCache)
	router.DELETE("/:registry/:namespace", h.DeleteCache)
	router.DELETE("/:registry/:namespace/:provider", h.DeleteCache)
	router.DELETE("/:registry/:namespace/:provider/:version", h.DeleteCache)
}
