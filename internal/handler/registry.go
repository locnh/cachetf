package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	regexp "regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RegistryHandler handles Terraform registry API requests
type RegistryHandler struct {
	logger     *logrus.Logger
	httpClient *http.Client
	apiVersion string
	cacheDir   string
	mu         sync.RWMutex // Protects concurrent access to the cache
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
func NewRegistryHandler(logger *logrus.Logger, cacheDir string) *RegistryHandler {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		logger.WithError(err).WithField("cache_dir", cacheDir).Fatal("Failed to create cache directory")
	}

	handler := &RegistryHandler{
		logger:   logger,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
		apiVersion: "v1",
	}

	// Log handler initialization
	logger.WithFields(logrus.Fields{
		"api_version": handler.apiVersion,
		"cache_dir":   cacheDir,
	}).Info("Initialized new RegistryHandler")

	return handler
}

// getCachePath returns the full path to a cached file
func (h *RegistryHandler) getCachePath(registry, namespace, provider, version, platform, arch string) string {
	// Create a clean filename from the provider details
	filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider, version, platform, arch)
	// Return path in format: /cache/registry/namespace/provider/version/filename.zip
	return filepath.Join(h.cacheDir, registry, namespace, provider, version, filename)
}

// Helper function to verify SHA256 checksum of a file
func verifyChecksum(filePath, expectedSum string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum verification: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualSum := hex.EncodeToString(hasher.Sum(nil))
	if actualSum != expectedSum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSum, actualSum)
	}

	return nil
}

// Helper function to download a file and save it to the cache with checksum verification
func (h *RegistryHandler) downloadFile(url, destPath, expectedSHA256 string) error {
	tempPath := destPath + ".tmp"
	// Create the directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Remove temp file if it exists
	_ = os.Remove(tempPath)

	// Create the file
	out, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	// Get the data
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		out.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to create request: %w", err)
	}

	h.logger.WithField("url", url).Debug("Downloading file")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		out.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		out.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Close the file
	if err := out.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Verify checksum if provided
	if expectedSHA256 != "" {
		h.logger.WithFields(logrus.Fields{
			"file":     tempPath,
			"expected": expectedSHA256,
		}).Debug("Verifying checksum")

		if err := verifyChecksum(tempPath, expectedSHA256); err != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		h.logger.WithField("file", tempPath).Debug("Checksum verified successfully")
	}

	// Rename the temp file to the final name
	if err := os.Rename(tempPath, destPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	h.logger.WithFields(logrus.Fields{
		"url":  url,
		"path": destPath,
	}).Info("Successfully downloaded and verified file")

	return nil
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

func isValidPlatform(platform string) bool {
	// Common Terraform platforms
	validPlatforms := map[string]bool{
		"darwin":  true,
		"freebsd": true,
		"linux":   true,
		"openbsd": true,
		"solaris": true,
		"windows": true,
	}
	_, valid := validPlatforms[platform]
	return valid
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

// GetProviderIndex returns the provider index
func (h *RegistryHandler) GetProviderIndex(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")

	// Validate parameters
	if !isValidRegistry(registry) || !isValidNamespace(namespace) || !isValidProvider(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"registry":  registry,
		"namespace": namespace,
		"provider":  provider,
	}).Info("Provider index requested")

	// Build the registry API URL
	url := fmt.Sprintf("%s/%s/providers/%s/%s", registry, h.apiVersion, namespace, provider)
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	h.logger.WithField("url", url).Debug("Fetching provider versions from registry")

	// Make the request to the registry
	resp, err := h.httpClient.Get(url)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch provider versions")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch provider versions"})
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
			"error":  "failed to fetch provider versions",
			"status": resp.Status,
		})
		return
	}

	// Parse the response
	var providerResp ProviderResponse
	if err := json.NewDecoder(resp.Body).Decode(&providerResp); err != nil {
		h.logger.WithError(err).Error("Failed to parse provider versions response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse provider versions"})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"provider_id": providerResp.ID,
		"versions":    len(providerResp.Versions),
	}).Debug("Fetched provider versions")

	// Convert versions to the required format
	versions := make(map[string]struct{})
	for _, v := range providerResp.Versions {
		versions[v] = struct{}{}
	}

	// Return the response in the required format
	c.JSON(http.StatusOK, &IndexResponse{
		Versions: versions,
	})
}

