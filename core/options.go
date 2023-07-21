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
	Linter                string
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

func (o *QodanaOptions) Id() string {
	if o._id == "" {
		length := 7
		projectAbs, _ := filepath.Abs(o.ProjectDir)
		o._id = fmt.Sprintf(
			"%s-%s",
			getHash(o.Linter)[0:length+1],
			getHash(projectAbs)[0:length+1],
		)
	}
	return o._id
}

// ValidateToken checks if QODANA_TOKEN is set in CLI args, or environment or the system keyring.
func (o *QodanaOptions) ValidateToken(refresh bool) {
	if o.getenv(qodanaToken) != "" {
		log.Debug("Loaded token from CLI args environment")
		return
	}

	tokenFromEnv := os.Getenv(qodanaToken)
	if tokenFromEnv != "" {
		o.setenv(qodanaToken, os.Getenv(qodanaToken))
		log.Debug("Loaded token from the environment variable")
		return
	}

	log.Debugf("project id: %s", o.Id())
	tokenFromKeychain, err := getCloudToken(o.Id())
	if err == nil && tokenFromKeychain != "" {
		WarningMessage(
			"Got %s from the system keyring, declare %s env variable or run %s to override it",
			PrimaryBold(qodanaToken),
			PrimaryBold(qodanaToken),
			PrimaryBold("qodana init -f"),
		)
		o.setenv(qodanaToken, tokenFromKeychain)
		log.Debugf("Loaded token from the system keyring with id %s", o.Id())
		if !refresh {
			return
		}
	}

	if IsInteractive() {
		WarningMessage("%s is not set – Qodana (non-EAP) linters require token to be configured", PrimaryBold(qodanaToken))
		token := setupToken(o.ProjectDir, o.Id())
		if token != "" {
			log.Debugf("Loaded token from the user input, saved to the system keyring with id %s", o.Id())
			o.setenv(qodanaToken, token)
		}
	}
}

func (o *QodanaOptions) GetLinterDir() string {
	return filepath.Join(
		getQodanaSystemDir(),
		o.Id(),
	)
}
