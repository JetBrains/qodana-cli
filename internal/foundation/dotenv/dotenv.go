// Package dotenv reads .env files into a map without mutating the process environment,
// wrapping github.com/subosito/gotenv. Build-time scripts use it to pick up local tokens.
package dotenv

import (
	"errors"
	"os"

	"github.com/subosito/gotenv"
)

// Read parses the env file at path into a map, without mutating the process environment. A missing
// file yields an empty map (not an error), so callers can treat "no .env" as "no overrides".
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
