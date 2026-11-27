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

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreconfiguredLinter(t *testing.T) {
	for name, tc := range map[string]struct {
		ide, linter, code, mode string
	}{
		"ide is native":          {"QDJVM", "", "QDJVM", "native"},
		"name, scan chooses":     {"", "qodana-jvm", "QDJVM", ""},
		"image is docker":        {"", "jetbrains/qodana-jvm:2025.3", "QDJVM", "docker"},
		"linter wins over ide":   {"QDPY", "qodana-jvm", "QDJVM", ""},
		"unknown name, no mode":  {"", "qodana-unknown", "", ""},
		"unknown image, no mode": {"", "example/linter:1", "", ""},
	} {
		code, mode := preconfiguredLinter(tc.ide, tc.linter)
		assert.Equal(t, tc.code, code, name)
		assert.Equal(t, tc.mode, mode, name)
	}
}
