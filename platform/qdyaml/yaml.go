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

package qdyaml

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GetQodanaYamlPath returns the path to qodana.yaml or qodana.yml
func GetQodanaYamlPath(project string) (string, error) {
	qodanaYamlPath := filepath.Join(project, "qodana.yaml")
	if _, err := os.Stat(qodanaYamlPath); errors.Is(err, os.ErrNotExist) {
		qodanaYamlPath = filepath.Join(project, "qodana.yml")
	}
	if _, err := os.Stat(qodanaYamlPath); errors.Is(err, os.ErrNotExist) {
		return "", errors.New("qodana.yaml or qodana.yml not found")
	}
	return qodanaYamlPath, nil
}

// GetQodanaYaml returns a parsed qodana.yaml or qodana.yml or error if not found/invalid
func GetQodanaYaml(project string) (QodanaYaml, error) {
	q := &QodanaYaml{}
	qodanaYamlPath, err := GetQodanaYamlPath(project)
	if err != nil {
		return *q, err
	}
	yamlFile, err := os.ReadFile(qodanaYamlPath)
	if err != nil {
		return *q, err
	}
	err = yaml.Unmarshal(yamlFile, q)
	if err != nil {
		return *q, fmt.Errorf("not a valid qodana.yaml: %w", err)
	}
	return *q, nil
}

// GetQodanaYamlOrDefault reads qodana.yaml or qodana.yml and returns an empty config if not found or invalid
func GetQodanaYamlOrDefault(project string) QodanaYaml {
	q, err := GetQodanaYaml(project)
	if err != nil {
		log.Printf("Problem loading qodana.yaml: %v ", err)
	}
	return q
}

// QodanaYaml A standard qodana.yaml (or qodana.yml) format for Qodana configuration.
// https://github.com/JetBrains/qodana-profiles/blob/master/schemas/qodana-yaml-1.0.json
type QodanaYaml struct {
	// The qodana.yaml version of this log file.
	Version string `yaml:"version,omitempty"`

	// Profile is the profile configuration for Qodana analysis (either a profile name or a profile path).
	Profile Profile `yaml:"profile,omitempty"`

	// FailThreshold is a number of problems to fail the analysis (to exit from Qodana with code 255).
	FailThreshold *int `yaml:"failThreshold,omitempty"`

	// Script is the run scenario. 'default' by default
	Script Script `yaml:"script,omitempty"`

	// Clude property to disable the wanted checks on the wanted paths.
	Excludes []Clude `yaml:"exclude,omitempty"`

	// Include property to enable the wanted checks.
	Includes []Clude `yaml:"include,omitempty"`

	// Linter to run.
	Linter string `yaml:"linter,omitempty"`

	// IDE to run.
	Ide string `yaml:"ide,omitempty"`

	// Bootstrap contains a command to run in the container before the analysis starts.
	Bootstrap string `yaml:"bootstrap,omitempty"`

	// Properties property to override IDE properties.
	Properties map[string]string `yaml:"properties,omitempty"`

	// LicenseRules contains a list of license rules to apply for license checks.
	LicenseRules []LicenseRule `yaml:"licenseRules,omitempty"`

	// DependencyIgnores contains a list of dependencies to ignore for license checks in Qodana.
	DependencyIgnores []DependencyIgnore `yaml:"dependencyIgnores,omitempty"`

	// DependencyOverrides contains a list of dependencies metadata to override for license checks in Qodana.
	DependencyOverrides []DependencyOverride `yaml:"dependencyOverrides,omitempty"`

	// Overrides the licenses attached to the project
	ProjectLicenses []LicenseOverride `yaml:"projectLicenses,omitempty"`

	// CustomDependencies contains a list of custom dependencies to add to license checks in Qodana.
	CustomDependencies []CustomDependency `yaml:"customDependencies,omitempty"`

	// Plugins property containing plugins to install.
	Plugins []Plugin `yaml:"plugins,omitempty"`

	// DotNet is the configuration for .NET solutions and projects (either a solution name or a project name).
	DotNet DotNet `yaml:"dotnet,omitempty"`

	// ProjectJdk is the configuration for the project JDK.
	ProjectJdk string `yaml:"projectJDK,omitempty"`

	// Php is the configuration for PHP projects.
	Php Php `yaml:"php,omitempty"`

	// DisableSanityInspections property to disable sanity inspections.
	DisableSanityInspections string `yaml:"disableSanityInspections,omitempty"`

	// FixesStrategy property to set fixes strategy. Can be none (default), apply, cleanup.
	FixesStrategy string `yaml:"fixesStrategy,omitempty"`

	// RunPromoInspections property to run promo inspections.
	RunPromoInspections string `yaml:"runPromoInspections,omitempty"`

	// IncludeAbsent property to include absent problems from baseline.
	IncludeAbsent string `yaml:"includeAbsent,omitempty"`

	// MaxRuntimeNotifications property defines maximum amount of internal errors to collect in the report
	MaxRuntimeNotifications int `yaml:"maxRuntimeNotifications,omitempty"`

	// FailOnErrorNotification property defines whether to fail the run when any internal error was encountered. In that case, the program returns exit code 70
	FailOnErrorNotification bool `yaml:"failOnErrorNotification,omitempty"`

	// FailureConditions configures individual failure conditions. Absent properties will not be checked
	FailureConditions FailureConditions `yaml:"failureConditions,omitempty"`

	// DependencySbomExclude property to define which dependencies to exclude from the generated SBOM report
	DependencySbomExclude []DependencyIgnore `yaml:"dependencySbomExclude,omitempty"`

	// ModulesToAnalyze property to define which submodules to include. Omitting this key will include all submodules.
	ModulesToAnalyze []ModuleToAnalyze `yaml:"modulesToAnalyze,omitempty"`

	// AnalyzeDevDependencies property whether to include dev dependencies in the analysis
	AnalyzeDevDependencies bool `yaml:"analyzeDevDependencies,omitempty"`

	// EnablePackageSearch property to start using package-search service for fetching license data for dependencies (only for jvm libraries)
	EnablePackageSearch bool `yaml:"enablePackageSearch,omitempty"`

	// RaiseLicenseProblems property to show license problems like other inspections.
	RaiseLicenseProblems bool `yaml:"raiseLicenseProblems,omitempty"`
}

