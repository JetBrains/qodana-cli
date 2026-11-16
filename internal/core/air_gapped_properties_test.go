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
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/stretchr/testify/assert"
)

func scanPropertiesWithStatisticsOn(t *testing.T, airGapped bool) []string {
	context := corescan.ContextBuilder{
		ConfigDir: filepath.Join(t.TempDir(), "config"),
		LogDir:    filepath.Join(t.TempDir(), "log"),
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Prod:      product.Product{Version: "2026.2"},
		Property:  []string{"idea.headless.enable.statistics=true"},
		AirGapped: airGapped,
	}.Build()
	return GetScanProperties(context)
}

// TestAirGappedTurnsOffStatistics verifies that air-gapped mode turns off FUS also when the user turns it on.
// With FUS on, the IDE starts statistics-uploader on exit, and the CLI waits for it in a network that drops traffic.
func TestAirGappedTurnsOffStatistics(t *testing.T) {
	properties := scanPropertiesWithStatisticsOn(t, true)

	assert.Contains(t, properties, "-Didea.headless.enable.statistics=false")
	assert.NotContains(t, properties, "-Didea.headless.enable.statistics=true")
}

func TestStatisticsKeepsTheUserValueWithoutAirGapped(t *testing.T) {
	properties := scanPropertiesWithStatisticsOn(t, false)

	assert.Contains(t, properties, "-Didea.headless.enable.statistics=true")
}
