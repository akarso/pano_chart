package scoring

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// envLoaded tracks whether .env has been loaded to avoid repeated file reads.
var envLoaded bool

// LoadEnv loads .env once. Safe to call multiple times.
func LoadEnv() {
	if !envLoaded {
		_ = godotenv.Load()
		envLoaded = true
	}
}

// EnvInt reads an integer from the environment, returning def if absent or unparseable.
func EnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

// EnvFloat reads a float64 from the environment, returning def if absent or unparseable.
func EnvFloat(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
