//go:build ignore

// Build the local Kotlin application into the same embedded bundle as the published Java tools.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/flock"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
)

func main() {
	// go generate runs in internal/tooling.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		log.Fatal(err)
	}
	err = flock.With(filepath.Join(root, ".cache", "edict", "build.lock"), func() {
		if err := buildEdict(root); err != nil {
			log.Fatal(err)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}

func buildEdict(root string) error {
	project := filepath.Join(root, "edict", "kotlin")
	args := []string{"--no-daemon", "--console=plain", "bundledJar"}
	command := exec.Command(filepath.Join(project, "gradlew"), args...)
	if runtime.GOOS == "windows" {
		// Batch files need cmd.exe; the working directory avoids quoting a script
		// path containing spaces using cmd.exe's different argument rules.
		command = exec.Command("cmd.exe", append([]string{"/d", "/c", "gradlew.bat"}, args...)...)
	}
	command.Dir = project
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build bundled Edict JAR (requires JDK 21): %w", err)
	}
	data, err := os.ReadFile(filepath.Join(project, "build", "libs", "edict-cli.jar"))
	if err != nil {
		return err
	}
	libs := filepath.Join(root, "internal", "tooling", "libs")
	// Keep the portable JAR for jdeps module inference. It is not embedded in the CLI.
	if err := writeEdictJar(libs, data); err != nil {
		return err
	}
	// Generate every target under the same lock: GoReleaser compiles different
	// platforms concurrently from this checkout. Only the matching JAR is embedded.
	for target, nativePlatform := range edictNativePlatforms {
		filtered, err := edictPlatformJar(data, nativePlatform)
		if err != nil {
			return fmt.Errorf("filter Edict JAR for %s: %w", target, err)
		}
		if err := writeEdictJar(filepath.Join(libs, target), filtered); err != nil {
			return err
		}
	}
	return nil
}

// ONNX Runtime 1.22.0 does not ship Windows ARM64 libraries. That target keeps
// the Java code, as before, but cannot run local embeddings with this dependency.
var edictNativePlatforms = map[string]string{
	"darwin-amd64":  "osx-x64",
	"darwin-arm64":  "osx-aarch64",
	"linux-amd64":   "linux-x64",
	"linux-arm64":   "linux-aarch64",
	"windows-amd64": "win-x64",
	"windows-arm64": "",
}

func edictPlatformJar(data []byte, nativePlatform string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	const nativePrefix = "ai/onnxruntime/native/"
	for _, entry := range reader.File {
		if strings.HasPrefix(entry.Name, nativePrefix) &&
			(nativePlatform == "" || !strings.HasPrefix(entry.Name, nativePrefix+nativePlatform+"/")) {
			continue
		}
		if entry.FileInfo().IsDir() {
			// Gradle deflates empty directory entries. Go's raw ZIP copy rejects
			// their nonzero compressed size, so recreate only the directory header.
			header := entry.FileHeader
			if _, err := writer.CreateHeader(&header); err != nil {
				return nil, err
			}
			continue
		}
		// Copy the existing compressed data without decompressing or recompressing it.
		if err := writer.Copy(entry); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeEdictJar(libs string, data []byte) error {
	// Tool extraction reuses existing cache files. Hash the filtered content too,
	// so a platform or dependency change cannot reuse a stale extracted JAR.
	digest := sha256.Sum256(data)
	if err := os.MkdirAll(libs, 0o755); err != nil {
		return err
	}
	destination := filepath.Join(libs, fmt.Sprintf("edict-cli-%x.jar", digest[:12]))
	if _, err := os.Stat(destination); err != nil {
		if err := fs.WriteFileAtomic(destination, data, 0o644); err != nil {
			return err
		}
	}
	previous, err := filepath.Glob(filepath.Join(libs, "edict-cli-*.jar"))
	if err != nil {
		return err
	}
	for _, path := range previous {
		if path != destination {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	log.Printf("Bundled %s (%d bytes)", destination, len(data))
	return nil
}
