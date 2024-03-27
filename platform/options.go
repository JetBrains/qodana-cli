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

package platform

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

const (
	IncrementalScriptLocalChanges = "local-changes"
	IncrementalScriptScope        = "scope"
)

// QodanaOptions is a struct that contains all the options to run a Qodana linter.
type QodanaOptions struct {
	ResultsDir                string
	CacheDir                  string
	ProjectDir                string
	ReportDir                 string
	CoverageDir               string
	Linter                    string
	Ide                       string
	SourceDirectory           string
	DisableSanity             bool
	ProfileName               string
	ProfilePath               string
	RunPromo                  string
	StubProfile               string // note: deprecated option
	Baseline                  string
	BaselineIncludeAbsent     bool
	SaveReport                bool
	ShowReport                bool
	Port                      int
	Property                  []string
	Script                    string
	FailThreshold             string
	Commit                    string
	DiffStart                 string
	DiffEnd                   string
	ForceIncrementalScript    string
	AnalysisId                string
	Env                       []string
	Volumes                   []string
	User                      string
	PrintProblems             bool
	GenerateCodeClimateReport bool
	SendBitBucketInsights     bool
	SkipPull                  bool
	ClearCache                bool
	ConfigName                string
	GitReset                  bool
	FullHistory               bool
	ApplyFixes                bool
	Cleanup                   bool
	FixesStrategy             string // note: deprecated option
	_id                       string
	LinterSpecific            interface{} // linter specific options
	LicensePlan               string
	NoStatistics              bool   // thirdparty common option
	Solution                  string // cdnet specific options
	Project                   string
	Configuration             string
	Platform                  string
	NoBuild                   bool
	CompileCommands           string // clang specific options
	ClangArgs                 string
	AnalysisTimeoutMs         int
	AnalysisTimeoutExitCode   int
	JvmDebugPort              int
}

func (o *QodanaOptions) LogOptions() {
	buffer := new(bytes.Buffer)
	w := new(tabwriter.Writer)
	w.Init(buffer, 0, 8, 2, '\t', 0)

	_, err := fmt.Fprintln(w, "Option\tValue\t")
	if err != nil {
		return
	}
	_, err = fmt.Fprintln(w, "------\t-----\t")
	if err != nil {
		return
	}

	value := reflect.ValueOf(o).Elem()
	typeInfo := value.Type()

	for i := 0; i < value.NumField(); i++ {
		fieldType := typeInfo.Field(i)
		if !unicode.IsUpper([]rune(fieldType.Name)[0]) {
			continue
		}
		fieldValue := value.Field(i)
		line := fmt.Sprintf("%s\t%v\t", fieldType.Name, fieldValue.Interface())
		_, err = fmt.Fprintln(w, line)
		if err != nil {
			return
		}
	}
	if err := w.Flush(); err != nil {
		return
	}
	log.Debug(buffer.String())
}

// GetToken default options function to obtain token (no user interaction)
func (o *QodanaOptions) GetToken() string {
	return o.LoadToken(false, o.RequiresToken(false), false)
}

