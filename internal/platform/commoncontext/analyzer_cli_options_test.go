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

package commoncontext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/sirupsen/logrus"
)

// Test this case on yaml is future effort due to communication with cloud
var NoneTest = TestCase{
	name:     "No parameters defined",
	expected: nil,
}

func TestAnalyzerCliOptions(t *testing.T) {
	for _, tt := range append(optionsTests, NoneTest) {
		t.Run(
			tt.name, func(t *testing.T) {
				var fatal bool
				if tt.failure {
					defer func() { logrus.StandardLogger().ExitFunc = nil }()
					logrus.StandardLogger().ExitFunc = func(int) { fatal = true }
				}

				analyzer := GuessAnalyzerFromEnvAndCLI(tt.ide, tt.linter, tt.image, tt.withinDocker)
				if tt.failure && !fatal {
					t.Errorf("Expected failure case, got %v", analyzer.Name())
					return
				}
				if analyzer == nil {
					if tt.expected != nil {
						t.Errorf("Expected linter to be %s, got nil", tt.expected.Name())
						return
					}
				} else if analyzer.GetLinter() != tt.expected.GetLinter() || analyzer.Name() != tt.expected.Name() {
					t.Errorf("Expected linetr to be %s, got %v", tt.expected.Name(), analyzer.Name())
				}
			},
		)
	}
}

func TestNativePathAnalyzerParams(t *testing.T) {
	distPath, err := os.MkdirTemp("", "TestNativePathAnalyzerParamsDist")
	if err != nil {
		t.FailNow()
	}
	makeFakeProductInfo(distPath, product.JvmLinterProperties.ProductInfoJsonCode)

	defer func(path string) {
		_ = os.RemoveAll(path)
	}(distPath)

	tests := []struct {
		name           string
		ide            string
		linter         string
		qodanaDistEnv  string
		expectedLinter product.Linter
		fatal          bool
		setup          func(t *testing.T)
	}{
		{
			"Pass through ENV",
			"",
			"",
			distPath,
			product.JvmLinter,
			false,
			func(t *testing.T) {},
		},
		{
			"Pass through --ide",
			distPath,
			"",
			"",
			product.JvmLinter,
			false,
			func(t *testing.T) {},
		},
		{
			"Pass through --ide and dist",
			distPath + "ignored",
			"",
			distPath,
			product.JvmLinter,
			false,
			func(t *testing.T) {},
		},
		{
			"Unknown dist",
			"",
			"",
			distPath + "wrong",
			product.JvmLinter,
			true,
			func(t *testing.T) {},
		},
		{
			"Pass through dist & flavour",
			distPath,
			"",
			distPath,
			product.JvmCommunityLinter,
			false,
			func(t *testing.T) {
				makeDistFlavourFile(distPath, product.JvmCommunityLinter.ProductCode)
			},
		},
		{
			"Pass through dist & flavour - unknown dist",
			distPath,
			"",
			distPath,
			product.UnknownLinter,
			true,
			func(t *testing.T) {
				makeDistFlavourFile(distPath, "wrong code")
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if tt.setup != nil {
					tt.setup(t)
				}
				t.Setenv(qdenv.QodanaDistEnv, tt.qodanaDistEnv)
				if tt.fatal {
					defer func() { logrus.StandardLogger().ExitFunc = nil }()
					var fatal bool
					logrus.StandardLogger().ExitFunc = func(int) { fatal = true }
					GuessAnalyzerFromEnvAndCLI(tt.ide, tt.linter, "", "")
					if !fatal {
						t.FailNow()
					}
					return
				}

				expected := product.PathNativeAnalyzer{
					Linter: tt.expectedLinter,
					Path:   distPath,
					IsEap:  false,
				}
				analyzer := GuessAnalyzerFromEnvAndCLI(tt.ide, tt.linter, "", "")

				pathNativeAnalyzer, ok := analyzer.(*product.PathNativeAnalyzer)
				if !ok {
					t.FailNow()
				}
				if *pathNativeAnalyzer != expected {
					t.Fatalf("Expected to be %v, got %v", expected, pathNativeAnalyzer)
				}
			},
		)
	}
}

func makeDistFlavourFile(distPath string, productCode string) {
	bytes := []byte(productCode)
	ideDir := distPath
	if //goland:noinspection ALL
	runtime.GOOS == "darwin" {
		ideDir = filepath.Join(distPath, "Resources")
	}
	_ = os.WriteFile(filepath.Join(ideDir, "dist.flavour.txt"), bytes, 0644)
}

func makeFakeProductInfo(ideDir string, productCode string) {
	if //goland:noinspection ALL
	runtime.GOOS == "darwin" {
		ideDir = filepath.Join(ideDir, "Resources")
	}
	_ = os.MkdirAll(ideDir, 0755)
	productInfo := product.InfoJson{
		ProductCode: productCode,
	}
	productInfoBytes, _ := json.Marshal(productInfo)
	_ = os.WriteFile(filepath.Join(ideDir, "product-info.json"), productInfoBytes, 0644)
}
