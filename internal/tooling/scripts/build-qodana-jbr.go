//go:build ignore

/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JetBrains/qodana-cli/internal/coreutils/archive"
	"github.com/JetBrains/qodana-cli/internal/coreutils/fs"
)

// Configuration ===============================================================================

const jbrBaseURL = "https://cache-redirector.jetbrains.com/intellij-jbr"
const jbrGitHubAPI = "https://api.github.com/repos/JetBrains/JetBrainsRuntime/releases/latest"

type platform struct {
	flavor string
	goos   string
	goarch string
}

var platforms = []platform{
	{"linux-x64", "linux", "amd64"},
	{"linux-aarch64", "linux", "arm64"},
	{"osx-x64", "darwin", "amd64"},
	{"osx-aarch64", "darwin", "arm64"},
	{"windows-x64", "windows", "amd64"},
	{"windows-aarch64", "windows", "arm64"},
}

// Cache layout (.cache/jbr/{goos}-{goarch}/):
//   src/              — extracted upstream JBR tree (top dir stripped)
//   build/            — jlink output (minimal runtime, unpacked)
//   dist/             — final qodana-jbr tar.gz archive
//   upstream.sha512   — SHA-512 of the upstream archive (validation anchor)
//
// Invalidation: fetch upstream .checksum, compare to upstream.sha512.
// If mismatch or missing → re-download, re-extract, rebuild, re-dist.

func main() {
	version, build := detectJBRVersion()
	log.Printf("JBR version: %s, build: %s", version, build)

	repoRoot := findRepoRoot()
	cacheDir := filepath.Join(repoRoot, ".cache", "jbr")
	embedDir := "qodana-jbrs" // relative to CWD (internal/tooling)

	// Fetch upstream checksums for all platforms (parallel, lightweight)
	upstreamSHA := fetchAllUpstreamChecksums(version, build)

	// Check if all platforms are up to date
	if allPlatformsCurrent(cacheDir, upstreamSHA) {
		linkAllToEmbed(cacheDir, embedDir)
		log.Println("All JBR archives up to date, skipping build")
		return
	}

	// Ensure host JBR is available for jlink/jdeps tools
	hostPlatform := findHostPlatform()
	ensurePlatformSrc(cacheDir, version, build, hostPlatform, upstreamSHA[hostPlatform.flavor])
	jlinkBin := findBinary(platformSrcDir(cacheDir, hostPlatform), "jlink")
	jdepsBin := findBinary(platformSrcDir(cacheDir, hostPlatform), "jdeps")
	log.Printf("Host JBR tools: jlink=%s, jdeps=%s", jlinkBin, jdepsBin)

	// Run jdeps on all JARs to determine required modules
	modules := runJdepsOnAllJars(jdepsBin)
	log.Printf("Required modules: %s", modules)

	// Ensure all platforms have src/ extracted (parallel)
	ensureAllPlatformSrcs(cacheDir, version, build, upstreamSHA)

	// Build each platform that needs it
	for _, p := range platforms {
		pDir := platformDir(cacheDir, p)
		sha := upstreamSHA[p.flavor]

		if readFile(filepath.Join(pDir, "upstream.sha512")) == sha && hasDist(pDir) {
			log.Printf("OK, SKIPPED: %s-%s", p.goos, p.goarch)
			continue
		}

		buildPlatform(jlinkBin, modules, cacheDir, p, version, build)
		writeFile(filepath.Join(pDir, "upstream.sha512"), sha)
		log.Printf("BUILT: %s-%s", p.goos, p.goarch)
	}

	linkAllToEmbed(cacheDir, embedDir)
	log.Println("Done building all qodana-jbr archives")
}

// Version detection ===========================================================================

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func detectJBRVersion() (version, build string) {
	version = os.Getenv("QODANA_JBR_VERSION")
	build = os.Getenv("QODANA_JBR_BUILD")
	if version != "" && build != "" {
		return version, build
	}

	tagName := fetchLatestJBRTag()
	tagName = strings.TrimPrefix(tagName, "jbr-release-")

	idx := strings.LastIndex(tagName, "b")
	if idx <= 0 {
		log.Fatalf("Cannot parse JBR tag %q: no build separator 'b' found", tagName)
	}

	if version == "" {
		version = tagName[:idx]
	}
	if build == "" {
		build = tagName[idx:]
	}
	return version, build
}

