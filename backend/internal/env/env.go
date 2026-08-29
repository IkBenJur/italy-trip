package env

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

func GetEnvOrErr(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}

	slog.Error("Env variable not found", "key", key)
	return "", fmt.Errorf("failed to get key")
}

func GetEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	slog.Warn("Env variable not found, resolving to default", "key", key, "default", defaultValue)
	return defaultValue
}

func GetEnvInt(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	slog.Warn("Env variable not found, resolving to default", "key", key, "default", defaultValue)
	return defaultValue
}

func GetEnvInt64(key string, defaultValue int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
		slog.Warn("Env variable is not a valid integer, resolving to default", "key", key, "default", defaultValue)
	}
	slog.Warn("Env variable not found, resolving to default", "key", key, "default", defaultValue)
	return defaultValue
}

func GetEnvBool(key string, defaultValue bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	slog.Warn("Env variable not found, resolving to default", "key", key, "default", defaultValue)
	return defaultValue
}

// MustGet returns the value of key, or aborts the process if it is unset or blank.
// Used for secrets and event configuration where a silent default is worse than
// not booting at all.
func MustGet(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		fatal(fmt.Sprintf("required env variable %s is not set", key))
	}
	return value
}

// ParseTime parses an RFC3339 timestamp. An offset is required: a bare date such
// as "2026-09-14" is rejected rather than silently read as UTC midnight.
func ParseTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp %q is not RFC3339 (want e.g. 2026-09-14T23:59:59+02:00): %w", trimmed, err)
	}

	return parsed, nil
}

// MustTime reads key and parses it as RFC3339, aborting the process on failure.
// The unlock moment for the whole app hangs off this value, so a malformed one
// must never boot.
func MustTime(key string) time.Time {
	parsed, err := ParseTime(MustGet(key))
	if err != nil {
		fatal(fmt.Sprintf("env variable %s: %v", key, err))
	}
	return parsed
}

// fatal is a var so tests can swap it for a panic they can recover from.
var fatal = func(message string) {
	slog.Error("Fatal configuration error", "error", message)
	fmt.Fprintf(os.Stderr, "FATAL: %s\n", message)
	os.Exit(1)
}
