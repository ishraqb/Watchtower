// Package config loads runtime settings from the environment (and a .env file
// during local dev). Secrets only ever live here, never in the logs.
package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from the environment.
type Config struct {
	FinnhubAPIKey string
	DatabaseURL   string
	RedisURL      string
	KafkaBrokers  string
	ServerPort    string
	// Broker selects the message broker: "kafka" (local/dev, the default) or
	// "redis" (Redis Streams, used on free hosting where Kafka isn't available).
	Broker string
	// WorkerWakeURL, if set, is pinged after publishing an anomaly so a
	// spun-down (free-tier) sentiment worker wakes up to process it.
	WorkerWakeURL string
	// AllowedOrigins is the list of browser origins allowed to call the API and
	// open a websocket. Defaults to the local dev/preview servers; in production
	// set ALLOWED_ORIGINS to your real frontend origin(s) so localhost isn't trusted.
	AllowedOrigins []string
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
		// Hosts like Render inject PORT; fall back to SERVER_PORT, then 8080 locally.
		ServerPort:    getOrDefault("PORT", getOrDefault("SERVER_PORT", "8080")),
		Broker:        getOrDefault("BROKER", "kafka"),
		WorkerWakeURL: os.Getenv("WORKER_WAKE_URL"),

		AllowedOrigins:     splitCSV(getOrDefault("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:4173")),
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

// splitCSV turns a comma-separated env value into a trimmed, non-empty slice.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