func fetchLatestJBRTag() string {
	req, err := http.NewRequest(http.MethodGet, jbrGitHubAPI, nil)
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Error fetching JBR releases: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf(
			"GitHub API rate limit hit (HTTP %d). Set GITHUB_TOKEN env var for authenticated access.\nResponse: %s",
			resp.StatusCode, strings.TrimSpace(string(body)),
		)
	}
	if resp.StatusCode == 401 {
		log.Fatalf("GitHub API unauthorized (HTTP 401). Check your GITHUB_TOKEN env var.")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("GitHub API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		log.Fatalf("Error decoding GitHub API response: %v", err)
	}
	if rel.TagName == "" {
		log.Fatal("GitHub API returned empty tag_name for latest JBR release")
	}

	log.Printf("Detected latest JBR release tag: %s", rel.TagName)
	return rel.TagName
}

// Platform helpers ============================================================================

func goosGoarchToFlavor(goos, goarch string) string {
	osName := goos
	if goos == "darwin" {
		osName = "osx"
	}
	archName := goarch
	switch goarch {
	case "amd64":
		archName = "x64"
	case "arm64":
		archName = "aarch64"
	}
	return osName + "-" + archName
}

func findHostPlatform() platform {
	flavor := goosGoarchToFlavor(runtime.GOOS, runtime.GOARCH)
	for _, p := range platforms {
		if p.flavor == flavor {
			return p
		}
	}
	log.Fatalf("Host platform %s-%s not in supported platforms", runtime.GOOS, runtime.GOARCH)
	return platform{}
}

func platformDir(cacheDir string, p platform) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s", p.goos, p.goarch))
}

func platformSrcDir(cacheDir string, p platform) string {
	return filepath.Join(platformDir(cacheDir, p), "src")
}

func jbrArchiveURL(version, flavor, build string) string {
	return fmt.Sprintf("%s/jbrsdk-%s-%s-%s.tar.gz", jbrBaseURL, version, flavor, build)
}

func jbrChecksumURL(version, flavor, build string) string {
	return fmt.Sprintf("%s/jbrsdk-%s-%s-%s.tar.gz.checksum", jbrBaseURL, version, flavor, build)
}

// Upstream checksum fetching ==================================================================

