//go:build ignore

// Build the local Kotlin application into the same embedded bundle as the published Java tools.
package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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
	// Tool extraction reuses existing cache files. A content hash prevents stale JARs
	// when the Kotlin sources or bundled skills change within one CLI version.
	digest := sha256.Sum256(data)
	libs := filepath.Join(root, "internal", "tooling", "libs")
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
	log.Printf("Bundled %s", filepath.Base(destination))
	return nil
}
