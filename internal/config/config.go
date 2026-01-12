package config

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// LoadEnv loads the .env file from the project root.
// It uses the caller's file path to locate the project root.
func LoadEnv() {
	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)

	// Assuming internal/config/config.go, project root is 2 levels up
	root := filepath.Join(basepath, "..", "..")
	envPath := filepath.Join(root, ".env")

	if err := godotenv.Load(envPath); err != nil {
		log.Printf("No .env file found at %s, relying on system environment variables", envPath)
	}
}

// GetEnv is a helper to get an environment variable or a default value.
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetRequiredEnv returns an environment variable or logs a fatal error if missing.
func GetRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}
