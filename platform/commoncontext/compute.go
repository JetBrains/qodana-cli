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
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
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
		ProjectDir:      projectDir,
		ResultsDir:      resultsDir,
		QodanaSystemDir: systemDir,
		ReportDir:       reportDir,
		Id:              qodanaId,
		QodanaToken:     qodanaCloudToken,
	}
	return commonCtx
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
