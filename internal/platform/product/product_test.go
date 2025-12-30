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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/stretchr/testify/assert"
)

func TestDockerAnalyzer_Methods(t *testing.T) {
	analyzer := &DockerAnalyzer{
		Linter: JvmLinter,
		Image:  "jetbrains/qodana-jvm:2024.1",
	}

	t.Run("IsContainer", func(t *testing.T) {
		assert.True(t, analyzer.IsContainer())
	})

	t.Run("IsEAP", func(t *testing.T) {
		assert.False(t, analyzer.IsEAP())

		eapAnalyzer := &DockerAnalyzer{
			Linter: JvmLinter,
			Image:  "jetbrains/qodana-jvm:2024.1-eap",
		}
		assert.True(t, eapAnalyzer.IsEAP())

		eapUpperAnalyzer := &DockerAnalyzer{
			Linter: JvmLinter,
			Image:  "jetbrains/qodana-jvm:2024.1-EAP",
		}
		assert.True(t, eapUpperAnalyzer.IsEAP())
	})

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "jetbrains/qodana-jvm:2024.1", analyzer.Name())
	})

	t.Run("GetLinter", func(t *testing.T) {
		assert.Equal(t, JvmLinter, analyzer.GetLinter())
	})

	t.Run("DownloadDist", func(t *testing.T) {
		assert.False(t, analyzer.DownloadDist())
	})

	t.Run("InitYaml", func(t *testing.T) {
		yaml := qdyaml.QodanaYaml{}
		result := analyzer.InitYaml(yaml)
		assert.Equal(t, JvmLinter.Name, result.Linter)
	})
}

func TestNativeAnalyzer_Methods(t *testing.T) {
	analyzer := &NativeAnalyzer{
		Linter: JvmLinter,
		Eap:    false,
	}

	t.Run("IsContainer", func(t *testing.T) {
		assert.False(t, analyzer.IsContainer())
	})

	t.Run("IsEAP", func(t *testing.T) {
		assert.False(t, analyzer.IsEAP())

		eapAnalyzer := &NativeAnalyzer{
			Linter: JvmLinter,
			Eap:    true,
		}
		assert.True(t, eapAnalyzer.IsEAP())
	})

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, QDJVM, analyzer.Name())

		eapAnalyzer := &NativeAnalyzer{
			Linter: JvmLinter,
			Eap:    true,
		}
		assert.Equal(t, QDJVM+EapSuffix, eapAnalyzer.Name())
	})

	t.Run("GetLinter", func(t *testing.T) {
		assert.Equal(t, JvmLinter, analyzer.GetLinter())
	})

	t.Run("DownloadDist", func(t *testing.T) {
		assert.True(t, analyzer.DownloadDist())
	})

	t.Run("InitYaml", func(t *testing.T) {
		yaml := qdyaml.QodanaYaml{}
		result := analyzer.InitYaml(yaml)
		assert.Equal(t, JvmLinter.Name, result.Linter)
	})
}

func TestPathNativeAnalyzer_Methods(t *testing.T) {
	analyzer := &PathNativeAnalyzer{
		Linter: JvmLinter,
		Path:   "/opt/idea",
		IsEap:  false,
	}

	t.Run("IsContainer", func(t *testing.T) {
		assert.False(t, analyzer.IsContainer())
	})

	t.Run("IsEAP", func(t *testing.T) {
		assert.False(t, analyzer.IsEAP())

		eapAnalyzer := &PathNativeAnalyzer{
			Linter: JvmLinter,
			Path:   "/opt/idea-eap",
			IsEap:  true,
		}
		assert.True(t, eapAnalyzer.IsEAP())
	})

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "/opt/idea", analyzer.Name())
	})

	t.Run("GetLinter", func(t *testing.T) {
		assert.Equal(t, JvmLinter, analyzer.GetLinter())
	})

	t.Run("DownloadDist", func(t *testing.T) {
		assert.False(t, analyzer.DownloadDist())
	})
}

func TestLinter_NativeAnalyzer(t *testing.T) {
	analyzer := JvmLinter.NativeAnalyzer()

	assert.False(t, analyzer.IsContainer())
	assert.Equal(t, JvmLinter, analyzer.GetLinter())
}

func TestLinter_DockerAnalyzer(t *testing.T) {
	analyzer := JvmLinter.DockerAnalyzer()

	assert.True(t, analyzer.IsContainer())
	assert.Equal(t, JvmLinter, analyzer.GetLinter())
}

func TestLinter_Image(t *testing.T) {
	image := JvmLinter.Image()
	assert.Contains(t, image, JvmLinter.DockerImage)
	assert.Contains(t, image, ReleaseVersion)
}

func TestFindLinterByImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected Linter
	}{
		{
			name:     "find jvm linter",
			image:    "jetbrains/qodana-jvm:2024.1",
			expected: JvmLinter,
		},
		{
			name:     "find jvm community linter",
			image:    "jetbrains/qodana-jvm-community:latest",
			expected: JvmCommunityLinter,
		},
		{
			name:     "find php linter",
			image:    "jetbrains/qodana-php:2024.1-eap",
			expected: PhpLinter,
		},
		{
			name:     "find go linter",
			image:    "jetbrains/qodana-go",
			expected: GoLinter,
		},
		{
			name:     "internal registry",
			image:    "registry.jetbrains.team/p/sa/containers/qodana-jvm:latest",
			expected: JvmLinter,
		},
		{
			name:     "with https prefix",
			image:    "https://jetbrains/qodana-jvm:latest",
			expected: JvmLinter,
		},
		{
			name:     "with http prefix",
			image:    "http://jetbrains/qodana-jvm:latest",
			expected: JvmLinter,
		},
		{
			name:     "unknown linter",
			image:    "hadolint/hadolint:latest",
			expected: UnknownLinter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindLinterByImage(tt.image)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindLinterByProductCode(t *testing.T) {
	tests := []struct {
		name        string
		productCode string
		expected    Linter
	}{
		{
			name:        "find jvm linter",
			productCode: QDJVM,
			expected:    JvmLinter,
		},
		{
			name:        "find jvm community linter",
			productCode: QDJVMC,
			expected:    JvmCommunityLinter,
		},
		{
			name:        "find php linter",
			productCode: QDPHP,
			expected:    PhpLinter,
		},
		{
			name:        "find with EAP suffix",
			productCode: QDJVM + EapSuffix,
			expected:    JvmLinter,
		},
		{
			name:        "unknown product code",
			productCode: "UNKNOWN",
			expected:    UnknownLinter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindLinterByProductCode(tt.productCode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindLinterByName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Linter
	}{
		{
			name:     "find jvm linter",
			input:    "qodana-jvm",
			expected: JvmLinter,
		},
		{
			name:     "find jvm community linter",
			input:    "qodana-jvm-community",
			expected: JvmCommunityLinter,
		},
		{
			name:     "find php linter",
			input:    "qodana-php",
			expected: PhpLinter,
		},
		{
			name:     "find go linter",
			input:    "qodana-go",
			expected: GoLinter,
		},
		{
			name:     "with EAP suffix",
			input:    "qodana-jvm" + EapSuffix,
			expected: JvmLinter,
		},
		{
			name:     "unknown name",
			input:    "unknown-linter",
			expected: UnknownLinter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindLinterByName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllLinters_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, AllLinters)
	assert.NotEmpty(t, AllImages)
	assert.NotEmpty(t, AllNames)
	assert.NotEmpty(t, AllNativeLinters)
	assert.NotEmpty(t, AllSupportedFreeLinters)
	assert.NotEmpty(t, AllNativeProductCodes)
}

func TestLangsToLinters_Coverage(t *testing.T) {
	langs := []string{"Java", "Kotlin", "PHP", "Python", "JavaScript", "TypeScript", "Go", "C#", "F#", "Visual Basic .NET", "C", "C++", "Ruby", "Rust"}

	for _, lang := range langs {
		t.Run(lang, func(t *testing.T) {
			linters, ok := LangsToLinters[lang]
			assert.True(t, ok, "Language %s should be in LangsToLinters", lang)
			assert.NotEmpty(t, linters, "Language %s should have at least one linter", lang)
		})
	}
}

func TestAllLintersFiltered(t *testing.T) {
	freeLinters := allLintersFiltered(AllLinters, func(linter *Linter) bool { return !linter.IsPaid })
	for _, linter := range freeLinters {
		assert.False(t, linter.IsPaid)
	}
	nativeLinters := allLintersFiltered(AllLinters, func(linter *Linter) bool { return linter.SupportNative })
	for _, linter := range nativeLinters {
		assert.True(t, linter.SupportNative)
	}
}

func TestProduct_IdeBin(t *testing.T) {
	p := Product{Home: "/opt/idea"}
	assert.Contains(t, p.IdeBin(), "bin")
}

func TestProduct_VmOptionsEnv(t *testing.T) {
	tests := []struct {
		scriptName string
		expected   string
	}{
		{Idea, "IDEA_VM_OPTIONS"},
		{PhpStorm, "PHPSTORM_VM_OPTIONS"},
		{WebStorm, "WEBIDE_VM_OPTIONS"},
		{Rider, "RIDER_VM_OPTIONS"},
		{PyCharm, "PYCHARM_VM_OPTIONS"},
		{RubyMine, "RUBYMINE_VM_OPTIONS"},
		{GoLand, "GOLAND_VM_OPTIONS"},
		{RustRover, "RUSTROVER_VM_OPTIONS"},
		{Clion, "CLION_VM_OPTIONS"},
	}

	for _, tt := range tests {
		t.Run(tt.scriptName, func(t *testing.T) {
			p := Product{BaseScriptName: tt.scriptName}
			assert.Equal(t, tt.expected, p.VmOptionsEnv())
		})
	}
}

func TestProduct_ParentPrefix(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{QDPHP, "PhpStorm"},
		{QDJS, "WebStorm"},
		{QDNET, "Rider"},
		{QDPY, "Python"},
		{QDPYC, "PyCharmCore"},
		{QDGO, "GoLand"},
		{QDRUBY, "Ruby"},
		{QDRST, "RustRover"},
		{QDCPP, "CLion"},
		{"UNKNOWN", "Idea"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			p := Product{Code: tt.code}
			assert.Equal(t, tt.expected, p.ParentPrefix())
		})
	}
}

func TestGetProductNameFromCode(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{QDJVMC, "Qodana Community for JVM"},
		{QDPYC, "Qodana Community for Python"},
		{QDJVM, "Qodana for JVM"},
		{QDPHP, "Qodana for PHP"},
		{QDJS, "Qodana for JS"},
		{QDNET, "Qodana for .NET"},
		{QDPY, "Qodana for Python"},
		{QDGO, "Qodana for Go"},
		{QDRST, "Qodana for Rust"},
		{QDRUBY, "Qodana for Ruby"},
		{"UNKNOWN", "Qodana"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetProductNameFromCode(tt.code))
			p := Product{Code: tt.code}
			assert.Equal(t, tt.expected, p.GetProductNameFromCode())
		})
	}
}

