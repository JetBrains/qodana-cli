package downloaddeps

import (
	"bufio"
	"os"
	"strings"
)

// loadEnv parses a .env file into a map. It never mutates the process environment, so importing
// this package has no global side effects. A missing file yields an empty map (not an error).
func loadEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// resolveEnv returns the value for key, preferring the real environment over the .env file.
func resolveEnv(key string, envFile map[string]string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return envFile[key]
}
