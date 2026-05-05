package app

import (
	"fmt"
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
	httpAddr := getenv("HTTP_ADDR", ":8080")
	if port := os.Getenv("PORT"); port != "" {
		httpAddr = fmt.Sprintf(":%s", port)
	}

	sqliteDSN := getenv("SQLITE_DSN", "file:data/inventory.db?_pragma=busy_timeout(5000)")
	if os.Getenv("DYNO") != "" && os.Getenv("SQLITE_DSN") == "" {
		sqliteDSN = "file:/tmp/inventory.db?_pragma=busy_timeout(5000)"
	}

	return Config{
		HTTPAddr:          httpAddr,
		SQLiteDSN:         sqliteDSN,
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
