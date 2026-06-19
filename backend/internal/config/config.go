package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from the environment.
type Config struct {
	FinnhubAPIKey string
	DatabaseURL   string
	RedisURL      string
	KafkaBrokers  string
	ServerPort    string
	// EnableDevEndpoints exposes local-only debug routes (e.g. anomaly simulation).
	// Must stay false in production so debug operations are never publicly reachable.
	EnableDevEndpoints bool
}

// Load reads configuration from a .env file (if present) and the environment.
// It never logs secret values — only which keys are missing.
func Load() *Config {
	// .env is optional; in containers the values come from the environment directly.
	if err := godotenv.Load("../.env", ".env"); err != nil {
		log.Println("config: no .env file loaded, relying on environment variables")
	}

	cfg := &Config{
		FinnhubAPIKey: os.Getenv("FINNHUB_API_KEY"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		KafkaBrokers:  os.Getenv("KAFKA_BROKERS"),
		ServerPort:    getOrDefault("SERVER_PORT", "8080"),

		EnableDevEndpoints: os.Getenv("ENABLE_DEV_ENDPOINTS") == "true",
	}

	// Warn on missing required values without ever printing their contents.
	if cfg.FinnhubAPIKey == "" {
		log.Println("config: warning — FINNHUB_API_KEY is not set")
	}
	if cfg.DatabaseURL == "" {
		log.Println("config: warning — DATABASE_URL is not set")
	}

	return cfg
}

func getOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
