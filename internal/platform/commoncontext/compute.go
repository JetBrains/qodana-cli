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

package commoncontext

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/platform/git"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	log "github.com/sirupsen/logrus"
)

func Compute(
	overrideLinter string,
	overrideIde string,
	overrideImage string,
	overrideWithinDocker string,
	cacheDirFromCliOptions string,
	resultsDirFromCliOptions string,
	reportDirFromCliOptions string,
	qodanaCloudToken string,
	clearCache bool,
	projectDir string,
	repositoryRoot string,
	localNotEffectiveQodanaYamlPathInProject string,
) Context {
	analyzer := GuessAnalyzerFromEnvAndCLI(overrideIde, overrideLinter, overrideImage, overrideWithinDocker)

	if analyzer == nil {
		analyzer = getAnalyzerFromProject(
			qodanaCloudToken,
			projectDir,
			localNotEffectiveQodanaYamlPathInProject,
		)
	}

	return computeCommon(
		analyzer,
		projectDir,
		repositoryRoot,
		cacheDirFromCliOptions,
		resultsDirFromCliOptions,
		reportDirFromCliOptions,
		clearCache,
		qodanaCloudToken,
	)
}

func Compute3rdParty(
	linterName string,
	isEap bool,
	cacheDirFromCliOptions string,
	resultsDirFromCliOptions string,
	reportDirFromCliOptions string,
	qodanaCloudToken string,
	clearCache bool,
	projectDir string,
	repositoryRoot string,
) Context {
	linter := product.FindLinterByName(linterName)
	if linter == product.UnknownLinter {
		log.Fatalf("Unsupported 3rd party linter name detected: %s", linterName)
	}
	analyzer := &product.NativeAnalyzer{
		Linter: linter,
		Eap:    isEap,
	}
	return computeCommon(
		analyzer,
		projectDir,
		repositoryRoot,
		cacheDirFromCliOptions,
		resultsDirFromCliOptions,
		reportDirFromCliOptions,
		clearCache,
		qodanaCloudToken,
	)
}

func computeCommon(
	analyzer product.Analyzer,
	projectDir string,
	repositoryRoot string,
	cacheDirFromCliOptions string,
	resultsDirFromCliOptions string,
	reportDirFromCliOptions string,
	clearCache bool,
	qodanaCloudToken string,
) Context {
	qodanaId := computeId(analyzer, projectDir)
	systemDir := computeQodanaSystemDir(cacheDirFromCliOptions)
	linterDir := filepath.Join(systemDir, qodanaId)
	resultsDir := computeResultsDir(resultsDirFromCliOptions, linterDir)
	cacheDir := computeCacheDir(cacheDirFromCliOptions, linterDir)
	reportDir := computeReportDir(reportDirFromCliOptions, resultsDir)

	commonCtx := Context{
		Analyzer:        analyzer,
		IsClearCache:    clearCache,
		CacheDir:        cacheDir,
		ResultsDir:      resultsDir,
		QodanaSystemDir: systemDir,
		ReportDir:       reportDir,
		Id:              qodanaId,
		QodanaToken:     qodanaCloudToken,
	}

	if repositoryRoot == "" {
		vcsRoot, err := git.Root(projectDir, commonCtx.LogDir())

		if err != nil {
			repositoryRoot = projectDir
		} else {
			repositoryRoot = vcsRoot
		}
	}

	normalizedProjectDir, err := normalizePath(projectDir)
	if err != nil {
		log.Fatalf("Can not normalize project dir %s: %v", projectDir, err)
	}

	// Normalize repositoryRoot to be a substring of projectDir path
	// This handles case-insensitive filesystems where /tmp/PROJECT and /tmp/project are the same
	normalizedRepoRoot, err := normalizeRepositoryRoot(normalizedProjectDir, repositoryRoot)
	if err != nil {
		log.Fatalf(
			"The project directory must be located inside repository root. Please, specify correct --repository-root argument. ProjectDir: %s. RepositoryRoot: %s. Error: %v",
			normalizedProjectDir,
			normalizedRepoRoot,
			err,
		)
	}

	log.Debugf("Repository root: %q", repositoryRoot)
	log.Debugf("Normalized repository root: %q", normalizedRepoRoot)
	log.Debugf("Project root: %q", projectDir)
	log.Debugf("Normalized project root: %q", normalizedProjectDir)

	commonCtx.ProjectDir = normalizedProjectDir
	commonCtx.RepositoryRoot = normalizedRepoRoot
	return commonCtx
}

