package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"runtime"
)

type ClangLinter struct {
}

func (l ClangLinter) RunAnalysis(c thirdpartyscan.Context) error {
	checks, err := allowedChecksByLicenseAndYaml(c)
	if err != nil {
		return err
	}

	filesAndCompilers, err := getFilesAndCompilers(c.ClangCompileCommands())
	if err != nil {
		return err
	}

	runClangTidyUnderProgress(c, filesAndCompilers, checks)

	_, err = mergeSarifReports(c)
	if err != nil {
		log.Errorf("Error merging SARIF reports: %s", err)
		return err
	}

	sarifPath := platform.GetSarifPath(c.ResultsDir())
	err = fixupClangLinterTaxa(sarifPath, err)
	if err != nil {
		return err
	}

	return nil
}

func (l ClangLinter) MountTools(tempPath string, mountPath string) (map[string]string, error) {
	clang := thirdpartyscan.Clang

	toolsPath := mountPath
	val := make(map[string]string)
	val[clang] = getBinaryPath(toolsPath)
	if _, err := os.Stat(val[clang]); err != nil {
		if os.IsNotExist(err) {
			clangArchive := clang + Ext
			clangArchivePath := platform.ProcessAuxiliaryTool(clangArchive, clang, tempPath, toolsPath, Clang)
			if err := platform.Decompress(clangArchivePath, toolsPath); err != nil {
				return nil, fmt.Errorf("failed to decompress clang archive: %w", err)
			}
			val[clang] = getBinaryPath(toolsPath)
		}
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

	// Check if the file exists; if it doesn't, try adding "bin/" before the binary name
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		binaryPath = filepath.Join(toolsPath, "bin", binaryName)
		//goland:noinspection GoBoolExpressions
		if runtime.GOOS == "windows" {
			binaryPath += ".exe"
		}
	}

	return binaryPath
}

func fixupClangLinterTaxa(sarifPath string, err error) error {
	r, err := platform.ReadReport(sarifPath)
	if err != nil {
		log.Errorf("Error reading SARIF reports: %s", err)
		return err
	}

	for _, taxa := range r.Runs[0].Tool.Driver.Taxa {
		if taxa.Relationships != nil && len(taxa.Relationships) == 1 &&
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