func fetchAllUpstreamChecksums(version, build string) map[string]string {
	result := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range platforms {
		wg.Add(1)
		go func(p platform) {
			defer wg.Done()
			sha := fetchUpstreamChecksum(version, p.flavor, build)
			mu.Lock()
			result[p.flavor] = sha
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return result
}

func fetchUpstreamChecksum(version, flavor, build string) string {
	url := jbrChecksumURL(version, flavor, build)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("Error creating checksum request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Error fetching checksum %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("Failed to fetch checksum %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading checksum %s: %v", url, err)
	}

	// Format: "<hex>  <filename>" — take just the hex part
	parts := strings.Fields(strings.TrimSpace(string(body)))
	if len(parts) == 0 {
		log.Fatalf("Empty checksum response from %s", url)
	}
	return parts[0]
}

// Cache validation ============================================================================

func allPlatformsCurrent(cacheDir string, upstreamSHA map[string]string) bool {
	for _, p := range platforms {
		pDir := platformDir(cacheDir, p)
		cached := readFile(filepath.Join(pDir, "upstream.sha512"))
		if cached != upstreamSHA[p.flavor] || !hasDist(pDir) {
			return false
		}
	}
	return true
}

func hasDist(pDir string) bool {
	entries, err := os.ReadDir(filepath.Join(pDir, "dist"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			return true
		}
	}
	return false
}

// Download + extract ==========================================================================

func ensurePlatformSrc(cacheDir, version, build string, p platform, expectedSHA string) {
	pDir := platformDir(cacheDir, p)
	srcDir := filepath.Join(pDir, "src")

	// If SHA matches, src/ is trusted
	if readFile(filepath.Join(pDir, "upstream.sha512")) == expectedSHA {
		if _, err := os.Stat(srcDir); err == nil {
			return
		}
	}

	// Need to (re)download and extract
	if err := os.MkdirAll(pDir, 0o755); err != nil {
		log.Fatalf("Failed to create platform dir %s: %v", pDir, err)
	}

	url := jbrArchiveURL(version, p.flavor, build)
	archivePath := filepath.Join(pDir, "upstream.tar.gz")
	log.Printf("DOWNLOADING: %s", filepath.Base(url))
	downloadAndVerify(url, archivePath, expectedSHA)

	// Extract (strip top-level directory)
	log.Printf("EXTRACTING: %s -> %s", filepath.Base(url), srcDir)
	if err := os.RemoveAll(srcDir); err != nil {
		log.Fatalf("Failed to clean src dir %s: %v", srcDir, err)
	}
	if err := archive.ExtractTarGz(archivePath, srcDir, true); err != nil {
		log.Fatalf("Failed to extract %s: %v", archivePath, err)
	}

	// Remove the downloaded archive — we only need the extracted tree
	os.Remove(archivePath)

	// Clear stale build/ and dist/ since src changed
	os.RemoveAll(filepath.Join(pDir, "build"))
	os.RemoveAll(filepath.Join(pDir, "dist"))

	// Write SHA marker (src is now valid; build/dist will be written by buildPlatform)
	writeFile(filepath.Join(pDir, "upstream.sha512"), expectedSHA)
}

func ensureAllPlatformSrcs(cacheDir, version, build string, upstreamSHA map[string]string) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	for _, p := range platforms {
		wg.Add(1)
		go func(p platform) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ensurePlatformSrc(cacheDir, version, build, p, upstreamSHA[p.flavor])
		}(p)
	}
	wg.Wait()
}

// Build =======================================================================================

func buildPlatform(jlinkBin, modules, cacheDir string, p platform, version, build string) {
	pDir := platformDir(cacheDir, p)
	srcDir := filepath.Join(pDir, "src")
	buildDir := filepath.Join(pDir, "build")
	distDir := filepath.Join(pDir, "dist")

	jmodsDir := findJmodsDir(srcDir)

	// jlink into build/
	if err := os.RemoveAll(buildDir); err != nil {
		log.Fatalf("Failed to clean build dir: %v", err)
	}
	log.Printf("Running jlink for %s-%s (%s)", p.goos, p.goarch, p.flavor)
	cmd := exec.Command(jlinkBin,
		"--module-path", jmodsDir,
		"--compress=zip-6",
		"--add-modules", modules,
		"--no-header-files",
		"--no-man-pages",
		"--strip-debug",
		"--output", buildDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("jlink failed for %s: %v", p.flavor, err)
	}

	// Package into dist/
	if err := os.RemoveAll(distDir); err != nil {
		log.Fatalf("Failed to clean dist dir: %v", err)
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		log.Fatalf("Failed to create dist dir: %v", err)
	}
	archiveName := fmt.Sprintf("qodana-jbrsdk-%s-%s-%s.tar.gz", version, p.flavor, build)
	topDir := archiveName[:len(archiveName)-len(".tar.gz")]
	if err := archive.CreateTarGz(buildDir, filepath.Join(distDir, archiveName), topDir); err != nil {
		log.Fatalf("Failed to create archive for %s: %v", p.flavor, err)
	}
}

// Embed linking ===============================================================================

func linkAllToEmbed(cacheDir, embedDir string) {
	for _, p := range platforms {
		distDir := filepath.Join(platformDir(cacheDir, p), "dist")
		srcArchive := findFirstTarGz(distDir)
		if srcArchive == "" {
			log.Fatalf("No dist archive found for %s-%s", p.goos, p.goarch)
		}

		dstDir := filepath.Join(embedDir, fmt.Sprintf("%s-%s", p.goos, p.goarch))
		dst := filepath.Join(dstDir, filepath.Base(srcArchive))

		if fs.SameFile(srcArchive, dst) {
			continue
		}

		if err := fs.CleanDirectory(dstDir); err != nil {
			log.Fatalf("Failed to clean embed dir %s: %v", dstDir, err)
		}
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			log.Fatalf("Failed to create embed dir %s: %v", dstDir, err)
		}

		if err := os.Link(srcArchive, dst); err != nil {
			log.Printf("Hardlink failed (%v), falling back to copy: %s -> %s", err, srcArchive, dst)
			if err := fs.CopyFile(srcArchive, dst); err != nil {
				log.Fatalf("Failed to copy %s -> %s: %v", srcArchive, dst, err)
			}
		}
	}
}