// GetProviderVersion returns the provider version details
func (h *RegistryHandler) GetProviderVersion(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")

	// Get version from context
	versionVal, exists := c.Get("version")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version not found in context"})
		return
	}

	// Convert version to string
	version, ok := versionVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid version format"})
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
	url := fmt.Sprintf("%s/%s/providers/%s/%s/versions", baseURL, h.apiVersion, namespace, provider)

	h.logger.WithField("url", url).Debug("Fetching provider versions from registry")

	// Make the request to the registry
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create request")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).Error("Failed to fetch provider versions")
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch provider versions"})
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
			"error":  "failed to fetch provider versions",
			"status": resp.Status,
		})
		return
	}

	// Parse the response
	var versionsResp ProviderVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionsResp); err != nil {
		h.logger.WithError(err).Error("Failed to parse provider versions response")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse provider versions"})
		return
	}

	// Find the requested version
	var foundVersion *struct {
		Version   string   `json:"version"`
		Protocols []string `json:"protocols"`
		Platforms []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"platforms"`
	}

	for _, v := range versionsResp.Versions {
		if v.Version == version {
			foundVersion = &v
			break
		}
	}

	if foundVersion == nil {
		h.logger.WithField("version", version).Warn("Version not found")
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	// Build the response
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
		"version":  version,
		"archives": len(response.Archives),
	}).Info("Returning version details")

	c.JSON(http.StatusOK, response)
}

// DownloadProvider downloads the provider binary
func (h *RegistryHandler) DownloadProvider(c *gin.Context) {
	registry := c.Param("registry")
	namespace := c.Param("namespace")
	provider := c.Param("provider")
	file := c.Param("file") // Full filename including .zip

	// Parse the filename to extract provider, version, os, and architecture
	re := regexp.MustCompile(`^terraform-provider-([a-z0-9-]+)_(\d+\.\d+\.\d+)_([a-z]+)_([a-z0-9]+)\.zip$`)
	matches := re.FindStringSubmatch(file)
	if len(matches) != 5 { // full match + 4 groups
		h.logger.WithField("file", file).Error("Invalid file format")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file format"})
		return
	}

	// Extract components from filename
	// matches[1] is the provider name (should match the one in the URL)
	// matches[2] is the version
	// matches[3] is the OS
	// matches[4] is the architecture
	version := matches[2]
	osName := matches[3]
	arch := matches[4]

	// Verify provider name in URL matches the one in the filename
	if matches[1] != provider {
		h.logger.WithFields(logrus.Fields{
			"url_provider":      provider,
			"filename_provider": matches[1],
		}).Error("Provider name in URL does not match filename")
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider name mismatch"})
		return
	}

	// Validate parameters
	if !isValidRegistry(registry) || !isValidNamespace(namespace) ||
		!isValidProvider(provider) || !isValidVersion(version) ||
		!isValidOS(osName) || !isValidArch(arch) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	logFields := logrus.Fields{
		"registry":  registry,
		"namespace": namespace,
		"provider":  provider,
		"version":   version,
		"os":        osName,
		"arch":      arch,
	}
	h.logger.WithFields(logFields).Info("Provider download requested")

	// Ensure the cache directory exists
	cacheDir := filepath.Dir(h.getCachePath(registry, namespace, provider, version, osName, arch))
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		h.logger.WithError(err).Error("Failed to create cache directory")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	// Check if the file is already in the cache
	cachePath := h.getCachePath(registry, namespace, provider, version, osName, arch)

	// Check if the file exists and is not empty
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		h.logger.WithFields(logFields).WithField("cache_path", cachePath).
			Debug("Serving provider binary from cache")
	} else {
		// Download the file
		h.mu.Lock()
		defer h.mu.Unlock()

		// Check again in case another goroutine downloaded it while we were waiting for the lock
		if _, err := os.Stat(cachePath); err != nil && os.IsNotExist(err) {
			// Build the download info URL
			downloadInfoURL := fmt.Sprintf("%s/%s/providers/%s/%s/%s/download/%s/%s",
				strings.TrimSuffix(registry, "/"),
				strings.Trim(h.apiVersion, "/"),
				namespace,
				provider,
				version,
				osName,
				arch)

			// Ensure the URL has the correct scheme
			if !strings.HasPrefix(downloadInfoURL, "http") {
				downloadInfoURL = "https://" + downloadInfoURL
			}

			h.logger.WithFields(logFields).
				WithField("download_info_url", downloadInfoURL).
				Info("Fetching download information")

			// Get the download information
			req, err := http.NewRequest("GET", downloadInfoURL, nil)
			if err != nil {
				h.logger.WithError(err).Error("Failed to create download info request")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}

			resp, err := h.httpClient.Do(req)
			if err != nil {
				h.logger.WithError(err).Error("Failed to fetch download info")
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch download information"})
				return
			}
			defer resp.Body.Close()

			// Read the response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				h.logger.WithError(err).Error("Failed to read response body")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response"})
				return
			}

			if resp.StatusCode != http.StatusOK {
				h.logger.WithFields(logrus.Fields{
					"status": resp.Status,
					"body":   string(body),
				}).Error("Unexpected response from registry for download info")
				c.JSON(http.StatusBadGateway, gin.H{
					"error":  "failed to fetch download information",
					"status": resp.Status,
				})
				return
			}

			// Parse the download information
			var downloadInfo struct {
				DownloadURL string   `json:"download_url"`
				SHASum      string   `json:"shasum"`
				Protocols   []string `json:"protocols"`
			}

			if err := json.Unmarshal(body, &downloadInfo); err != nil {
				h.logger.WithError(err).Error("Failed to parse download info response")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse download information"})
				return
			}

			if downloadInfo.DownloadURL == "" || downloadInfo.SHASum == "" {
				h.logger.Error("Missing download URL or SHA256 checksum in response")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid download information"})
				return
			}

			h.logger.WithFields(logrus.Fields{
				"download_url": downloadInfo.DownloadURL,
				"sha256":       downloadInfo.SHASum,
			}).Info("Downloading provider binary")

			// Download the file to a temporary location first
			tmpPath := cachePath + ".tmp"
			if err := h.downloadFile(downloadInfo.DownloadURL, tmpPath, downloadInfo.SHASum); err != nil {
				// Clean up the temporary file if it exists
				if _, statErr := os.Stat(tmpPath); statErr == nil {
					os.Remove(tmpPath)
				}
				h.logger.WithError(err).Error("Failed to download or verify provider binary")
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to download or verify provider binary",
					"details": err.Error(),
				})
				return
			}

			// Rename the temporary file to the final location atomically
			if err := os.Rename(tmpPath, cachePath); err != nil {
				h.logger.WithError(err).Error("Failed to move downloaded file to cache")
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to cache downloaded file",
					"details": err.Error(),
				})
				return
			}

			h.logger.WithFields(logrus.Fields{
				"cache_path": cachePath,
				"sha256":     downloadInfo.SHASum,
			}).Info("Successfully downloaded and verified provider binary")
		} else if err != nil {
			h.logger.WithError(err).Error("Error checking cache file")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error checking cache"})
			return
		}
	}

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(file)))
	c.Header("Content-Type", "application/octet-stream")

	// Serve the file
	c.File(cachePath)
}