// WriteConfig writes QodanaYaml to the given path.
func (q *QodanaYaml) WriteConfig(path string) error {
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)
	err := yamlEncoder.Encode(&q)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, b.Bytes(), 0o600)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
	return nil
}

// Profile A profile is some template set of checks to run with Qodana analysis.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type Profile struct {
	// Name profile name to use.
	Name string `yaml:"name,omitempty"`

	// Path profile path to use.
	Path string `yaml:"path,omitempty"`
}

// Clude A check id to enable/disable for include/exclude YAML field.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type Clude struct {
	// The name of check to include/exclude.
	Name string `yaml:"name"`

	// Relative to the project root path to enable/disable analysis.
	Paths []string `yaml:"paths,omitempty"`
}

// Plugin to be installed during the Qodana run.
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type Plugin struct {
	// Id plugin id to install.
	Id string `yaml:"id"`
}

// DependencyIgnore is a dependency to ignore for license checks in Qodana
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type DependencyIgnore struct {
	// Name is the name of the dependency to ignore.
	Name string `yaml:"name"`
}

// LicenseRule is a license rule to apply for license compatibility checks in Qodana
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type LicenseRule struct {
	// Keys is the list of project license SPDX IDs.
	Keys []string `yaml:"keys"`

	// Allowed is the list of allowed dependency licenses for project licenses.
	Allowed []string `yaml:"allowed,omitempty"`

	// Prohibited is the list of prohibited dependency licenses for project licenses.
	Prohibited []string `yaml:"prohibited,omitempty"`
}

