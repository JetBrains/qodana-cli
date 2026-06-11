//go:build ignore

// Downloads and sha256-verifies the pinned clang-tidy archive for this build target from the
// versioned, auth-only JB Space Files repo, writing it next to clang/clang-tidy.json for the
// subsequent prepare-clang-tidy-binary.go + //go:embed steps. Run via `go generate ./clang/...`.
//
// Honors TARGETOS/TARGETARCH (set per target by the goreleaser cross-build) over runtime.GOOS/ARCH,
// matching prepare-clang-tidy-binary.go's archive naming so the downloaded file is the one it hashes.
// Env (also read from a repo-root .env):
//
//	QODANA_CLI_DEPS_TOKEN  Space bearer token. Empty => write a 0-byte placeholder so the build still
//	                       compiles without the closed-source archive (external contributors).
//	QODANA_CLI_DEPS_FORCE  Re-download and rewrite the pin's hash(es).
//	QODANA_CLI_DEPS_ALL    With FORCE, refresh every platform's hash, not just this target's.
//
// See QD-14839.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

const (
	pinFile  = "clang-tidy.json"
	tokenEnv = "QODANA_CLI_DEPS_TOKEN"
)

type pin struct {
	Version string            `json:"version"`
	URL     string            `json:"url"`
	Sha256  map[string]string `json:"sha256"`
}

func (p pin) resolveURL(filename string) string {
	return strings.NewReplacer("$version", p.Version, "$filename", filename).Replace(p.URL)
}

func main() {
	env := loadDotenv()
	token := resolveEnv(tokenEnv, env)
	force := truthy(resolveEnv("QODANA_CLI_DEPS_FORCE", env))
	all := truthy(resolveEnv("QODANA_CLI_DEPS_ALL", env))

	p := readPin()

	// Which archives to fetch: every platform under --all (hash refresh), else just this target's.
	var selected []string
	if all {
		for f := range p.Sha256 {
			selected = append(selected, f)
		}
		sort.Strings(selected)
	} else {
		selected = []string{targetArchive()}
	}

	// A partial --force refresh (one platform) of the multi-platform clang-tidy pin would rewrite
	// only the host's hash and silently leave the others stale; warn so a maintainer adds --all.
	if force && !all && len(selected) < len(p.Sha256) {
		fmt.Fprintf(os.Stderr, "download-clang-tidy: refreshing only %d of %d platform hashes; set QODANA_CLI_DEPS_ALL=1 to refresh all\n", len(selected), len(p.Sha256))
	}

	if token == "" {
		if force {
			log.Fatalf("QODANA_CLI_DEPS_FORCE is set but %s is empty: the auth-only repo cannot be refreshed", tokenEnv)
		}
		for _, f := range selected {
			placeholder(f)
		}
		fmt.Fprintf(os.Stderr, "download-clang-tidy: no %s set, wrote %d placeholder(s)\n", tokenEnv, len(selected))
		return
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	changed := false
	for _, f := range selected {
		expected, ok := p.Sha256[f]
		if !ok {
			log.Fatalf("%s: %q is not pinned", pinFile, f)
		}
		got := fetch(client, p.resolveURL(f), token, f, expected, force)
		if force {
			p.Sha256[f] = got
			changed = true
		}
	}
	if force && changed {
		writePin(p)
	}
}

// targetArchive is the archive filename for this build target, mirroring prepare-clang-tidy-binary.go.
func targetArchive() string {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if v := os.Getenv("TARGETOS"); v != "" {
		goos = v
	}
	if v := os.Getenv("TARGETARCH"); v != "" {
		goarch = v
	}
	name := fmt.Sprintf("clang-tidy-%s-%s", goos, goarch)
	if goos == "windows" {
		return name + ".zip"
	}
	return name + ".tar.gz"
}

// fetch ensures filename holds the pinned bytes and returns its hex sha256. Normal mode: a cache hit
// (existing file already matches the pin) skips the download, and a post-download mismatch is fatal
// (the bad file is removed). Force mode: always downloads and returns the actual hash to re-pin.
func fetch(client *http.Client, url, token, filename, expected string, force bool) string {
	if !force && expected != "" {
		if h, err := hash.GetFileSha256(filename); err == nil && hex.EncodeToString(h[:]) == expected {
			return expected
		}
	}
	got := download(client, url, token, filename)
	if force {
		return got
	}
	if got != expected {
		_ = os.Remove(filename)
		log.Fatalf("%s: sha256 mismatch: got %s, pinned %s (set QODANA_CLI_DEPS_FORCE=1 to refresh)", filename, got, expected)
	}
	return got
}

// download GETs url (optional Bearer token), writes it to destPath atomically, and returns the hex
// sha256 of the bytes. A non-200 or empty body is fatal and destPath is left untouched (the atomic
// writer is aborted, never committing a truncated file).
func download(client *http.Client, url, token, destPath string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("get %s: %s", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Fatalf("get %s: HTTP %d: %s", url, resp.StatusCode, body)
	}

	w, err := fs.CreateAtomic(destPath, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		log.Fatal(errors.Join(fmt.Errorf("download %s: %w", url, err), w.Abort()))
	}
	if n == 0 {
		log.Fatal(errors.Join(fmt.Errorf("download %s: empty response", url), w.Abort()))
	}
	if err := w.Close(); err != nil { // commit
		log.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// placeholder creates a 0-byte file if absent so //go:embed and the build still work without the
// closed-source archive. An existing (possibly real) archive is left untouched.
func placeholder(name string) {
	if _, err := os.Stat(name); err == nil {
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func readPin() pin {
	data, err := os.ReadFile(pinFile)
	if err != nil {
		log.Fatal(err)
	}
	var p pin
	if err := json.Unmarshal(data, &p); err != nil {
		log.Fatalf("parse %s: %s", pinFile, err)
	}
	// Pin keys become filenames written next to the pin; reject anything that could escape the dir.
	for name := range p.Sha256 {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			log.Fatalf("%s: invalid file name %q in sha256 (no path separators allowed)", pinFile, name)
		}
	}
	return p
}

func writePin(p pin) {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := fs.WriteFileAtomic(pinFile, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

// loadDotenv parses a repo-root .env (walked up from CWD to the dir holding go.mod) into a map
// without mutating the process environment. A missing file or root yields an empty map.
func loadDotenv() map[string]string {
	out := map[string]string{}
	dir, err := os.Getwd()
	if err != nil {
		return out
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
	}
	f, err := os.Open(filepath.Join(dir, ".env"))
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

// resolveEnv prefers the real environment over .env; an explicitly-set var wins even when empty.
func resolveEnv(key string, env map[string]string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return env[key]
}

func truthy(v string) bool { return v == "1" || strings.EqualFold(v, "true") }
