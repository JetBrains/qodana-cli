// Package downloaddeps fetches the closed-source third-party linter archives (clang-tidy, cdnet)
// from the versioned JB Space Files repo and verifies them against the committed pin files. It is
// invoked at build time via `go generate` through the scripts/download-deps.go shim. See QD-14839.
package downloaddeps

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const tokenEnv = "QODANA_CLI_DEPS_TOKEN"

// Main downloads (or, without a token, stubs out) the files for a single dependency pin file,
// resolving the token and force/all flags from the environment and the repo-root .env.
func Main(configName string) error {
	if configName == "" || configName == "." || configName == ".." || strings.ContainsAny(configName, `/\`) {
		return fmt.Errorf("invalid dependency name %q", configName)
	}

	start, err := os.Getwd()
	if err != nil {
		return err
	}
	// go generate runs each directive with the CWD set to its package dir (e.g. clang/), so walk
	// up to the module root to resolve the pin file and destination paths.
	root, err := findRepoRoot(start)
	if err != nil {
		return err
	}

	env := loadEnv(filepath.Join(root, ".env"))
	token := resolveEnv(tokenEnv, env)
	force := truthy(resolveEnv("QODANA_CLI_DEPS_FORCE", env))
	all := truthy(resolveEnv("QODANA_CLI_DEPS_ALL", env))
	goos, goarch := resolveTarget()

	client := &http.Client{Timeout: 5 * time.Minute}
	return run(client, root, configName, goos, goarch, token, force, all)
}

// run resolves the pin file, selects the files relevant to goos/goarch, and either downloads and
// verifies them (token present), refreshes their pinned hashes (force), or writes empty placeholders
// so the build still compiles without the closed-source archives (no token).
func run(client *http.Client, root, configName, goos, goarch, token string, force, all bool) error {
	cfgPath := filepath.Join(root, "scripts", "downloaddeps", configName+".json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return err
	}
	if cfg.DestDir == "" || filepath.IsAbs(cfg.DestDir) || strings.Contains(cfg.DestDir, "..") {
		return fmt.Errorf("%s: dest_dir %q must be a relative path under the repo root", configName, cfg.DestDir)
	}

	names := make([]string, 0, len(cfg.Sha256))
	for f := range cfg.Sha256 {
		names = append(names, f)
	}
	sort.Strings(names)
	selected := selectFiles(names, goos, goarch, all)

	if token == "" {
		if force {
			return fmt.Errorf("QODANA_CLI_DEPS_FORCE is set but %s is empty: hashes can only be refreshed from the auth-only repo", tokenEnv)
		}
		for _, f := range selected {
			if err := writePlaceholder(filepath.Join(root, cfg.DestDir, f)); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "downloaddeps: %s: no %s set, wrote %d placeholder(s) (mock mode)\n", configName, tokenEnv, len(selected))
		return nil
	}

	changed := false
	for _, f := range selected {
		dest := filepath.Join(root, cfg.DestDir, f)
		sha, err := processFile(client, cfg, f, token, dest, force)
		if err != nil {
			return err
		}
		if force {
			cfg.Sha256[f] = sha
			changed = true
		}
	}
	if force && changed {
		return writeConfig(cfgPath, cfg)
	}
	return nil
}

// writePlaceholder creates an empty file when one does not already exist, mirroring the `touch` the
// mock-dependencies action performs. An existing (possibly real) archive is left untouched.
func writePlaceholder(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// resolveTarget mirrors prepare-clang-tidy-binary.go: the goreleaser cross-build sets TARGETOS/
// TARGETARCH per target, and the downloaded archive must match the one that script then hashes.
func resolveTarget() (goos, goarch string) {
	goos, goarch = runtime.GOOS, runtime.GOARCH
	if v := os.Getenv("TARGETOS"); v != "" {
		goos = v
	}
	if v := os.Getenv("TARGETARCH"); v != "" {
		goarch = v
	}
	return goos, goarch
}

// findRepoRoot walks up from start until it finds the directory holding go.mod.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (no go.mod) above %s", start)
		}
		dir = parent
	}
}

// truthy reports whether an env value enables a flag: "1" or "true" (any case).
func truthy(v string) bool {
	return v == "1" || strings.EqualFold(v, "true")
}
