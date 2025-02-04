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

package startupargs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"os"
	"path/filepath"
)

func Compute(
	linterFromCliOptions string,
	ideFromCliOptions string,
	cacheDirFromCliOptions string,
	resultsDirFromCliOptions string,
	qodanaCloudToken string,
	qodanaLicenseOnlyToken string,
	clearCache bool,
	projectDir string,
	qodanaYamlPath string,
) Args {
	linter, ide := computeActualLinterAndIde(linterFromCliOptions, ideFromCliOptions, qodanaCloudToken, projectDir, qodanaYamlPath)
	qodanaId := computeId(linter, ide, projectDir)

	systemDir := computeQodanaSystemDir(cacheDirFromCliOptions)
	linterDir := filepath.Join(systemDir, qodanaId)
	resultsDir := computeResultsDir(resultsDirFromCliOptions, linterDir)
	cacheDir := computeCacheDir(cacheDirFromCliOptions, linterDir)

	args := Args{
		Linter:                 linter,
		Ide:                    ide,
		IsClearCache:           clearCache,
		CacheDir:               cacheDir,
		ProjectDir:             projectDir,
		ResultsDir:             resultsDir,
		QodanaSystemDir:        systemDir,
		Id:                     qodanaId,
		QodanaToken:            qodanaCloudToken,
		QodanaLicenseOnlyToken: qodanaLicenseOnlyToken,
	}
	return args
}

func computeActualLinterAndIde(
	linterFromCliOptions string,
	ideFromCliOptions string,
	qodanaCloudToken string,
	projectDir string,
	qodanaYamlPath string,
) (string, string) {
	linter := linterFromCliOptions
	ide := ideFromCliOptions

	if linter == "" && ide == "" {
		qodanaYaml := platform.LoadQodanaYaml(projectDir, qodanaYamlPath)
		if qodanaYaml.Linter == "" && qodanaYaml.Ide == "" {
			platform.WarningMessage(
				"No valid `linter:` or `ide:` field found in %s. Have you run %s? Running that for you...",
				platform.PrimaryBold(qodanaYamlPath),
				platform.PrimaryBold("qodana init"),
			)
			analyzer := platform.GetAnalyzer(projectDir, qodanaCloudToken)
			if platform.IsNativeAnalyzer(analyzer) {
				ide = analyzer
			} else {
				linter = analyzer
			}
		} else if qodanaYaml.Linter != "" && qodanaYaml.Ide != "" {
			platform.ErrorMessage("You have both `linter:` (%s) and `ide:` (%s) fields set in %s. Modify the configuration file to keep one of them",
				qodanaYaml.Linter,
				qodanaYaml.Ide,
				qodanaYamlPath)
			os.Exit(1)
		}
		if qodanaYaml.Linter != "" {
			linter = qodanaYaml.Linter
		} else if qodanaYaml.Ide != "" {
			ide = qodanaYaml.Ide
		}
	}
	return linter, ide
}

func computeId(linter string, ide string, projectDir string) string {
	var analyzer string
	if linter != "" {
		analyzer = linter
	} else if ide != "" {
		analyzer = ide
	}
	length := 7
	projectAbs, _ := filepath.Abs(projectDir)
	id := fmt.Sprintf(
		"%s-%s",
		getHash(analyzer)[0:length+1],
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

	if platform.IsContainer() {
		return "/data/results"
	} else {
		return filepath.Join(linterDir, "results")
	}
}

func computeCacheDir(cacheDirFromCliOptions string, linterDir string) string {
	if cacheDirFromCliOptions != "" {
		return cacheDirFromCliOptions
	}

	if platform.IsContainer() {
		return "/data/results"
	} else {
		return filepath.Join(linterDir, "results")
	}
}
