package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	ServerPort string
	URIPrefix  string // Base path for all API endpoints
	CacheDir   string // Directory to store cached provider binaries
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Set default values
	cwd, _ := os.Getwd()
	config := &Config{
		ServerPort: getEnv("PORT", "8080"),
		URIPrefix:  getEnv("URI_PREFIX", "/providers"),
		CacheDir:   getEnv("CACHE_DIR", filepath.Join(cwd, "cache")),
	}

	return config, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
