package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"cachetf/internal/storage"
)

// RegistryHandler handles Terraform registry API requests
type RegistryHandler struct {
	logger     *logrus.Logger
	httpClient *http.Client
	apiVersion string
	storage    storage.Storage
	mu         sync.RWMutex // Protects concurrent access to the cache
}

// Logger returns the logger instance for this handler
func (h *RegistryHandler) Logger() *logrus.Logger {
	return h.logger
}

// ProviderVersionsResponse represents the response from the Terraform registry versions endpoint
type ProviderVersionsResponse struct {
	ID       string `json:"id"`
	Versions []struct {
		Version   string   `json:"version"`
		Protocols []string `json:"protocols"`
		Platforms []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"platforms"`
	} `json:"versions"`
	Warnings interface{} `json:"warnings"`
}

// ProviderResponse represents the response from the Terraform registry
// for provider details
type ProviderResponse struct {
	ID          string   `json:"id"`
	Owner       string   `json:"owner"`
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Versions    []string `json:"versions"`
}

// ArchiveInfo contains download URL information for a specific platform
// This is a named type to ensure consistent JSON tags
type ArchiveInfo struct {
	URL string `json:"url"`
}

// DownloadResponse represents the response from the download endpoint
type DownloadResponse struct {
	OS                  string   `json:"os"`
	Arch                string   `json:"arch"`
	Filename            string   `json:"filename"`
	DownloadURL         string   `json:"download_url"`
	SHASumsURL          string   `json:"shasums_url"`
	SHASumsSignatureURL string   `json:"shasums_signature_url"`
	SHASum              string   `json:"shasum,omitempty"`
	Protocols           []string `json:"protocols"`
	SigningKeys         struct {
		GPGPublicKeys []struct {
			KeyID          string `json:"key_id"`
			ASCIIArmor     string `json:"ascii_armor"`
			TrustSignature string `json:"trust_signature"`
			Source         string `json:"source"`
			SourceURL      string `json:"source_url"`
		} `json:"gpg_public_keys"`
	} `json:"signing_keys"`
}

// VersionResponse represents the response for a specific version
type VersionResponse struct {
	Archives map[string]ArchiveInfo `json:"archives"`
}

// IndexResponse represents the response for the index.json endpoint
type IndexResponse struct {
	Versions map[string]struct{} `json:"versions"`
}

// NewRegistryHandler creates a new RegistryHandler
func NewRegistryHandler(logger *logrus.Logger, storage storage.Storage) *RegistryHandler {
	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &RegistryHandler{
		logger:     logger,
		httpClient: httpClient,
		apiVersion: "1.0.0",
		storage:    storage,
	}
}

// getCacheKey returns the storage key for a cached file in the format:
// registry/namespace/provider/version/filename.zip
// where filename is in the format: terraform-provider-{name}_{version}_{os}_{arch}.zip
func (h *RegistryHandler) getCacheKey(registry, namespace, provider, version, platform, arch string) string {
	// Construct the filename in the standard Terraform provider format
	filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider, version, platform, arch)

	// Return the full path with the original filename
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		registry, namespace, provider, version, filename)
}

// Helper function to download a file and store it with checksum verification
func (h *RegistryHandler) downloadFile(url, key, expectedSHA256 string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.WithFields(logrus.Fields{
		"url": url,
		"key": key,
	}).Debug("Downloading file")

	// Download the file
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set a timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create a buffer to store the downloaded data for checksum verification
	var buf bytes.Buffer

	// Create a tee reader to both save to buffer and calculate checksum
	hasher := sha256.New()
	multiWriter := io.MultiWriter(&buf, hasher)

	// Download the file to memory for checksum verification
	data, err := io.ReadAll(io.TeeReader(resp.Body, multiWriter))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Verify the checksum if provided
	if expectedSHA256 != "" {
		computedSum := hex.EncodeToString(hasher.Sum(nil))
		if computedSum != expectedSHA256 {
			return nil, fmt.Errorf("checksum verification failed: expected %s, got %s",
				expectedSHA256, computedSum)
		}
	}

	// Store the file in the storage backend
	if err := h.storage.Put(context.Background(), key, bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("failed to store file: %w", err)
	}

	h.logger.WithField("key", key).Debug("Successfully downloaded and verified file")
	return data, nil
}

// Validation functions
func isValidOS(osName string) bool {
	// Common Terraform operating systems
	validOS := map[string]bool{
		"darwin":  true,
		"freebsd": true,
		"linux":   true,
		"openbsd": true,
		"solaris": true,
		"windows": true,
	}
	_, valid := validOS[osName]
	return valid
}

func isValidRegistry(registry string) bool {
	// Registry should be a valid hostname or IP address
	// For now, just check it's not empty and doesn't contain invalid characters
	if registry == "" {
		return false
	}
	// Basic check for invalid characters
	return !strings.ContainsAny(registry, " /\\")
}

func isValidNamespace(namespace string) bool {
	// Namespace should be a valid DNS label (alphanumeric and hyphens, not starting/ending with hyphen)
	if namespace == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`, namespace)
	return matched
}

