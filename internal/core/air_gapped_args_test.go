/*
 * Copyright 2021-2026 JetBrains s.r.o.
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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func ideArgs(t *testing.T, analyser product.Analyzer, airGapped bool) []string {
	context := corescan.ContextBuilder{
		Analyser:   analyser,
		ResultsDir: t.TempDir(),
		Prod:       product.Product{Version: "2026.2"},
		AirGapped:  airGapped,
	}.Build()
	return GetIdeArgs(context)
}

// TestAirGappedPassesTheOptionToTheLinter verifies a native run: the linter gets --air-gapped, as it gets --disable-sanity.
func TestAirGappedPassesTheOptionToTheLinter(t *testing.T) {
	assert.Contains(t, ideArgs(t, product.DotNetLinter.NativeAnalyzer(), true), "--air-gapped")
}

// TestAirGappedPassesTheOptionToTheContainerCli verifies Docker mode: the CLI in the container gets --air-gapped
// as its own option and passes it on to the linter.
func TestAirGappedPassesTheOptionToTheContainerCli(t *testing.T) {
	assert.Contains(t, ideArgs(t, product.DotNetLinter.DockerAnalyzer(), true), "--air-gapped")
}

func TestNoAirGappedOptionWithoutTheMode(t *testing.T) {
	assert.NotContains(t, ideArgs(t, product.DotNetLinter.NativeAnalyzer(), false), "--air-gapped")
}

// TestAirGappedKeepsTheStatisticsSetting verifies that the CLI does not override FUS in air-gapped mode:
// the linter turns off the FUS upload itself.
func TestAirGappedKeepsTheStatisticsSetting(t *testing.T) {
	context := corescan.ContextBuilder{
		ResultsDir: t.TempDir(),
		Prod:       product.Product{Version: "2026.2"},
		Property:   []string{"idea.headless.enable.statistics=true"},
		AirGapped:  true,
	}.Build()

	assert.Contains(t, GetScanProperties(context), "-Didea.headless.enable.statistics=true")
}

// TestAirGappedFromQodanaYamlSuppressesTheStatisticsReport verifies that airGapped in qodana.yaml without --air-gapped
// also adds idea.suppress.statistics.report to the vmoptions, so the IDE starts no FUS uploader on exit.
func TestAirGappedFromQodanaYamlSuppressesTheStatisticsReport(t *testing.T) {
	context := corescan.ContextBuilder{
		ResultsDir:       t.TempDir(),
		Prod:             product.Product{Version: "2026.2"},
		QodanaYamlConfig: corescan.YamlConfig(qdyaml.QodanaYaml{AirGapped: true}),
	}.Build()

	assert.Contains(t, GetScanProperties(context), "-Didea.suppress.statistics.report=true")
}