// ModuleToAnalyze is a submodule to include in the analysis
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type ModuleToAnalyze struct {
	// Name corresponds to the JSON schema field "name".
	Name *string `yaml:"name,omitempty"`
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
type DependencyOverride struct {
	// Name is dependency name.
	Name string `yaml:"name"`

	// Version is the dependency version.
	Version string `yaml:"version"`

	// Url is the dependency URL.
	Url string `yaml:"url,omitempty"`

	// LicenseOverride is the license of the dependency.
	Licenses []LicenseOverride `yaml:"licenses"`
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
type LicenseOverride struct {
	// Key is the SPDX ID of the license.
	Key string `yaml:"key"`

	// Url is the URL of the license.
	Url string `yaml:"url,omitempty"`
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
type CustomDependency struct {
	// Name is the name of the dependency.
	Name string `yaml:"name"`

	// Version is the dependency version.
	Version string `yaml:"version"`

	// Url is the dependency URL.
	Url string `yaml:"url,omitempty"`

	// LicenseOverride is the license of the dependency.
	Licenses []LicenseOverride `yaml:"licenses"`
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
type DotNet struct {
	// Solution is the name of a .NET solution inside the Qodana project.
	Solution string `yaml:"solution,omitempty"`

	// Project is the name of a .NET project inside the Qodana project.
	Project string `yaml:"project,omitempty"`

	// Configuration is the configuration in which .NET project should be opened by Qodana.
	Configuration string `yaml:"configuration,omitempty"`

	// Platform is the target platform in which .NET project should be opened by Qodana.
	Platform string `yaml:"platform,omitempty"`

	// Frameworks is a semicolon-separated list of target framework monikers (TFM) to be analyzed.
	Frameworks string `yaml:"frameworks,omitempty"`
}

type FailureConditions struct {
	// SeverityThresholds corresponds to the JSON schema field "severityThresholds".
	SeverityThresholds *SeverityThresholds `yaml:"severityThresholds,omitempty"`

	// TestCoverageThresholds corresponds to the JSON schema field
	// "testCoverageThresholds".
	TestCoverageThresholds *CoverageThresholds `yaml:"testCoverageThresholds,omitempty"`
}

// SeverityThresholds Configures maximum thresholds for different problem severities. Absent properties are not checked. If a baseline is given, only new results are counted
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type SeverityThresholds struct {
	// The run fails if the total amount of results exceeds this number.
	Any *int `yaml:"any,omitempty"`

	// The run fails if the amount results with severity CRITICAL exceeds this number.
	Critical *int `yaml:"critical,omitempty"`

	// The run fails if the amount results with severity HIGH exceeds this number.
	High *int `yaml:"high,omitempty"`

	// The run fails if the amount results with severity INFO exceeds this number.
	Info *int `yaml:"info,omitempty"`

	// The run fails if the amount results with severity LOW exceeds this number.
	Low *int `yaml:"low,omitempty"`

	// The run fails if the amount results with severity MODERATE exceeds this number.
	Moderate *int `yaml:"moderate,omitempty"`
}

// CoverageThresholds Configures minimum thresholds for test coverage metrics. Absent properties are not checked
//
//goland:noinspection GoUnnecessarilyExportedIdentifiers
type CoverageThresholds struct {
	// The run fails if the percentage of fresh lines covered is lower than this
	// number
	Fresh *int `json:"fresh,omitempty" yaml:"fresh,omitempty" mapstructure:"fresh,omitempty"`

	// The run fails if the percentage of total lines covered is lower than this
	// number.
	Total *int `json:"total,omitempty" yaml:"total,omitempty" mapstructure:"total,omitempty"`
}

type Script struct {
	Name       string                 `yaml:"name,omitempty"`
	Parameters map[string]interface{} `yaml:"parameters,omitempty"`
}

// IsEmpty checks whether the .NET configuration is empty or not.
func (d DotNet) IsEmpty() bool {
	return d.Solution == "" && d.Project == ""
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
type Php struct {
	// Version is the PHP version to use for the analysis.
	Version string `yaml:"version,omitempty"`
}

// FindDefaultQodanaYaml checks whether qodana.yaml exists or not
func FindDefaultQodanaYaml(project string) string {
	filename := "qodana.yml"
	if info, _ := os.Stat(filepath.Join(project, filename)); info != nil {
		return filename
	} else {
		return "qodana.yaml"
	}
}

func GetQodanaYamlPathWithProject(project string, filename string) string {
	if filename == "" {
		filename = FindDefaultQodanaYaml(project)
	}
	qodanaYamlPath := filepath.Join(project, filename)
	return qodanaYamlPath
}

// LoadQodanaYaml gets Qodana YAML from the project.
func LoadQodanaYaml(project string, filename string) QodanaYaml {
	qodanaYamlPath := GetQodanaYamlPathWithProject(project, filename)
	q := LoadQodanaYamlByFullPath(qodanaYamlPath)
	return q
}

func LoadQodanaYamlByFullPath(fullPath string) QodanaYaml {
	q := &QodanaYaml{}
	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return *q
	}
	yamlFile, err := os.ReadFile(fullPath)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, q)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return *q
}

// Sort makes QodanaYaml prettier.
func (q *QodanaYaml) Sort() *QodanaYaml {
	sort.Slice(
		q.Includes, func(i, j int) bool {
			return utils.Lower(q.Includes[i].Name) < utils.Lower(q.Includes[j].Name)
		},
	)
	sort.Slice(
		q.Excludes, func(i, j int) bool {
			return utils.Lower(q.Excludes[i].Name) < utils.Lower(q.Excludes[j].Name)
		},
	)
	for _, rule := range q.LicenseRules {
		sort.Slice(
			rule.Keys, func(i, j int) bool {
				return utils.Lower(rule.Keys[i]) < utils.Lower(rule.Keys[j])
			},
		)
		sort.Slice(
			rule.Allowed, func(i, j int) bool {
				return utils.Lower(rule.Allowed[i]) < utils.Lower(rule.Allowed[j])
			},
		)
		sort.Slice(
			rule.Prohibited, func(i, j int) bool {
				return utils.Lower(rule.Prohibited[i]) < utils.Lower(rule.Prohibited[j])
			},
		)
	}
	sort.Slice(
		q.DependencyIgnores, func(i, j int) bool {
			return utils.Lower(q.DependencyIgnores[i].Name) < utils.Lower(q.DependencyIgnores[j].Name)
		},
	)
	sort.Slice(
		q.DependencyOverrides, func(i, j int) bool {
			return utils.Lower(q.DependencyOverrides[i].Name) < utils.Lower(q.DependencyOverrides[j].Name)
		},
	)
	sort.Slice(
		q.CustomDependencies, func(i, j int) bool {
			return utils.Lower(q.CustomDependencies[i].Name) < utils.Lower(q.CustomDependencies[j].Name)
		},
	)
	sort.Slice(
		q.Plugins, func(i, j int) bool {
			return utils.Lower(q.Plugins[i].Id) < utils.Lower(q.Plugins[j].Id)
		},
	)
	return q
}

func (q *QodanaYaml) IsDotNet() bool {
	return strings.Contains(q.Linter, "dotnet") || strings.Contains(q.Linter, "cdnet") || strings.Contains(
		q.Ide,
		"QDNET",
	)
}

// WriteQodanaLinterToYamlFile adds the linter to the qodana.yaml file.
func WriteQodanaLinterToYamlFile(path string, linter string, filename string, allProductCodes []string) {
	q := LoadQodanaYaml(path, filename)
	if q.Version == "" {
		q.Version = "1.0"
	}
	q.Sort()
	if utils.Contains(allProductCodes, linter) {
		q.Ide = linter
	} else {
		q.Linter = linter
	}
	err := q.WriteConfig(filepath.Join(path, filename))
	if err != nil {
		log.Fatalf("writeConfig: %v", err)
	}
}

// setQodanaDotNet adds the .NET configuration to the qodana.yaml file.
func SetQodanaDotNet(path string, dotNet *DotNet, filename string) bool {
	q := LoadQodanaYaml(path, filename)
	q.DotNet = *dotNet
	err := q.WriteConfig(filepath.Join(path, filename))
	if err != nil {
		log.Fatalf("writeConfig: %v", err)
	}
	return true
}
