package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// StorageType defines the type of storage to use
type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeS3    StorageType = "s3"
)

// S3Config holds S3 storage configuration
type S3Config struct {
	Bucket  string `env:"S3_BUCKET"`
	Region  string `env:"S3_REGION" envDefault:"eu-central-1"`
	RoleARN string `env:"S3_ROLE_ARN"`
}

// Validate checks if the S3 configuration is valid
func (c *S3Config) Validate() error {
	if c.Bucket == "" {
		return fmt.Errorf("S3_BUCKET is required when using S3 storage")
	}
	if c.Region == "" {
		return fmt.Errorf("S3_REGION is required when using S3 storage")
	}
	return nil
}

// Config holds the application configuration
type Config struct {
	ServerPort  int         `env:"PORT" envDefault:"8080"`
	URIPrefix   string      `env:"URI_PREFIX" envDefault:"/providers"`
	StorageType StorageType `env:"STORAGE_TYPE" envDefault:"local"`
	CacheDir    string      `env:"CACHE_DIR" envDefault:"./cache"`
	LogLevel    string      `env:"LOG_LEVEL" envDefault:"info"`
	S3          S3Config
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("invalid PORT: must be between 1 and 65535")
	}

	if c.StorageType == StorageTypeS3 {
		if err := c.S3.Validate(); err != nil {
			return fmt.Errorf("invalid S3 configuration: %w", err)
		}
	} else if c.StorageType != StorageTypeLocal {
		return fmt.Errorf("invalid STORAGE_TYPE: must be 'local' or 's3'")
	}

	return nil
}

// IsS3 returns true if the storage type is S3
func (c *Config) IsS3() bool {
	return c.StorageType == StorageTypeS3
}

// IsLocal returns true if the storage type is local filesystem
func (c *Config) IsLocal() bool {
	return c.StorageType == StorageTypeLocal
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists (ignoring errors as .env is optional)
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	// Load basic configuration
	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT value: %w", err)
	}

	storageType := StorageType(getEnv("STORAGE_TYPE", "local"))
	if storageType != StorageTypeLocal && storageType != StorageTypeS3 {
		return nil, fmt.Errorf("invalid STORAGE_TYPE: must be 'local' or 's3'")
	}

	// Create config instance
	cfg := &Config{
		ServerPort:  port,
		URIPrefix:   getEnv("URI_PREFIX", "/providers"),
		StorageType: storageType,
		CacheDir:    getEnv("CACHE_DIR", "./cache"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		S3: S3Config{
			Bucket:  getEnv("S3_BUCKET", ""),
			Region:  getEnv("S3_REGION", "eu-central-1"),
			RoleARN: getEnv("S3_ROLE_ARN", ""),
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
