// Package dotenv reads .env files into a map without mutating the process environment,
// wrapping github.com/subosito/gotenv. Build-time scripts use it to pick up local tokens.
package dotenv

import (
	"errors"
	"os"

	"github.com/subosito/gotenv"
)

// Read parses the env file at path into a map. A missing file yields an empty map (not an error),
// so callers can treat "no .env" as "no overrides". It never mutates the process environment
// (gotenv.Read only opens and parses; only gotenv.Load/Apply call os.Setenv). gotenv.Read returns
// the raw os.Open error for a missing file, which wraps os.ErrNotExist.
func Read(path string) (map[string]string, error) {
	env, err := gotenv.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return env, nil
}

// Value returns the value for key, preferring the real environment over the file map. An
// explicitly-set environment variable wins even when empty (so `KEY= cmd` forces an override).
func Value(key string, fileEnv map[string]string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fileEnv[key]
}
