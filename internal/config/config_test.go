package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Save current env vars
	oldPort := os.Getenv("PORT")
	oldMetricsPort := os.Getenv("METRICS_PORT")
	oldStorageType := os.Getenv("STORAGE_TYPE")
	oldS3Bucket := os.Getenv("S3_BUCKET")

	// Set default values for required fields
	os.Setenv("PORT", "8080")
	os.Setenv("METRICS_PORT", "9100")
	// Clear other environment variables that might affect the test
	os.Unsetenv("STORAGE_TYPE")
	os.Unsetenv("S3_BUCKET")

	// Restore env vars after test
	defer func() {
		os.Setenv("PORT", oldPort)
		os.Setenv("METRICS_PORT", oldMetricsPort)
		os.Setenv("STORAGE_TYPE", oldStorageType)
		os.Setenv("S3_BUCKET", oldS3Bucket)
	}()

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.ServerPort)
	assert.Equal(t, 9100, cfg.MetricsPort)
	assert.Equal(t, "/providers", cfg.URIPrefix)
	assert.Equal(t, StorageTypeLocal, cfg.StorageType)
	assert.Equal(t, "./cache", cfg.CacheDir)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.True(t, cfg.IsLocal())
	assert.False(t, cfg.IsS3())
}

func TestLoadConfig_EnvironmentOverrides(t *testing.T) {
	// Save current env vars
	oldPort := os.Getenv("PORT")
	oldMetricsPort := os.Getenv("METRICS_PORT")
	oldURIPrefix := os.Getenv("URI_PREFIX")
	oldStorageType := os.Getenv("STORAGE_TYPE")
	oldCacheDir := os.Getenv("CACHE_DIR")
	oldLogLevel := os.Getenv("LOG_LEVEL")
	oldS3Bucket := os.Getenv("S3_BUCKET")
	oldS3Region := os.Getenv("S3_REGION")

	// Set test env vars
	os.Setenv("PORT", "3000")
	os.Setenv("METRICS_PORT", "9000")
	os.Setenv("URI_PREFIX", "/custom")
	os.Setenv("STORAGE_TYPE", "s3")
	os.Setenv("CACHE_DIR", "/tmp/cache")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("S3_BUCKET", "my-bucket")
	os.Setenv("S3_REGION", "us-west-2")

	// Restore env vars after test
	defer func() {
		os.Setenv("PORT", oldPort)
		os.Setenv("METRICS_PORT", oldMetricsPort)
		os.Setenv("URI_PREFIX", oldURIPrefix)
		os.Setenv("STORAGE_TYPE", oldStorageType)
		os.Setenv("CACHE_DIR", oldCacheDir)
		os.Setenv("LOG_LEVEL", oldLogLevel)
		os.Setenv("S3_BUCKET", oldS3Bucket)
		os.Setenv("S3_REGION", oldS3Region)
	}()

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 3000, cfg.ServerPort)
	assert.Equal(t, 9000, cfg.MetricsPort)
	assert.Equal(t, "/custom", cfg.URIPrefix)
	assert.Equal(t, StorageTypeS3, cfg.StorageType)
	assert.Equal(t, "/tmp/cache", cfg.CacheDir)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "my-bucket", cfg.S3.Bucket)
	assert.Equal(t, "us-west-2", cfg.S3.Region)
	assert.True(t, cfg.IsS3())
	assert.False(t, cfg.IsLocal())
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	// Save current env vars
	oldPort := os.Getenv("PORT")
	oldMetricsPort := os.Getenv("METRICS_PORT")
	oldStorageType := os.Getenv("STORAGE_TYPE")

	// Set up test environment
	os.Setenv("PORT", "99999")        // Invalid port number
	os.Setenv("METRICS_PORT", "9100")  // Valid metrics port
	os.Setenv("STORAGE_TYPE", "local") // Valid storage type

	// Clean up
	defer func() {
		os.Setenv("PORT", oldPort)
		os.Setenv("METRICS_PORT", oldMetricsPort)
		os.Setenv("STORAGE_TYPE", oldStorageType)
	}()

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PORT")
}

func TestLoadConfig_InvalidStorageType(t *testing.T) {
	// Set required environment variables first
	os.Setenv("PORT", "8080")
	os.Setenv("METRICS_PORT", "9100")
	// Set invalid storage type
	os.Setenv("STORAGE_TYPE", "invalid")
	defer os.Unsetenv("STORAGE_TYPE")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid STORAGE_TYPE")
}

func TestS3Config_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  S3Config
		hasErr  bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  S3Config{Bucket: "my-bucket", Region: "us-west-2"},
			hasErr:  false,
		},
		{
			name:    "missing bucket",
			config:  S3Config{Region: "us-west-2"},
			hasErr:  true,
			errMsg:  "S3_BUCKET is required",
		},
		{
			name:    "missing region",
			config:  S3Config{Bucket: "my-bucket"},
			hasErr:  true,
			errMsg:  "S3_REGION is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.hasErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadConfig_FromDotEnv(t *testing.T) {
	// Create a temporary .env file
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	envContent := `
PORT=3001
METRICS_PORT=9001
STORAGE_TYPE=s3
S3_BUCKET=test-bucket
S3_REGION=eu-west-1
`
	err := os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	// Change to the temp directory and reset the working directory after the test
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(oldWd) })
	os.Chdir(dir)

	// Clear any environment variables that might affect the test
	os.Unsetenv("PORT")
	os.Unsetenv("METRICS_PORT")
	os.Unsetenv("STORAGE_TYPE")
	os.Unsetenv("S3_BUCKET")
	os.Unsetenv("S3_REGION")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 3001, cfg.ServerPort)
	assert.Equal(t, 9001, cfg.MetricsPort)
	assert.Equal(t, StorageTypeS3, cfg.StorageType)
	assert.Equal(t, "test-bucket", cfg.S3.Bucket)
	assert.Equal(t, "eu-west-1", cfg.S3.Region)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr string
	}{
		{
			name: "valid local config",
			config: &Config{
				ServerPort:  8080,
				MetricsPort: 9100,
				StorageType: StorageTypeLocal,
			},
			wantErr: "",
		},
		{
			name: "valid s3 config",
			config: &Config{
				ServerPort:  8080,
				MetricsPort: 9100,
				StorageType: StorageTypeS3,
				S3: S3Config{
					Bucket: "my-bucket",
					Region: "us-west-2",
				},
			},
			wantErr: "",
		},
		{
			name: "invalid port",
			config: &Config{
				ServerPort: 0, // Invalid port
				MetricsPort: 9100,
				StorageType: StorageTypeLocal,
			},
			wantErr: "invalid PORT",
		},
		{
			name: "invalid storage type",
			config: &Config{
				ServerPort:  8080,
				MetricsPort: 9100,
				StorageType: "invalid",
			},
			wantErr: "invalid STORAGE_TYPE",
		},
		{
			name: "missing s3 bucket",
			config: &Config{
				ServerPort:  8080,
				MetricsPort: 9100,
				StorageType: StorageTypeS3,
				S3: S3Config{
					// Missing bucket
					Region: "us-west-2",
				},
			},
			wantErr: "S3_BUCKET is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
