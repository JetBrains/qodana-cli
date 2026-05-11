package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	_ "embed"

	"github.com/JetBrains/qodana-cli/internal/foundation/hash"
	"github.com/JetBrains/qodana-cli/internal/foundation/shlex"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	log "github.com/sirupsen/logrus"
)

type ClangLinter struct {
}

func (l ClangLinter) RunAnalysis(c thirdpartyscan.Context) error {
	// Parse --clang-args first so a malformed value fails the run upfront,
	// before any shell bootstrap (processConfig calls utils.Bootstrap) or
	// disk I/O (getFilesAndCompilers reads compile_commands.json).
	extraClangArgs, err := prepareClangArgs(c.ClangArgs())
	if err != nil {
		return err
	}

	checks, configFile, err := processConfig(c)
	if err != nil {
		return err
	}

	filesAndCompilers, err := getFilesAndCompilers(c.ClangCompileCommands())
	if err != nil {
		return err
	}

	runClangTidyUnderProgress(c, filesAndCompilers, checks, configFile, extraClangArgs)

	_, err = mergeSarifReports(c)
	if err != nil {
		log.Errorf("Error merging SARIF reports: %s", err)
		return err
	}

	sarifPath := platform.GetSarifPath(c.ResultsDir())
	if err = fixupClangLinterTaxa(sarifPath); err != nil {
		return err
	}

	return nil
}

// prepareClangArgs parses the user's --clang-args value with POSIX-shell
// rules and prepends "--" when the user did not include their own separator.
//
// By default, --clang-args is treated as trailing compiler args (everything
// after "--"). Users can include "--" themselves to put options before it,
// reaching clang-tidy's own option parser (e.g. --config-file=).
func prepareClangArgs(raw string) ([]string, error) {
	tokens, err := shlex.Split(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid --clang-args %q: %w", raw, err)
	}
	if len(tokens) > 0 && !slices.Contains(tokens, "--") {
		tokens = append([]string{"--"}, tokens...)
	}
	return tokens, nil
}

//go:generate go run scripts/prepare-clang-tidy-binary.go

//goland:noinspection GoRedundantElseInIf
func (l ClangLinter) MountTools(path string) (map[string]string, error) {
	clang := thirdpartyscan.Clang

	val := make(map[string]string)
	val[clang] = getBinaryPath(path)

	_, err := os.Stat(val[clang])
	var isBinaryOk bool

	if errors.Is(err, os.ErrNotExist) {
		isBinaryOk = false
	} else if err != nil {
		return nil, err
	} else {
		hash, err := hash.GetFileSha256(val[clang])
		if err != nil {
			log.Warningf("getting sha256 of %q failed: %s", val[clang], err)
			isBinaryOk = false
		} else {
			isBinaryOk = bytes.Equal(hash[:], ClangTidySha256)
			if !isBinaryOk {
				log.Warningf(
					"failed to verify sha256 checksum of %q: expected %s, got %s", val[clang],
					hex.EncodeToString(ClangTidySha256),
					hex.EncodeToString(hash[:]),
				)
			}
		}
	}

	if !isBinaryOk {
		extension := ".tar.gz"
		if runtime.GOOS == "windows" {
			extension = ".zip"
		}

		clangArchive := clang + extension
		clangArchivePath := platform.ProcessAuxiliaryTool(clangArchive, clang, path, ClangTidyArchive)
		if err := platform.Decompress(clangArchivePath, path); err != nil {
			return nil, fmt.Errorf("failed to decompress clang archive: %w", err)
		}
		val[clang] = getBinaryPath(path)
	}

	return val, nil
}

func getBinaryPath(toolsPath string) string {
	binaryName := "clang-tidy"
	binaryPath := filepath.Join(toolsPath, binaryName)
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	if _, err := os.Stat(binaryPath); errors.Is(err, os.ErrNotExist) {
		binaryPath = filepath.Join(toolsPath, "bin", binaryName)
		//goland:noinspection GoBoolExpressions
		if runtime.GOOS == "windows" {
			binaryPath += ".exe"
		}
	}

	return binaryPath
}

func fixupClangLinterTaxa(sarifPath string) error {
	r, err := platform.ReadReport(sarifPath)
	if err != nil {
		log.Errorf("Error reading SARIF reports: %s", err)
		return err
	}

	for _, taxa := range r.Runs[0].Tool.Driver.Taxa {
		if len(taxa.Relationships) == 1 &&
			taxa.Relationships[0].Target != nil && taxa.Relationships[0].Target.Id == taxa.Id {
			taxa.Relationships[0].Target.Id = r.Runs[0].Tool.Driver.Taxa[0].Id
		}
	}

	err = platform.WriteReport(sarifPath, r)
	if err != nil {
		log.Errorf("Error writing SARIF reports: %s", err)
		return err
	}
	return nil
}
