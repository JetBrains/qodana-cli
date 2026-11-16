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
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

const airGappedOn = AirGappedProperty + "=true"

func TestAirGappedFromOption(t *testing.T) {
	context := ContextBuilder{ResultsDir: t.TempDir(), AirGapped: true}.Build()

	assert.True(t, context.AirGapped())
	assert.Contains(t, context.Property(), airGappedOn)
}

func TestAirGappedFromQodanaYaml(t *testing.T) {
	yaml := YamlConfig(qdyaml.QodanaYaml{AirGapped: true})
	context := ContextBuilder{ResultsDir: t.TempDir(), QodanaYamlConfig: yaml}.Build()

	assert.True(t, context.AirGapped())
	assert.Contains(t, context.Property(), airGappedOn)
}

// TestAirGappedFromCliProperty verifies the case of the CLI in a container: the CLI on the host passes the mode
// as --property, and the CLI in the container must know it too.
func TestAirGappedFromCliProperty(t *testing.T) {
	context := ContextBuilder{ResultsDir: t.TempDir(), Property: []string{"qodana.air.gapped=TRUE"}}.Build()

	assert.True(t, context.AirGapped())
}

func TestAirGappedFromQodanaYamlProperties(t *testing.T) {
	yaml := YamlConfig(qdyaml.QodanaYaml{Properties: map[string]string{"qodana.air.gapped": "true"}})
	context := ContextBuilder{ResultsDir: t.TempDir(), QodanaYamlConfig: yaml}.Build()

	assert.True(t, context.AirGapped())
}

func TestNotAirGappedWithoutSources(t *testing.T) {
	context := ContextBuilder{ResultsDir: t.TempDir(), Property: []string{"qodana.air.gapped=1"}}.Build()

	assert.False(t, context.AirGapped())
	for _, p := range context.Property() {
		assert.False(t, strings.HasPrefix(p, airGappedOn), "unexpected %s", p)
	}
}

// TestSecondStageOfScopedScriptKeepsAirGapped verifies that the second linter run of the scoped script
// gets the same air-gapped mode as the first one.
func TestSecondStageOfScopedScriptKeepsAirGapped(t *testing.T) {
	resultsDir := t.TempDir()
	context := ContextBuilder{ResultsDir: resultsDir, LogDir: filepath.Join(resultsDir, "log"), AirGapped: true}.
		Build().
		SecondStageOfScopedScript(filepath.Join(resultsDir, "scope.json"), filepath.Join(resultsDir, "start.json"))

	assert.True(t, context.AirGapped())
	assert.Contains(t, context.Property(), airGappedOn)
}