// Tool discovery ==============================================================================

func findBinary(dir, name string) string {
	binName := name
	if runtime.GOOS == "windows" {
		binName = name + ".exe"
	}

	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == binName {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && found == "" {
		log.Fatalf("Error searching for %s in %s: %v", binName, dir, err)
	}
	if found == "" {
		log.Fatalf("Binary %s not found in %s", binName, dir)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(found, 0o755); err != nil {
			log.Printf("WARN: failed to chmod %s: %v", found, err)
		}
	}
	return found
}

func findJmodsDir(dir string) string {
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "jmods" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && found == "" {
		log.Fatalf("Error searching for jmods in %s: %v", dir, err)
	}
	if found == "" {
		log.Fatalf("jmods directory not found in %s", dir)
	}
	return found
}

// jdeps =======================================================================================

func runJdepsOnAllJars(jdepsBin string) string {
	libsDir := "libs"
	entries, err := os.ReadDir(libsDir)
	if err != nil {
		log.Fatalf("Failed to read libs directory %s: %v", libsDir, err)
	}

	allModules := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}
		jarPath := filepath.Join(libsDir, entry.Name())
		modules := runJdeps(jdepsBin, jarPath)
		log.Printf("Detected jmods for %s: %s", entry.Name(), strings.Join(modules, ", "))
		for _, m := range modules {
			allModules[m] = true
		}
	}

	if len(allModules) == 0 {
		log.Fatal("No modules detected by jdeps. Ensure JARs exist in libs/ directory.")
	}

	sorted := make([]string, 0, len(allModules))
	for m := range allModules {
		sorted = append(sorted, m)
	}
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

func runJdeps(jdepsBin, jarPath string) []string {
	cmd := exec.Command(jdepsBin, "--list-deps", "--ignore-missing-deps", jarPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		if strings.HasPrefix(outStr, "Error:") {
			log.Fatalf("jdeps error for %s:\n%s", jarPath, outStr)
		}
		log.Fatalf("Failed to run jdeps on %s: %v\nOutput: %s", jarPath, err, outStr)
	}

	var modules []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "/"); idx > 0 {
			line = line[:idx]
		}
		modules = append(modules, line)
	}
	return modules
}

// Download + verification =====================================================================

// downloadAndVerify streams url to destPath while computing SHA-512, then verifies.
func downloadAndVerify(url, destPath, expectedSHA string) {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("Error creating request %s: %v", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error downloading %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("Download failed for %s: HTTP %d", url, resp.StatusCode)
	}

	tmp := destPath + ".part"
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		log.Fatalf("Failed to create directory for %s: %v", destPath, err)
	}

	out, err := os.Create(tmp)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", tmp, err)
	}

	hasher := sha512.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hasher), resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		log.Fatalf("Failed to download %s: %v", url, copyErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		log.Fatalf("Failed to close %s: %v", tmp, closeErr)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != expectedSHA {
		os.Remove(tmp)
		log.Fatalf("SHA-512 mismatch for %s:\n  expected: %s\n  actual:   %s", filepath.Base(url), expectedSHA, actualSHA)
	}

	if err := os.Rename(tmp, destPath); err != nil {
		log.Fatalf("Failed to rename %s -> %s: %v", tmp, destPath, err)
	}
}

// File utilities ==============================================================================

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeFile(path string, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatalf("Failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Fatalf("Failed to write %s: %v", path, err)
	}
}

func findFirstTarGz(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}


// Repo root detection =========================================================================

func findRepoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	log.Printf("WARN: git rev-parse failed (%v), defaulting to current directory for cache root", err)
	cwd, _ := os.Getwd()
	return cwd
}
