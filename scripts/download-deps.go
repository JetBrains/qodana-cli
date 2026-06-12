//go:build ignore

// Downloads and sha256-verifies a pinned third-party linter dependency's archive(s) from the
// versioned, auth-only JB Space Files repo, writing them next to the pin file for the subsequent
// prepare/process + //go:embed steps. Shared by clang and cdnet; run via `go generate` from the
// dependency's package dir, e.g.
//
//	//go:generate go run ../scripts/download-deps.go clang-tidy.json
//
// The pin's sha256 map keys are archive filenames. A key with a recognized <os>-<arch> segment is
// platform-specific and is fetched only for the current build target (TARGETOS/TARGETARCH, falling
// back to runtime.GOOS/GOARCH); a key without one (e.g. clt.zip) is platform-agnostic and always
// fetched. This selects clang-tidy's single target archive and cdnet's lone clt.zip with one path.
//
// QODANA_CLI_DEPS_TOKEN (a JB Space bearer token, from the environment or a repo-root .env) is
// REQUIRED — without it the build fails; there is no mock fallback.
//
// Usage: go run ../scripts/download-deps.go [--force] [--all] <pin-file>
//
//	--force   re-download and rewrite the pin's sha256 hash(es)
//	--all     operate on every platform's archive, not just the current build target's
//
// See QD-14839.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/download"
	"github.com/JetBrains/qodana-cli/internal/foundation/dotenv"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

const tokenEnv = "QODANA_CLI_DEPS_TOKEN"

type dependency struct {
	Version string            `json:"version"`
	URL     string            `json:"url"`
	Sha256  map[string]string `json:"sha256"`
}

func (d dependency) resolveURL(filename string) string {
	return strings.NewReplacer("$version", d.Version, "$filename", filename).Replace(d.URL)
}

var platformRe = regexp.MustCompile(`(linux|darwin|windows)-(amd64|arm64)`)

func main() {
	force := flag.Bool("force", false, "re-download and rewrite the pin's sha256 hash(es)")
	all := flag.Bool("all", false, "operate on every platform's archive, not just the current build target's")
	flag.Parse()
	pinPath := flag.Arg(0)
	if pinPath == "" {
		log.Fatal("usage: go run scripts/download-deps.go [--force] [--all] <pin-file>")
	}

	token := dotenv.Value(tokenEnv, repoRootEnv())
	if token == "" {
		log.Fatalf("%s is required to download %s; set it in your environment or a repo-root .env", tokenEnv, pinPath)
	}

	dep := readDependency(pinPath)
	selected := selectFiles(dep.Sha256, *all)

	// A partial --force refresh (one platform) of a multi-platform pin (clang-tidy) would rewrite
	// only the host's hash and silently leave the others stale; warn so a maintainer adds --all.
	if *force && !*all && len(selected) < len(dep.Sha256) {
		fmt.Fprintf(os.Stderr, "download-deps: %s: refreshing only %d of %d hashes; pass --all to refresh every platform\n", pinPath, len(selected), len(dep.Sha256))
	}

	dir := filepath.Dir(pinPath)
	changed := false
	for _, f := range selected {
		got := fetch(dep.resolveURL(f), token, filepath.Join(dir, f), dep.Sha256[f], *force)
		if *force {
			dep.Sha256[f] = got
			changed = true
		}
	}
	if *force && changed {
		writeDependency(pinPath, dep)
	}
}

// selectFiles returns the pin's filenames relevant to the current build target: platform-agnostic
// names plus those whose <os>-<arch> matches TARGETOS/TARGETARCH (or runtime.GOOS/GOARCH). all=true
// returns every name. The result is sorted for deterministic ordering.
func selectFiles(sha map[string]string, all bool) []string {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if v := os.Getenv("TARGETOS"); v != "" {
		goos = v
	}
	if v := os.Getenv("TARGETARCH"); v != "" {
		goarch = v
	}
	var out []string
	for name := range sha {
		if all || matchesTarget(name, goos, goarch) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// matchesTarget reports whether a pin filename applies to goos/goarch. A name without a recognized
// <os>-<arch> segment is platform-agnostic (applies to every target); otherwise it must match.
func matchesTarget(name, goos, goarch string) bool {
	m := platformRe.FindStringSubmatch(name)
	if m == nil {
		return true
	}
	return m[1] == goos && m[2] == goarch
}

// fetch ensures destPath holds the expected bytes and returns its hex sha256. Normal mode: a cache
// hit (existing file already matches) skips the download; otherwise download.ToFile verifies against
// expected and commits no file on mismatch (its atomic temp+rename leaves any existing target
// untouched). Force mode: always downloads (no verify) and returns the actual hash to re-record.
func fetch(url, token, destPath, expected string, force bool) string {
	if !force && expected != "" {
		if h, err := hash.GetFileSha256(destPath); err == nil && hex.EncodeToString(h[:]) == expected {
			return expected
		}
	}
	opts := download.Options{Bearer: token, Hash: sha256.New}
	if !force {
		opts.ExpectedHex = expected
	}
	got, err := download.ToFile(url, destPath, opts)
	if err != nil {
		log.Fatal(err)
	}
	return got
}

func readDependency(pinPath string) dependency {
	data, err := os.ReadFile(pinPath)
	if err != nil {
		log.Fatal(err)
	}
	var d dependency
	if err := json.Unmarshal(data, &d); err != nil {
		log.Fatalf("parse %s: %s", pinPath, err)
	}
	// Pin keys become filenames written next to the pin; reject anything that could escape the dir.
	for name := range d.Sha256 {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			log.Fatalf("%s: invalid file name %q in sha256 (no path separators allowed)", pinPath, name)
		}
	}
	return d
}

func writeDependency(pinPath string, d dependency) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := fs.WriteFileAtomic(pinPath, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

// repoRootEnv reads the repo-root .env (walking up from CWD to the dir holding go.mod) into a map.
// go generate runs each directive in its package dir, so the .env lives a couple of levels up.
func repoRootEnv() map[string]string {
	dir, err := os.Getwd()
	if err != nil {
		return nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
	env, err := dotenv.Read(filepath.Join(dir, ".env"))
	if err != nil {
		log.Fatalf("reading .env: %s", err)
	}
	return env
}