func (o *QodanaOptions) FetchAnalyzerSettings() {
	if o.Linter == "" && o.Ide == "" {
		qodanaYaml := LoadQodanaYaml(o.ProjectDir, o.ConfigName)
		if qodanaYaml.Linter == "" && qodanaYaml.Ide == "" {
			WarningMessage(
				"No valid `linter:` or `ide:` field found in %s. Have you run %s? Running that for you...",
				PrimaryBold(o.ConfigName),
				PrimaryBold("qodana init"),
			)
			analyzer := GetAnalyzer(o.ProjectDir, o.ConfigName, o.GetToken())
			if IsNativeAnalyzer(analyzer) {
				o.Ide = analyzer
			} else {
				o.Linter = analyzer
			}
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

// Setenv sets the Qodana container environment variables if such variable was not set before.
func (o *QodanaOptions) Setenv(key string, value string) {
	for _, e := range o.Env {
		if strings.HasPrefix(e, key) {
			return
		}
	}
	if value != "" {
		o.Env = append(o.Env, fmt.Sprintf("%s=%s", key, value))
	}
}

// Getenv returns the Qodana container environment variables.
func (o *QodanaOptions) Getenv(key string) string {
	for _, e := range o.Env {
		if strings.HasPrefix(e, key) {
			return strings.TrimPrefix(e, key+"=")
		}
	}
	return ""
}

// Unsetenv unsets the Qodana container environment variables.
func (o *QodanaOptions) Unsetenv(key string) {
	for i, e := range o.Env {
		if strings.HasPrefix(e, key) {
			o.Env = append(o.Env[:i], o.Env[i+1:]...)
			return
		}
	}
}

func (o *QodanaOptions) StartHash() (string, error) {
	switch {
	case o.Commit == o.DiffStart:
		return o.Commit, nil
	case o.Commit == "":
		return o.DiffStart, nil
	case o.DiffStart == "":
		return o.Commit, nil
	default:
		return "", fmt.Errorf("conflicting CLI arguments: --commit=%s --diff-start=%s", o.Commit, o.DiffStart)
	}
}

func (o *QodanaOptions) Id() string {
	if o._id == "" {
		var analyzer string
		if o.Linter != "" {
			analyzer = o.Linter
		} else if o.Ide != "" {
			analyzer = o.Ide
		}
		if analyzer == "" {
			qYaml := LoadQodanaYaml(o.ProjectDir, o.ConfigName)
			if qYaml.Ide != "" {
				analyzer = qYaml.Linter
			} else if qYaml.Linter != "" {
				analyzer = qYaml.Ide
			}
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

func (o *QodanaOptions) GetQodanaSystemDir() string {
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
		o.GetQodanaSystemDir(),
		o.Id(),
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

func (o *QodanaOptions) LogDirPath() string {
	return filepath.Join(o.resultsDirPath(), "log")
}

func (o *QodanaOptions) ConfDirPath() string {
	if conf, ok := os.LookupEnv(QodanaConfEnv); ok {
		return conf
	}
	confDir := filepath.Join(o.GetLinterDir(), "config")
	return confDir
}

func (o *QodanaOptions) Properties() (map[string]string, []string) {
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

func (o *QodanaOptions) RequiresToken(isCommunityOrEap bool) bool {
	if os.Getenv(QodanaToken) != "" || o.Getenv(QodanaLicenseOnlyToken) != "" {
		return true
	}

	var analyzer string
	if o.Linter != "" {
		analyzer = o.Linter
	} else if o.Ide != "" {
		analyzer = o.Ide
	}

	if os.Getenv(QodanaLicense) != "" ||
		Contains(append(AllSupportedFreeImages, AllSupportedFreeCodes...), analyzer) ||
		strings.Contains(Lower(analyzer), "eap") ||
		isCommunityOrEap {
		return false
	}

	for _, e := range AllSupportedPaidCodes {
		if strings.HasPrefix(Image(e), o.Linter) || strings.HasPrefix(e, o.Ide) {
			return true
		}
	}

	return false
}

func (o *QodanaOptions) GetAnalysisTimeout() time.Duration {
	if o.AnalysisTimeoutMs <= 0 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(o.AnalysisTimeoutMs) * time.Millisecond
}

func (o *QodanaOptions) IsCommunity() bool {
	return o.LicensePlan == "COMMUNITY"
}

func (o *QodanaOptions) GetTmpResultsDir() string {
	return path.Join(o.ResultsDir, "tmp")
}

func (o *QodanaOptions) GetSarifPath() string {
	return path.Join(o.ResultsDir, "qodana.sarif.json")
}

func (o *QodanaOptions) GetShortSarifPath() string {
	return path.Join(o.ResultsDir, "qodana-short.sarif.json")
}
