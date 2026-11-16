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

package corescan

import (
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/startup"
	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func airGapped(options platformcmd.CliOptions, yaml qdyaml.QodanaYaml) bool {
	return CreateContext(options, commoncontext.Context{}, startup.PreparedHost{}, YamlConfig(yaml), "").AirGapped()
}

func TestAirGappedFromOption(t *testing.T) {
	assert.True(t, airGapped(platformcmd.CliOptions{AirGapped: true}, qdyaml.QodanaYaml{}))
}

func TestAirGappedFromQodanaYaml(t *testing.T) {
	assert.True(t, airGapped(platformcmd.CliOptions{}, qdyaml.QodanaYaml{AirGapped: true}))
}

// TestAirGappedFromCliProperty verifies the case of the CLI in a container: the CLI on the host passes the mode
// as --property, and the CLI in the container must know it too.
func TestAirGappedFromCliProperty(t *testing.T) {
	assert.True(t, airGapped(platformcmd.CliOptions{Property: []string{"qodana.air.gapped=TRUE"}}, qdyaml.QodanaYaml{}))
}

func TestAirGappedFromQodanaYamlProperties(t *testing.T) {
	yaml := qdyaml.QodanaYaml{Properties: map[string]string{"qodana.air.gapped": "true"}}
	assert.True(t, airGapped(platformcmd.CliOptions{}, yaml))
}

func TestNotAirGappedWithoutSources(t *testing.T) {
	assert.False(t, airGapped(platformcmd.CliOptions{Property: []string{"qodana.air.gapped=1"}}, qdyaml.QodanaYaml{}))
}

// TestSecondStageOfScopedScriptKeepsAirGapped verifies that the second linter run of the scoped script
// gets the same air-gapped mode as the first one.
func TestSecondStageOfScopedScriptKeepsAirGapped(t *testing.T) {
	resultsDir := t.TempDir()
	context := ContextBuilder{ResultsDir: resultsDir, LogDir: filepath.Join(resultsDir, "log"), AirGapped: true}.
		Build().
		SecondStageOfScopedScript(filepath.Join(resultsDir, "scope.json"), filepath.Join(resultsDir, "start.json"))

	assert.True(t, context.AirGapped())
}
