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

package product

import (
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	log "github.com/sirupsen/logrus"
	"strings"
)

type Analyzer interface {
	GetLinter() Linter
	IsContainer() bool
	IsEAP() bool
	Name() string
	DownloadDist() bool
	InitYaml(yaml qdyaml.QodanaYaml) qdyaml.QodanaYaml
}

type DockerAnalyzer struct {
	Linter Linter
	Image  string
}

func (a *DockerAnalyzer) IsContainer() bool {
	return true
}

func (a *DockerAnalyzer) IsEAP() bool {
	return strings.Contains(strings.ToLower(a.Image), "eap")
}

func (a *DockerAnalyzer) Name() string {
	return a.Image
}

func (a *DockerAnalyzer) GetLinter() Linter {
	return a.Linter
}

func (a *DockerAnalyzer) DownloadDist() bool {
	return false
}

func (a *DockerAnalyzer) InitYaml(yaml qdyaml.QodanaYaml) qdyaml.QodanaYaml {
	yaml.Linter = a.Image
	return yaml
}

type NativeAnalyzer struct {
	Linter Linter
	Ide    string
}

func (a *NativeAnalyzer) IsContainer() bool {
	return false
}

func (a *NativeAnalyzer) IsEAP() bool {
	return strings.Contains(strings.ToLower(a.Ide), "eap")
}

func (a *NativeAnalyzer) Name() string {
	return a.Ide
}

func (a *NativeAnalyzer) GetLinter() Linter {
	return a.Linter
}

func (a *NativeAnalyzer) DownloadDist() bool {
	return true
}

func (a *NativeAnalyzer) InitYaml(yaml qdyaml.QodanaYaml) qdyaml.QodanaYaml {
	yaml.Ide = a.Ide
	return yaml
}

type PathNativeAnalyzer struct {
	Linter Linter
	Path   string
	IsEap  bool
}

func (a *PathNativeAnalyzer) IsContainer() bool {
	return false
}

func (a *PathNativeAnalyzer) IsEAP() bool {
	return strings.Contains(strings.ToLower(a.Path), "eap")
}

func (a *PathNativeAnalyzer) Name() string {
	return a.Path
}

func (a *PathNativeAnalyzer) GetLinter() Linter {
	return a.Linter
}

func (a *PathNativeAnalyzer) DownloadDist() bool {
	return false
}

func (a *PathNativeAnalyzer) InitYaml(_ qdyaml.QodanaYaml) qdyaml.QodanaYaml {
	log.Fatalf("Customised path can't be stored to Yaml")
	return qdyaml.QodanaYaml{}
}
