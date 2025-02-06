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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"testing"
)

func TestQodanaOptions_guessProduct(t *testing.T) {
	tests := []struct {
		name     string
		ide      string
		linter   string
		expected string
	}{
		{"IDE defined", "QDNET", "", "QDNET"},
		{"IDE defined with EapSuffix", "QDNET-EAP", "", "QDNET"},
		{"IDE defined not in Products", "NEVERGONNAGIVEYOUUP", "", ""},
		{"Linter defined", "", "jetbrains/qodana-dotnet:2023.3-eap", "QDNET"},
		{"TC defined", "", "registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc", "QDPHP"},
		{"Both defined", "QDNET", "jetbrains/qodana-php:2023.3-eap", "QDNET"},
		{"Unknown linter defined", "", "jetbrains/qodana-unknown:2023.3-eap", ""},
		{"None defined", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {

				opts := &QodanaOptions{
					&platform.QodanaOptions{
						Ide:    tt.ide,
						Linter: tt.linter,
					},
				}
				if got := product.GuessProductCode(opts.Ide, opts.Linter); got != tt.expected {
					t.Errorf("QodanaOptions.guessProduct() = %v, want %v", got, tt.expected)
				}
			},
		)
	}
}