// normalizeRepositoryRoot checks if projectDir is inside or equal to repositoryRoot and returns
// the repositoryRoot path as it appears in projectDir (to handle case-insensitive filesystems).
// Returns error if projectDir is not inside repositoryRoot.
func normalizeRepositoryRoot(projectDir, repositoryRoot string) (string, error) {
	normalizedRepoRoot, err := normalizePath(repositoryRoot)
	if err != nil {
		return repositoryRoot, err
	}

	repoRootInfo, err := os.Stat(normalizedRepoRoot)
	if err != nil {
		return repositoryRoot, err
	}

	// Walk up from projectDir to find the directory that matches repositoryRoot
	// Return the path as it appears in projectDir's path
	current := projectDir
	for {
		currentInfo, err := os.Stat(current)
		if err != nil {
			return repositoryRoot, err
		}

		if os.SameFile(currentInfo, repoRootInfo) {
			// Found the matching directory - return current path (from projectDir's perspective)
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached root without finding repositoryRoot
			return normalizedRepoRoot, fmt.Errorf("projectDir is not inside repositoryRoot")
		}
		current = parent
	}
}

func normalizePath(path string) (string, error) {
	pathWithoutSymlinks, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(pathWithoutSymlinks)
}

func getAnalyzerFromProject(
	qodanaCloudToken string,
	projectDir string,
	localNotEffectiveQodanaYamlPathInProject string,
) product.Analyzer {
	qodanaYamlPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(
		projectDir,
		localNotEffectiveQodanaYamlPathInProject,
	)
	qodanaYaml := qdyaml.LoadQodanaYamlByFullPath(qodanaYamlPath)
	if qodanaYaml.Linter == "" && qodanaYaml.Ide == "" && qodanaYaml.Image == "" {
		msg.WarningMessage(
			"No valid `linter:` or `image:` field found in %s. Have you run %s? Running that for you...",
			msg.PrimaryBold(localNotEffectiveQodanaYamlPathInProject),
			msg.PrimaryBold("qodana init"),
		)
		return SelectAnalyzerForPath(projectDir, qodanaCloudToken)
	}
	if qodanaYaml.Ide != "" {
		msg.WarningMessage(
			"`ide:` field in %s is deprecated. Please use `--linter` and `--within-docker=false` instead.",
			qodanaYamlPath,
		)
	}

	if qodanaYaml.Linter != "" && qodanaYaml.Ide != "" {
		log.Fatalf(
			"You have both `linter:` (%s) and `ide:` (%s) fields set in %s. Modify the configuration file to keep one of them",
			qodanaYaml.Linter,
			qodanaYaml.Ide,
			qodanaYamlPath,
		)
		return nil
	}

	if qodanaYaml.Image != "" && qodanaYaml.Ide != "" {
		log.Fatalf(
			"You have both `image:` (%s) and `ide:` (%s) fields set in %s. Modify the configuration file to keep one of them",
			qodanaYaml.Image,
			qodanaYaml.Ide,
			localNotEffectiveQodanaYamlPathInProject,
		)
		return nil
	}

	return guessAnalyzerFromParams(qodanaYaml.Ide, qodanaYaml.Linter, qodanaYaml.Image, qodanaYaml.WithinDocker)
}

func computeId(analyzer product.Analyzer, projectDir string) string {
	length := 7
	projectAbs, _ := filepath.Abs(projectDir)
	id := fmt.Sprintf(
		"%s-%s",
		getHash(analyzer.Name())[0:length+1],
		getHash(projectAbs)[0:length+1],
	)
	return id
}

// getHash returns a SHA256 hash of a given string.
func getHash(s string) string {
	sha256sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sha256sum[:])
}

func computeQodanaSystemDir(cacheDirFromCliOptions string) string {
	if cacheDirFromCliOptions != "" {
		return filepath.Dir(filepath.Dir(cacheDirFromCliOptions))
	}

	userCacheDir, _ := os.UserCacheDir()
	return filepath.Join(
		userCacheDir,
		"JetBrains",
		"Qodana",
	)
}

func computeResultsDir(resultsDirFromCliOptions string, linterDir string) string {
	if resultsDirFromCliOptions != "" {
		return resultsDirFromCliOptions
	}
	if qdenv.IsContainer() {
		return qdcontainer.DataResultsDir
	} else {
		return filepath.Join(linterDir, "results")
	}
}

func computeCacheDir(cacheDirFromCliOptions string, linterDir string) string {
	if cacheDirFromCliOptions != "" {
		return cacheDirFromCliOptions
	}
	if qdenv.IsContainer() {
		return qdcontainer.DataCacheDir
	} else {
		return filepath.Join(linterDir, "cache")
	}
}

func computeReportDir(reportDirFromCliOptions string, resultsDir string) string {
	if reportDirFromCliOptions != "" {
		return reportDirFromCliOptions
	}
	if qdenv.IsContainer() {
		return qdcontainer.DataResultsReportDir
	} else {
		return filepath.Join(resultsDir, "report")
	}
}
