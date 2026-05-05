package app

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	SQLiteDSN         string
	HoldDuration      time.Duration
	ExpirySweepPeriod time.Duration
	SeedFile          string
}

func LoadConfig() Config {
	return Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		SQLiteDSN:         getenv("SQLITE_DSN", "file:data/inventory.db?_pragma=busy_timeout(5000)"),
		HoldDuration:      durationEnv("HOLD_DURATION", 2*time.Minute),
		ExpirySweepPeriod: durationEnv("EXPIRY_SWEEP_PERIOD", 5*time.Second),
		SeedFile:          getenv("SEED_FILE", "seeds/products.json"),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
