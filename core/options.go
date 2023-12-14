/*
 * Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

// QodanaOptions is a struct that contains all the options to run a Qodana linter.
type QodanaOptions struct {
	ResultsDir            string
	CacheDir              string
	ProjectDir            string
	ReportDir             string
	CoverageDir           string
	Linter                string
	Ide                   string
	SourceDirectory       string
	DisableSanity         bool
	ProfileName           string
	ProfilePath           string
	RunPromo              string
	StubProfile           string // note: deprecated option
	Baseline              string
	BaselineIncludeAbsent bool
	SaveReport            bool
	ShowReport            bool
	Port                  int
	Property              []string
	Script                string
	FailThreshold         string
	Commit                string
	AnalysisId            string
	Env                   []string
	Volumes               []string
	User                  string
	PrintProblems         bool
	SkipPull              bool
	ClearCache            bool
	YamlName              string
	GitReset              bool
	FullHistory           bool
	ApplyFixes            bool
	Cleanup               bool
	FixesStrategy         string // note: deprecated option
	_id                   string
	NoStatistics          bool   // thirdparty common option
	Solution              string // cdnet specific options
	Project               string
	Configuration         string
	Platform              string
	NoBuild               bool
	CompileCommands       string // clang specific options
	ClangArgs             string
}

func (o *QodanaOptions) FetchAnalyzerSettings() {
	if o.Linter == "" && o.Ide == "" {
		qodanaYaml := LoadQodanaYaml(o.ProjectDir, o.YamlName)
		if qodanaYaml.Linter == "" && qodanaYaml.Ide == "" {
			WarningMessage(
				"No valid `linter:` field found in %s. Have you run %s? Running that for you...",
				PrimaryBold(o.YamlName),
				PrimaryBold("qodana init"),
			)
			o.Linter = GetLinter(o.ProjectDir, o.YamlName)
			EmptyMessage()
		} else {
			o.Linter = qodanaYaml.Linter
		}
		if o.Ide == "" {
			o.Ide = qodanaYaml.Ide
		}
	}
	o.ResultsDir = o.resultsDirPath()
	o.ReportDir = o.reportDirPath()
	o.CacheDir = o.cacheDirPath()
}

// setenv sets the Qodana container environment variables if such variable was not set before.
func (o *QodanaOptions) setenv(key string, value string) {
	for _, e := range o.Env {
		if strings.HasPrefix(e, key) {
			return
		}
	}
	if value != "" {
		o.Env = append(o.Env, fmt.Sprintf("%s=%s", key, value))
	}
}

// getenv returns the Qodana container environment variables.
func (o *QodanaOptions) getenv(key string) string {
	for _, e := range o.Env {
		if strings.HasPrefix(e, key) {
			return strings.TrimPrefix(e, key+"=")
		}
	}
	return ""
}

// unsetenv unsets the Qodana container environment variables.
func (o *QodanaOptions) unsetenv(key string) {
	for i, e := range o.Env {
		if strings.HasPrefix(e, key) {
			o.Env = append(o.Env[:i], o.Env[i+1:]...)
			return
		}
	}
}

func (o *QodanaOptions) id() string {
	if o._id == "" {
		var analyzer string
		if o.Linter != "" {
			analyzer = o.Linter
		} else if o.Ide != "" {
			analyzer = o.Ide
		}
		if analyzer == "" {
			analyzer = LoadQodanaYaml(o.ProjectDir, o.YamlName).Linter
		}
		length := 7
		projectAbs, _ := filepath.Abs(o.ProjectDir)
		o._id = fmt.Sprintf(
			"%s-%s",
			getHash(analyzer)[0:length+1],
			getHash(projectAbs)[0:length+1],
		)
	}
	return o._id
}

func (o *QodanaOptions) getQodanaSystemDir() string {
	if o.CacheDir != "" {
		return filepath.Dir(filepath.Dir(o.CacheDir))
	}

	userCacheDir, _ := os.UserCacheDir()
	return filepath.Join(
		userCacheDir,
		"JetBrains",
		"Qodana",
	)
}

func (o *QodanaOptions) GetLinterDir() string {
	return filepath.Join(
		o.getQodanaSystemDir(),
		o.id(),
	)
}

func (o *QodanaOptions) resultsDirPath() string {
	if o.ResultsDir == "" {
		if IsContainer() {
			o.ResultsDir = "/data/results"
		} else {
			o.ResultsDir = filepath.Join(o.GetLinterDir(), "results")
		}
	}
	return o.ResultsDir
}

func (o *QodanaOptions) cacheDirPath() string {
	if o.CacheDir == "" {
		if IsContainer() {
			o.CacheDir = "/data/cache"
		} else {
			o.CacheDir = filepath.Join(o.GetLinterDir(), "cache")
		}
	}
	return o.CacheDir
}

func (o *QodanaOptions) reportDirPath() string {
	if o.ReportDir == "" {
		if IsContainer() {
			o.ReportDir = "/data/results/report"
		} else {
			o.ReportDir = filepath.Join(o.resultsDirPath(), "report")
		}
	}
	return o.ReportDir
}

func (o *QodanaOptions) CoverageDirPath() string {
	if o.CoverageDir == "" {
		if IsContainer() {
			o.CoverageDir = "/data/coverage"
		} else {
			o.CoverageDir = filepath.Join(o.ProjectDir, ".qodana", "code-coverage")
		}
	}
	return o.CoverageDir
}

func (o *QodanaOptions) ReportResultsPath() string {
	return filepath.Join(o.reportDirPath(), "results")
}

func (o *QodanaOptions) logDirPath() string {
	return filepath.Join(o.resultsDirPath(), "log")
}

func (o *QodanaOptions) vmOptionsPath() string {
	return filepath.Join(o.ConfDirPath(), "ide.vmoptions")
}

func (o *QodanaOptions) ConfDirPath() string {
	if conf, ok := os.LookupEnv(QodanaConfEnv); ok {
		return conf
	}
	confDir := filepath.Join(o.GetLinterDir(), "config")
	return confDir
}

func (o *QodanaOptions) appInfoXmlPath(ideBinDir string) string {
	appInfoPath := filepath.Join(ideBinDir, qodanaAppInfoFilename)
	if _, err := os.Stat(appInfoPath); err != nil && o.AnalysisId != "FAKE" {
		log.Fatalf("%s should exist in IDE directory %s. Unsupported IDE detected, exiting.", qodanaAppInfoFilename, ideBinDir)
	}
	return appInfoPath
}

func (o *QodanaOptions) properties() (map[string]string, []string) {
	var flagsArr []string
	props := map[string]string{}
	for _, arg := range o.Property {
		kv := strings.Split(arg, "=")
		if len(kv) == 2 {
			props[kv[0]] = kv[1]
		} else {
			flagsArr = append(flagsArr, arg)
		}
	}
	return props, flagsArr
}

func (o *QodanaOptions) RequiresToken() bool {
	if os.Getenv(QodanaToken) != "" || o.getenv(QodanaLicenseOnlyToken) != "" {
		return true
	}

	var analyzer string
	if o.Linter != "" {
		analyzer = o.Linter
	} else if o.Ide != "" {
		analyzer = o.Ide
	}

	if os.Getenv(QodanaLicense) != "" ||
		Contains(append(allSupportedFreeImages, allSupportedFreeCodes...), analyzer) ||
		strings.Contains(lower(analyzer), "eap") ||
		Prod.IsCommunity() || Prod.EAP {
		return false
	}

	for _, e := range allSupportedPaidCodes {
		if strings.HasPrefix(Image(e), o.Linter) || strings.HasPrefix(e, o.Ide) {
			return true
		}
	}

	return false
}

func (o *QodanaOptions) fixesSupported() bool {
	return o.guessProduct() != QDNET && o.guessProduct() != QDNETC && o.guessProduct() != QDCL
}

func (o *QodanaOptions) guessProduct() string {
	if o.Ide != "" {
		productCode := o.Ide
		if strings.HasSuffix(productCode, EapSuffix) {
			productCode = strings.TrimSuffix(productCode, EapSuffix)
		}
		if _, ok := Products[productCode]; ok {
			return productCode
		}
		return ""
	} else if o.Linter != "" {
		// if Linter contains registry.jetbrains.team/p/sa/containers/ or https://registry.jetbrains.team/p/sa/containers/
		// then replace it with jetbrains/ and do the comparison
		linter := o.Linter
		if strings.HasPrefix(linter, "https://") {
			linter = strings.TrimPrefix(linter, "https://")
		}
		if strings.HasPrefix(linter, "registry.jetbrains.team/p/sa/containers/") {
			linter = strings.TrimPrefix(linter, "registry.jetbrains.team/p/sa/containers/")
			linter = "jetbrains/" + linter
		}
		for k, v := range DockerImageMap {
			if strings.HasPrefix(linter, v) {
				return k
			}
		}
	}
	return ""
}
