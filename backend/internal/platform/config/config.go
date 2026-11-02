// Package config reads service configuration from environment variables.
//
// Every service is configured exclusively through the environment (12-factor),
// so the same image runs unchanged in docker compose and in Kubernetes. Helpers
// here return the provided default when a variable is unset or empty, and
// report malformed values as errors instead of silently falling back.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// String returns the value of key, or def when the variable is unset or empty.
func String(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// Required returns the value of key or an error when it is unset or empty.
// Use it for settings that have no safe default (DSNs, secrets).
func Required(key string) (string, error) {
	v := String(key, "")
	if v == "" {
		return "", fmt.Errorf("config: required environment variable %s is not set", key)
	}
	return v, nil
}

// Int returns key parsed as an integer, def when unset, or an error when malformed.
func Int(key string, def int) (int, error) {
	v := String(key, "")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not an integer", key, v)
	}
	return n, nil
}

// Bool returns key parsed with strconv.ParseBool, def when unset, or an error when malformed.
func Bool(key string, def bool) (bool, error) {
	v := String(key, "")
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: %s=%q is not a boolean", key, v)
	}
	return b, nil
}

// Duration returns key parsed with time.ParseDuration (e.g. "30s", "8h"),
// def when unset, or an error when malformed.
func Duration(key string, def time.Duration) (time.Duration, error) {
	v := String(key, "")
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a duration", key, v)
	}
	return d, nil
}

// List returns key split on commas with blanks removed, or def when unset.
func List(key string, def []string) []string {
	v := String(key, "")
	if v == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
