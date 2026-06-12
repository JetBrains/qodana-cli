//go:build ignore

// Downloads and sha256-verifies the pinned ReSharper CLT archive (clt.zip) from the versioned,
// auth-only JB Space Files repo, writing it next to cdnet/cdnet.json for the subsequent
// process-cltzip.go + //go:embed steps. Run via `go generate ./cdnet/...`.
//
// clt.zip is platform-agnostic, so there is no build-target selection. QODANA_CLI_DEPS_TOKEN (a JB
// Space bearer token, from the environment or a repo-root .env) is REQUIRED — without it the build
// fails; there is no mock fallback. Flag: -force re-downloads and rewrites the dependency's hash.
//
// See QD-14839.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/dotenv"
	"github.com/JetBrains/qodana-cli/internal/foundation/download"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
)

const (
	depFile  = "cdnet.json"
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
	force := flag.Bool("force", false, "re-download and rewrite the dependency's sha256 hash")
	flag.Parse()

	token := dotenv.Value(tokenEnv, repoRootEnv())
	if token == "" {
		log.Fatalf("%s is required to download the closed-source ReSharper CLT archive; set it in your environment or a repo-root .env", tokenEnv)
	}

	dep := readDependency()
	changed := false
	for name, expected := range dep.Sha256 {
		got := fetch(dep.resolveURL(name), token, name, expected, *force)
		if *force {
			dep.Sha256[name] = got
			changed = true
		}
	}
	if *force && changed {
		writeDependency(dep)
	}
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
