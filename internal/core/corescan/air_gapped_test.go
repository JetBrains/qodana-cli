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

func TestAirGappedFromOption(t *testing.T) {
	context := CreateContext(platformcmd.CliOptions{AirGapped: true}, commoncontext.Context{}, startup.PreparedHost{}, QodanaYamlConfig{}, "")

	assert.True(t, context.AirGapped())
}

// TestAirGappedIgnoresQodanaYaml verifies that the CLI does not read airGapped from qodana.yaml:
// the linter reads it, as it reads disableSanityInspections.
func TestAirGappedIgnoresQodanaYaml(t *testing.T) {
	yaml := YamlConfig(qdyaml.QodanaYaml{AirGapped: true})
	context := CreateContext(platformcmd.CliOptions{}, commoncontext.Context{}, startup.PreparedHost{}, yaml, "")

	assert.False(t, context.AirGapped())
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