func isValidProvider(provider string) bool {
	// Provider name should be alphanumeric with hyphens
	if provider == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, provider)
	return matched && !strings.HasPrefix(provider, "-") && !strings.HasSuffix(provider, "-")
}

func isValidVersion(version string) bool {
	// Version should be in semantic versioning format (e.g., 1.2.3)
	if version == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.+-]+)?(\+[a-zA-Z0-9.+-]+)?$`, version)
	return matched
}

func isValidArch(arch string) bool {
	// Common Terraform architectures
	validArches := map[string]bool{
		"386":     true,
		"amd64":   true,
		"arm":     true,
		"arm64":   true,
		"ppc64le": true,
	}
	_, valid := validArches[arch]
	return valid
}

// isBrokenPipeError checks if the error is a broken pipe error
func isBrokenPipeError(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*os.SyscallError); ok {
			return syscallErr.Err.Error() == "broken pipe" ||
				syscallErr.Err.Error() == "connection reset by peer"
		}
	}
	return false
}

// GetProviderIndex returns the provider index
func (h *RegistryHandler) GetProviderIndex(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")

	h.logger.WithFields(logrus.Fields{
		"registry":  registry,
		"namespace": namespace,
		"provider":  provider,
	}).Info("Provider index requested")

	// Validate parameters
	if !isValidRegistry(registry) || !isValidNamespace(namespace) || !isValidProvider(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	// Build the registry API URL
	baseURL := registry
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	url := fmt.Sprintf("%s/v1/providers/%s/%s/versions", baseURL, namespace, provider)

	h.logger.WithField("url", url).Debug("Fetching provider versions from registry")

	// Make the request to the registry
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create request")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	// Add Terraform user agent
	req.Header.Set("User-Agent", "Terraform/1.0.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch provider versions")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch provider versions"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		h.logger.WithFields(logrus.Fields{
			"status": resp.Status,
			"body":   string(body),
		}).Error("Unexpected response from registry")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "failed to fetch provider versions",
			"status": resp.Status,
		})
		return
	}

	// Parse the response to get the list of versions
	var versionsResp struct {
		ID       string `json:"id"`
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	}

	if err := json.Unmarshal(body, &versionsResp); err != nil {
		h.logger.WithError(err).Error("Failed to parse provider versions response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse provider versions"})
		return
	}

	// Create a map with versions as keys and empty objects as values
	versionsMap := make(map[string]struct{})
	for _, v := range versionsResp.Versions {
		versionsMap[v.Version] = struct{}{}
	}

	h.logger.WithFields(logrus.Fields{
		"provider": provider,
		"versions": len(versionsMap),
	}).Info("Returning provider versions")

	// Return the versions in the expected format with empty objects as values
	c.JSON(http.StatusOK, gin.H{
		"versions": versionsMap,
	})
}

// GetProviderVersion returns the provider version details
func (h *RegistryHandler) GetProviderVersion(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")

	// Get version from context (set by the route handler)
	versionVal, exists := c.Get("version")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version not found in context"})
		return
	}

	version, ok := versionVal.(string)
	if !ok || version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version parameter"})
		return
	}

	// Validate parameters
	if !isValidRegistry(registry) || !isValidNamespace(namespace) ||
		!isValidProvider(provider) || !isValidVersion(version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"registry":  registry,
		"namespace": namespace,
		"provider":  provider,
		"version":   version,
	}).Info("Fetching provider version details")

	// Build the registry API URL
	baseURL := registry
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	url := fmt.Sprintf("%s/v1/providers/%s/%s/versions", baseURL, namespace, provider)

	h.logger.WithField("url", url).Debug("Fetching provider versions from registry")

	// Make the request to the registry
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create request")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	// Add Terraform user agent
	req.Header.Set("User-Agent", "Terraform/1.0.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch provider versions")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch provider versions"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		h.logger.WithFields(logrus.Fields{
			"status": resp.Status,
			"body":   string(body),
		}).Error("Unexpected response from registry")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "failed to fetch provider versions",
			"status": resp.Status,
		})
		return
	}

	// Parse the response
	var versionsResp struct {
		ID       string `json:"id"`
		Versions []struct {
			Version   string   `json:"version"`
			Protocols []string `json:"protocols"`
			Platforms []struct {
				OS   string `json:"os"`
				Arch string `json:"arch"`
			} `json:"platforms"`
		} `json:"versions"`
	}

	if err := json.Unmarshal(body, &versionsResp); err != nil {
		h.logger.WithError(err).Error("Failed to parse provider versions response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse provider versions"})
		return
	}

	// Build the response with all available versions
	versions := make([]string, 0, len(versionsResp.Versions))
	for _, v := range versionsResp.Versions {
		versions = append(versions, v.Version)
	}

	// If a specific version was requested, return its details
	if version != "" && version != "index.json" {
		var foundVersion *struct {
			Version   string   `json:"version"`
			Protocols []string `json:"protocols"`
			Platforms []struct {
				OS   string `json:"os"`
				Arch string `json:"arch"`
			} `json:"platforms"`
		}

		for i, v := range versionsResp.Versions {
			if v.Version == version {
				// Create a copy of the version to avoid referencing loop variable
				ver := versionsResp.Versions[i]
				foundVersion = &ver
				break
			}
		}

		if foundVersion == nil {
			h.logger.WithField("version", version).Warn("Version not found")
			c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
			return
		}

		// Build the response for specific version
		response := VersionResponse{
			Archives: make(map[string]ArchiveInfo),
		}

		// Add each platform/arch combination to the response
		for _, platform := range foundVersion.Platforms {
			key := fmt.Sprintf("%s_%s", platform.OS, platform.Arch)
			filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip",
				provider, version, platform.OS, platform.Arch)

			response.Archives[key] = ArchiveInfo{
				URL: filename,
			}
		}

		h.logger.WithFields(logrus.Fields{
			"registry":  registry,
			"namespace": namespace,
			"provider":  provider,
			"version":   version,
		}).Info("Returning version details")

		c.JSON(http.StatusOK, response)
		return
	}

	// Return the list of versions if no specific version was requested
	c.JSON(http.StatusOK, gin.H{
		"versions": versions,
	})
}

// DownloadProvider downloads the provider binary
func (h *RegistryHandler) DownloadProvider(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")

	// Get version, os, and arch from context (set by the route handler)
	versionVal, exists := c.Get("version")
	if !exists {
		h.logger.Error("Version not found in context")
		c.JSON(http.StatusBadRequest, gin.H{"error": "version not found in context"})
		return
	}

	osVal, exists := c.Get("os")
	if !exists {
		h.logger.Error("OS not found in context")
		c.JSON(http.StatusBadRequest, gin.H{"error": "os not found in context"})
		return
	}

	archVal, exists := c.Get("arch")
	if !exists {
		h.logger.Error("Architecture not found in context")
		c.JSON(http.StatusBadRequest, gin.H{"error": "architecture not found in context"})
		return
	}

	version := versionVal.(string)
	osName := osVal.(string)
	arch := archVal.(string)

	h.logger.WithFields(logrus.Fields{
		"registry":  registry,
		"namespace": namespace,
		"provider":  provider,
		"version":   version,
		"os":        osName,
		"arch":      arch,
	}).Debug("Processing download request")

	// Validate inputs
	if !isValidRegistry(registry) || !isValidNamespace(namespace) || !isValidProvider(provider) ||
		!isValidVersion(version) || !isValidOS(osName) || !isValidArch(arch) {
		h.logger.WithFields(logrus.Fields{
			"registry":  registry,
			"namespace": namespace,
			"provider":  provider,
			"version":   version,
			"os":        osName,
			"arch":      arch,
		}).Error("Invalid parameters")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	// Construct the filename
	filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider, version, osName, arch)

	// Get the cache key
	cacheKey := h.getCacheKey(registry, namespace, provider, version, osName, arch)

	// Try to get the file directly - this will handle cache hit/miss metrics
	h.logger.WithField("key", cacheKey).Debug("Attempting to get file from cache")
	fileReader, err := h.storage.Get(c.Request.Context(), cacheKey)
	if err == nil {
		// File exists in cache, serve it
		defer fileReader.Close()
		h.logger.WithField("key", cacheKey).Info("Serving from cache")

		// Set the appropriate headers
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		// Stream the file
		_, err = io.Copy(c.Writer, fileReader)
		if err != nil && !isBrokenPipeError(err) {
			h.logger.WithError(err).Error("Failed to send file")
		}
		return
	} else if err != os.ErrNotExist {
		// Handle other errors
		h.logger.WithError(err).WithField("key", cacheKey).Error("Failed to get file from cache")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get file from cache"})
		return
	}

	// File not in cache, download it
	h.logger.WithField("key", cacheKey).Info("File not found in cache, downloading...")

	// Get the download URL from the upstream registry
	downloadURL := fmt.Sprintf("https://%s/v1/providers/%s/%s/%s/download/%s/%s",
		registry,
		namespace,
		provider,
		version,
		osName,
		arch,
	)

	h.logger.WithField("url", downloadURL).Debug("Fetching download info from upstream")

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create request")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch download info")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch download info"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		h.logger.WithFields(logrus.Fields{
			"status": resp.Status,
			"body":   string(body),
		}).Error("Unexpected response from registry")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "failed to fetch download info",
			"status": resp.Status,
		})
		return
	}

	var downloadInfo DownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&downloadInfo); err != nil {
		h.logger.WithError(err).Error("Failed to parse download info response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse download info"})
		return
	}

	// Validate download info
	if downloadInfo.DownloadURL == "" || downloadInfo.SHASum == "" {
		h.logger.Error("Missing download URL or SHA256 checksum in response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid download information"})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"download_url": downloadInfo.DownloadURL,
		"sha256":       downloadInfo.SHASum,
	}).Info("Downloading provider binary")

	// Download and store the file
	_, err = h.downloadFile(downloadInfo.DownloadURL, cacheKey, downloadInfo.SHASum)
	if err != nil {
		h.logger.WithError(err).Error("Failed to download or verify provider binary")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to download or verify provider binary",
			"details": err.Error(),
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"key":    cacheKey,
		"sha256": downloadInfo.SHASum,
	}).Info("Successfully downloaded and verified provider binary")

	// Get the file from storage
	reader, err := h.storage.Get(c.Request.Context(), cacheKey)
	if err != nil {
		h.logger.WithError(err).Error("Error getting file from storage")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error retrieving file from storage"})
		return
	}
	defer reader.Close()

	// Get file info for content length if available
	var contentLength int64 = -1
	if fi, ok := reader.(interface{ Stat() (os.FileInfo, error) }); ok {
		if stat, err := fi.Stat(); err == nil {
			contentLength = stat.Size()
		}
	}

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/zip")

	if contentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
	}

	// Use a buffer to stream the file in chunks
	buf := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if _, err := c.Writer.Write(buf[:n]); err != nil {
				if !isBrokenPipeError(err) {
					h.logger.WithError(err).Error("Error writing file chunk to response")
				}
				return
			}
			c.Writer.Flush()
		}
		if err != nil {
			if err != io.EOF {
				h.logger.WithError(err).Error("Error reading file from storage")
			}
			break
		}
	}
}