func TestProduct_GetVersionBranch(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2024.1", "241"},
		{"2023.3", "233"},
		{"2022.2", "222"},
		{"invalid", "master"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			p := Product{Version: tt.version}
			assert.Equal(t, tt.expected, p.GetVersionBranch())
		})
	}
}

func TestProduct_VersionChecks(t *testing.T) {
	p233 := Product{Version: "2023.3"}
	assert.True(t, p233.Is233orNewer())
	assert.False(t, p233.Is242orNewer())

	p242 := Product{Version: "2024.2"}
	assert.True(t, p242.Is233orNewer())
	assert.True(t, p242.Is242orNewer())
	assert.False(t, p242.Is251orNewer())
}

func TestToQodanaCode(t *testing.T) {
	tests := []struct {
		ideCode  string
		expected string
	}{
		{"IC", QDJVMC},
		{"PC", QDPYC},
		{"IU", QDJVM},
		{"PS", QDPHP},
		{"WS", QDJS},
		{"RD", QDNET},
		{"PY", QDPY},
		{"GO", QDGO},
		{"RM", QDRUBY},
		{"RR", QDRST},
		{"CL", QDCPP},
		{"XX", "QD"},
	}

	for _, tt := range tests {
		t.Run(tt.ideCode, func(t *testing.T) {
			assert.Equal(t, tt.expected, toQodanaCode(tt.ideCode))
		})
	}
}

func TestGetScriptSuffix(t *testing.T) {
	suffix := getScriptSuffix()
	assert.IsType(t, "", suffix)
}

func TestFindIde(t *testing.T) {
	dir := t.TempDir()
	result := findIde(dir)
	assert.Empty(t, result)
}

func TestProduct_javaHome(t *testing.T) {
	p := Product{Home: "/opt/idea"}
	assert.Contains(t, p.javaHome(), "jbr")
}

func TestProduct_JbrJava(t *testing.T) {
	p := Product{Home: "/opt/idea"}
	result := p.JbrJava()
	assert.Contains(t, result, "java")
}

func TestProduct_isRuby(t *testing.T) {
	rubyProduct := Product{Code: QDRUBY}
	assert.True(t, rubyProduct.isRuby())

	jvmProduct := Product{Code: QDJVM}
	assert.False(t, jvmProduct.isRuby())
}

func TestIsEap(t *testing.T) {
	tests := []struct {
		name     string
		info     *InfoJson
		expected bool
	}{
		{
			name:     "no launch",
			info:     &InfoJson{},
			expected: false,
		},
		{
			name: "qodana with eap flag",
			info: &InfoJson{
				Launch: []Launch{
					{
						CustomCommands: []struct {
							Commands               []string
							AdditionalJvmArguments []string
						}{
							{
								Commands:               []string{"qodana"},
								AdditionalJvmArguments: []string{"-Dqodana.eap=true"},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "qodana without eap flag",
			info: &InfoJson{
				Launch: []Launch{
					{
						CustomCommands: []struct {
							Commands               []string
							AdditionalJvmArguments []string
						}{
							{
								Commands:               []string{"qodana"},
								AdditionalJvmArguments: []string{},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEap(tt.info))
		})
	}
}

func TestProduct_CustomPluginsPath(t *testing.T) {
	p := Product{
		Code:    QDJVM,
		Version: "2024.1",
		Home:    "/opt/idea",
	}
	path := p.CustomPluginsPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "custom-plugins")
}

func TestProduct_DisabledPluginsFilePath(t *testing.T) {
	p := Product{
		Code:    QDJVM,
		Version: "2024.1",
		Home:    "/opt/idea",
	}
	path := p.DisabledPluginsFilePath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "disabled_plugins.txt")
}

func TestProduct_isNotOlderThan(t *testing.T) {
	p233 := Product{Version: "2023.3"}
	assert.True(t, p233.isNotOlderThan(233))
	assert.False(t, p233.isNotOlderThan(241))

	pInvalid := Product{Version: "invalid"}
	assert.False(t, pInvalid.isNotOlderThan(233))
}
