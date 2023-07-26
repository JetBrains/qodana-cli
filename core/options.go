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
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
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
	StubProfile           string
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
	_id                   string
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
		var linter string
		if o.Linter != "" {
			linter = o.Linter
		} else if o.Ide != "" {
			linter = o.Ide
		}
		length := 7
		projectAbs, _ := filepath.Abs(o.ProjectDir)
		o._id = fmt.Sprintf(
			"%s-%s",
			getHash(linter)[0:length+1],
			getHash(projectAbs)[0:length+1],
		)
	}
	return o._id
}

// ValidateToken checks if QODANA_TOKEN is set in CLI args, or environment or the system keyring, returns it's value.
func (o *QodanaOptions) ValidateToken(refresh bool) string {
	tokenFromCliArgs := o.getenv(qodanaToken)
	if tokenFromCliArgs != "" {
		log.Debug("Loaded token from CLI args environment")
		return tokenFromCliArgs
	}

	tokenFromEnv := os.Getenv(qodanaToken)
	if tokenFromEnv != "" {
		o.setenv(qodanaToken, os.Getenv(qodanaToken))
		log.Debug("Loaded token from the environment variable")
		return tokenFromEnv
	}

	log.Debugf("project id: %s", o.id())
	tokenFromKeychain, err := getCloudToken(o.id())
	if err == nil && tokenFromKeychain != "" {
		WarningMessage(
			"Got %s from the system keyring, declare %s env variable or run %s to override it",
			PrimaryBold(qodanaToken),
			PrimaryBold(qodanaToken),
			PrimaryBold("qodana init -f"),
		)
		o.setenv(qodanaToken, tokenFromKeychain)
		log.Debugf("Loaded token from the system keyring with id %s", o.id())
		if !refresh {
			return tokenFromKeychain
		}
	}

	if IsInteractive() {
		WarningMessage(emptyTokenMessage)
		token := setupToken(o.ProjectDir, o.id())
		if token != "" {
			log.Debugf("Loaded token from the user input, saved to the system keyring with id %s", o.id())
			o.setenv(qodanaToken, token)
			return token
		}
	}

	return ""
}

func (o *QodanaOptions) GetLinterDir() string {
	return filepath.Join(
		getQodanaSystemDir(),
		o.id(),
	)
}

func (o *QodanaOptions) ResultsDirPath() string {
	if o.ResultsDir == "" {
		if isDocker() {
			o.ResultsDir = "/data/results"
		} else {
			o.ResultsDir = filepath.Join(o.GetLinterDir(), "results")
		}
	}
	return o.ResultsDir
}

func (o *QodanaOptions) CacheDirPath() string {
	if o.CacheDir == "" {
		if isDocker() {
			o.CacheDir = "/data/cache"
		} else {
			o.CacheDir = filepath.Join(o.GetLinterDir(), "cache")
		}
	}
	return o.CacheDir
}

func (o *QodanaOptions) ReportDirPath() string {
	if o.ReportDir == "" {
		if isDocker() {
			o.ReportDir = "/data/results/report"
		} else {
			o.ReportDir = filepath.Join(o.ResultsDirPath(), "report")
		}
	}
	return o.ReportDir
}

func (o *QodanaOptions) stabProfilePath() string {
	return filepath.Join(o.CacheDirPath(), "profile.xml")
}

func (o *QodanaOptions) reportResultsPath() string {
	return filepath.Join(o.ReportDirPath(), "results")
}

func (o *QodanaOptions) logDirPath() string {
	return filepath.Join(o.ResultsDirPath(), "log")
}

func (o *QodanaOptions) vmOptionsPath() string {
	return filepath.Join(o.confDirPath(), "ide.vmoptions")
}

func (o *QodanaOptions) confDirPath() string {
	if conf, ok := os.LookupEnv(QodanaConfEnv); ok {
		return conf
	}
	confDir := filepath.Join(o.GetLinterDir(), "config")
	return confDir
}

func (o *QodanaOptions) appInfoXmlPath(ideBinDir string) string {
	if _, err := os.Stat(filepath.Join(ideBinDir, qodanaAppInfoFilename)); err != nil {
		return filepath.Join(o.confDirPath(), qodanaAppInfoFilename)
	}
	return filepath.Join(ideBinDir, qodanaAppInfoFilename)
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
