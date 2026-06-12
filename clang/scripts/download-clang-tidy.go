//go:build ignore

// Downloads and sha256-verifies the pinned clang-tidy archive for this build target from the
// versioned, auth-only JB Space Files repo, writing it next to clang/clang-tidy.json for the
// subsequent prepare-clang-tidy-binary.go + //go:embed steps. Run via `go generate ./clang/...`.
//
// Honors TARGETOS/TARGETARCH (set per target by the goreleaser cross-build) over runtime.GOOS/ARCH,
// matching prepare-clang-tidy-binary.go's archive naming so the downloaded file is the one it hashes.
// QODANA_CLI_DEPS_TOKEN (a JB Space bearer token, from the environment or a repo-root .env) is
// REQUIRED — without it the build fails; there is no mock fallback. Flags:
//
//	--force   re-download and rewrite the dependency's sha256 hash(es)
//	--all     operate on every platform's archive, not just this build target's (with --force,
//	          refresh every platform's hash)
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
	"runtime"
	"sort"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/dotenv"
	"github.com/JetBrains/qodana-cli/internal/foundation/download"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

const (
	depFile  = "clang-tidy.json"
	tokenEnv = "QODANA_CLI_DEPS_TOKEN"
)

type dependency struct {
	Version string            `json:"version"`
	URL     string            `json:"url"`
	Sha256  map[string]string `json:"sha256"`
}

func (d dependency) resolveURL(filename string) string {
	return strings.NewReplacer("$version", d.Version, "$filename", filename).Replace(d.URL)
}

func main() {
	force := flag.Bool("force", false, "re-download and rewrite the dependency's sha256 hash(es)")
	all := flag.Bool("all", false, "operate on every platform's archive, not just this build target's")
	flag.Parse()

	token := dotenv.Value(tokenEnv, repoRootEnv())
	if token == "" {
		log.Fatalf("%s is required to download the closed-source clang-tidy archive; set it in your environment or a repo-root .env", tokenEnv)
	}

	dep := readDependency()

	// Which archives to fetch: every platform under -all (hash refresh), else just this target's.
	var selected []string
	if *all {
		for f := range dep.Sha256 {
			selected = append(selected, f)
		}
		sort.Strings(selected)
	} else {
		selected = []string{targetArchive()}
	}

	// A partial -force refresh (one platform) of the multi-platform clang-tidy dependency would
	// rewrite only the host's hash and silently leave the others stale; warn so a maintainer adds -all.
	if *force && !*all && len(selected) < len(dep.Sha256) {
		fmt.Fprintf(os.Stderr, "download-clang-tidy: refreshing only %d of %d platform hashes; pass --all to refresh every platform\n", len(selected), len(dep.Sha256))
	}

	changed := false
	for _, f := range selected {
		expected, ok := dep.Sha256[f]
		if !ok {
			log.Fatalf("%s: %q is not in the dependency file", depFile, f)
		}
		got := fetch(dep.resolveURL(f), token, f, expected, *force)
		if *force {
			dep.Sha256[f] = got
			changed = true
		}
	}
	if *force && changed {
		writeDependency(dep)
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

// fetch ensures filename holds the expected bytes and returns its hex sha256. Normal mode: a cache
// hit (existing file already matches) skips the download; otherwise download.ToFile verifies against
// expected and commits no file on mismatch (its atomic temp+rename leaves any existing target
// untouched). Force mode: always downloads (no verify) and returns the actual hash to re-record.
func fetch(url, token, filename, expected string, force bool) string {
	if !force && expected != "" {
		if h, err := hash.GetFileSha256(filename); err == nil && hex.EncodeToString(h[:]) == expected {
			return expected
		}
	}
	opts := download.Options{Bearer: token, Hash: sha256.New}
	if !force {
		opts.ExpectedHex = expected
	}
	got, err := download.ToFile(url, filename, opts)
	if err != nil {
		log.Fatal(err)
	}
	return got
}

func readDependency() dependency {
	data, err := os.ReadFile(depFile)
	if err != nil {
		log.Fatal(err)
	}
	var d dependency
	if err := json.Unmarshal(data, &d); err != nil {
		log.Fatalf("parse %s: %s", depFile, err)
	}
	// Dependency keys become filenames written next to the file; reject anything that could escape the dir.
	for name := range d.Sha256 {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			log.Fatalf("%s: invalid file name %q in sha256 (no path separators allowed)", depFile, name)
		}
	}
	return d
}

func writeDependency(d dependency) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := fs.WriteFileAtomic(depFile, append(b, '\n'), 0o644); err != nil {
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
