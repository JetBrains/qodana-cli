/*
 * Copyright 2021-2022 JetBrains s.r.o.
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
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// QodanaYaml A standard qodana.yaml (or qodana.yml) format for Qodana configuration.
// https://github.com/JetBrains/qodana-profiles/blob/master/schemas/qodana-yaml-1.0.json
type QodanaYaml struct {
	// The qodana.yaml version of this log file.
	Version string `yaml:"version,omitempty"`

	// Linter to run.
	Linter string `yaml:"linter"`

	// Profile is the profile configuration for Qodana analysis.
	Profile Profile `yaml:"profile,omitempty"`

	// FailThreshold is a number of problems to fail the analysis (to exit from Qodana with code 255).
	FailThreshold int `yaml:"failThreshold,omitempty"`

	// Exclude property to disable the wanted checks on the wanted paths.
	Excludes []Exclude `yaml:"exclude,omitempty"`

	// Include property to enable the wanted checks.
	Includes []Include `yaml:"include,omitempty"`

	// Properties property to override IDE properties.
	Properties map[string]string `yaml:"properties,omitempty"`

	// Bootstrap contains a command to run in the container before the analysis starts.
	Bootstrap string `yaml:"bootstrap,omitempty"`

	// LicenseRules contains a list of license rules to apply for license checks.
	LicenseRules []LicenseRule `yaml:"licenseRules,omitempty"`

	// DependencyIgnores contains a list of dependencies to ignore for license checks in Qodana.
	DependencyIgnores []DependencyIgnore `yaml:"dependencyIgnores,omitempty"`

	// DependencyOverrides contains a list of dependencies metadata to override for license checks in Qodana.
	DependencyOverrides []DependencyOverride `yaml:"dependencyOverrides,omitempty"`

	// CustomDependencies contains a list of custom dependencies to add to license checks in Qodana.
	CustomDependencies []CustomDependency `yaml:"customDependencies,omitempty"`

	// Plugins property containing plugins to install.
	Plugins []Plugin `yaml:"plugins,omitempty"`
}

// Profile A profile is some template set of checks to run with Qodana analysis.
type Profile struct {
	// Name profile name to use.
	Name string `yaml:"name,omitempty"`

	// Path profile path to use.
	Path string `yaml:"path,omitempty"`
}

// Exclude A check id to disable.
type Exclude struct {
	// The name of check to exclude.
	Name string `yaml:"name"`

	// Relative to the project root path to disable analysis.
	Paths []string `yaml:"paths,omitempty"`
}

// Include A check id to enable.
type Include struct {
	// The name of check to exclude.
	Name string `yaml:"name"`
}

// Plugin to be installed during the Qodana run.
type Plugin struct {
	// Id plugin id to install.
	Id string `yaml:"id"`

	// Version plugin version to install.
	Version string `yaml:"plugins,omitempty"`
}

// DependencyIgnore is a dependency to ignore for license checks in Qodana
type DependencyIgnore struct {
	// Name is the name of the dependency to ignore.
	Name string `yaml:"name"`
}

// LicenseRule is a license rule to apply for license compatibility checks in Qodana
type LicenseRule struct {
	// Keys is the list of project license SPDX IDs.
	Keys []string `yaml:"keys"`

	// Allowed is the list of allowed dependency licenses for project licenses.
	Allowed []string `yaml:"allowed,omitempty"`

	// Prohibited is the list of prohibited dependency licenses for project licenses.
	Prohibited []string `yaml:"prohibited,omitempty"`
}

type DependencyOverride struct {
	// Name is dependency name.
	Name string `yaml:"name"`

	// Version is the dependency version.
	Version string `yaml:"version"`

	// Url is the dependency URL.
	Url string `yaml:"url,omitempty"`

	// License is the license of the dependency.
	Licenses []License `yaml:"licenses"`
}

type License struct {
	// Key is the SPDX ID of the license.
	Key string `yaml:"key"`

	// Url is the URL of the license.
	Url string `yaml:"url,omitempty"`
}

type CustomDependency struct {
	// Name is the name of the dependency.
	Name string `yaml:"name"`

	// Version is the dependency version.
	Version string `yaml:"version"`

	// Url is the dependency URL.
	Url string `yaml:"url,omitempty"`

	// License is the license of the dependency.
	Licenses []License `yaml:"licenses"`
}

// LoadQodanaYaml gets Qodana YAML from the project.
func LoadQodanaYaml(project string) *QodanaYaml {
	q := &QodanaYaml{}
	qodanaYamlPath := filepath.Join(project, "qodana.yaml")
	if _, err := os.Stat(qodanaYamlPath); errors.Is(err, os.ErrNotExist) {
		qodanaYamlPath = filepath.Join(project, "qodana.yml")
	}
	if _, err := os.Stat(qodanaYamlPath); errors.Is(err, os.ErrNotExist) {
		return q
	}
	yamlFile, err := ioutil.ReadFile(qodanaYamlPath)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, q)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return q
}

// sort makes QodanaYaml prettier.
func (q *QodanaYaml) sort() *QodanaYaml {
	sort.Slice(q.Includes, func(i, j int) bool {
		return lower(q.Includes[i].Name) < lower(q.Includes[j].Name)
	})
	sort.Slice(q.Excludes, func(i, j int) bool {
		return lower(q.Excludes[i].Name) < lower(q.Excludes[j].Name)
	})
	for _, rule := range q.LicenseRules {
		sort.Slice(rule.Keys, func(i, j int) bool {
			return lower(rule.Keys[i]) < lower(rule.Keys[j])
		})
		sort.Slice(rule.Allowed, func(i, j int) bool {
			return lower(rule.Allowed[i]) < lower(rule.Allowed[j])
		})
		sort.Slice(rule.Prohibited, func(i, j int) bool {
			return lower(rule.Prohibited[i]) < lower(rule.Prohibited[j])
		})
	}
	sort.Slice(q.DependencyIgnores, func(i, j int) bool {
		return lower(q.DependencyIgnores[i].Name) < lower(q.DependencyIgnores[j].Name)
	})
	sort.Slice(q.DependencyOverrides, func(i, j int) bool {
		return lower(q.DependencyOverrides[i].Name) < lower(q.DependencyOverrides[j].Name)
	})
	sort.Slice(q.CustomDependencies, func(i, j int) bool {
		return lower(q.CustomDependencies[i].Name) < lower(q.CustomDependencies[j].Name)
	})
	return q
}

// WriteQodanaYaml writes the qodana.yaml file to the given path.
func WriteQodanaYaml(path string, linters []string) {
	q := LoadQodanaYaml(path)
	if q.Version == "" {
		q.Version = "1.0"
	}
	q.sort()
	q.Linter = linters[0]
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)
	err := yamlEncoder.Encode(&q)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(filepath.Join(path, "qodana.yaml"), b.Bytes(), 0o600)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
}
