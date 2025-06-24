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

package core

import (
	"github.com/JetBrains/qodana-cli/v2025/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"testing"
)

func TestQodanaOptions_GuessAnalyzerFromParams(t *testing.T) {
	tests := []struct {
		name     string
		ide      string
		linter   string
		expected product.Analyzer
	}{
		{
			"IDE defined",
			"QDNET",
			"",
			product.DotNetLinter.NativeAnalyzer(),
		},
		{
			"IDE defined with EapSuffix",
			"QDNET-EAP",
			"",
			&product.NativeAnalyzer{Linter: product.DotNetLinter, Eap: true},
		},
		{
			"Linter defined",
			"",
			"jetbrains/qodana-dotnet:2023.3-eap",
			&product.DockerAnalyzer{Linter: product.DotNetLinter, Image: "jetbrains/qodana-dotnet:2023.3-eap"},
		},
		{
			"TC defined",
			"",
			"registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc",
			&product.DockerAnalyzer{
				Linter: product.PhpLinter,
				Image:  "registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc",
			},
		},

		{
			"Both defined",
			"QDNET",
			"jetbrains/qodana-php:2023.3-eap",
			product.DotNetLinter.NativeAnalyzer(),
		},
		{
			"Unknown linter defined",
			"",
			"jetbrains/qodana-unknown:2023.3-eap",
			&product.DockerAnalyzer{
				Linter: product.UnknownLinter,
				Image:  "jetbrains/qodana-unknown:2023.3-eap",
			},
		},
		{
			"None defined", "", "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				analyzer := commoncontext.GuessAnalyzerFromParams(
					tt.ide,
					tt.linter,
				)
				if analyzer == nil {
					if tt.expected != nil {
						t.Errorf("Expected linetr to be %s, got nil", tt.expected.Name())
						return
					}
				} else if analyzer.GetLinter() != tt.expected.GetLinter() || analyzer.Name() != tt.expected.Name() {
					t.Errorf("Expected linetr to be %s, got %v", tt.expected.Name(), analyzer.Name())
				}
			},
		)
	}
}
