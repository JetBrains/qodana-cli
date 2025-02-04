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

package main

import (
	"github.com/JetBrains/qodana-cli/v2024/core"
	"github.com/JetBrains/qodana-cli/v2024/core/preparehost"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func createDefaultYaml(sln string, prj string, cfg string, plt string) *platform.QodanaYaml {
	return &platform.QodanaYaml{
		DotNet: platform.DotNet{
			Solution:      sln,
			Project:       prj,
			Configuration: cfg,
			Platform:      plt,
		},
	}
}

func TestComputeCdnetArgs(t *testing.T) {
	tests := []struct {
		name         string
		options      *platform.QodanaOptions
		yaml         *platform.QodanaYaml
		expectedArgs []string
		expectedErr  string
	}{
		{
			name: "No solution/project specified",
			options: &platform.QodanaOptions{
				Property:   []string{},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: nil,
			expectedErr:  "solution/project relative file path is not specified. Use --solution or --project flags or create qodana.yaml file with respective fields",
		},
		{
			name: "project specified",
			options: &platform.QodanaOptions{
				Property:     []string{},
				ResultsDir:   "",
				CdnetProject: "project",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "project", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "project specified in yaml",
			options: &platform.QodanaOptions{
				Property:   []string{},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("", "project", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "project", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "solution specified",
			options: &platform.QodanaOptions{
				Property:      []string{},
				ResultsDir:    "",
				CdnetSolution: "solution",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "solution specified",
			options: &platform.QodanaOptions{
				Property:   []string{},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "configuration specified in yaml",
			options: &platform.QodanaOptions{
				Property:   []string{},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "cfg", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Configuration=cfg"},
			expectedErr:  "",
		},
		{
			name: "configuration specified",
			options: &platform.QodanaOptions{
				Property:           []string{},
				ResultsDir:         "",
				CdnetConfiguration: "cfg",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Configuration=cfg"},
			expectedErr:  "",
		},
		{
			name: "platform specified in cfg",
			options: &platform.QodanaOptions{
				Property:   []string{},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", "x64"),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "platform specified",
			options: &platform.QodanaOptions{
				Property:      []string{},
				ResultsDir:    "",
				CdnetPlatform: "x64",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "many options",
			options: &platform.QodanaOptions{
				Property:           []string{"prop1=val1", "prop2=val2"},
				ResultsDir:         "",
				CdnetPlatform:      "x64",
				CdnetConfiguration: "Debug",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:prop1=val1;prop2=val2;Configuration=Debug;Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "no-build",
			options: &platform.QodanaOptions{
				Property:     []string{},
				ResultsDir:   "",
				CdnetNoBuild: true,
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--no-build"},
			expectedErr:  "",
		},
		{
			name: "TeamCity args ignored",
			options: &platform.QodanaOptions{
				Property:   []string{"log.project.structure.changes=true", "idea.log.config.file=warn.xml", "qodana.default.file.suspend.threshold=100000", "qodana.default.module.suspend.threshold=100000", "qodana.default.project.suspend.threshold=100000", "idea.diagnostic.opentelemetry.file=/data/results/log/open-telemetry.json", "jetbrains.security.package-checker.synchronizationTimeout=1000"},
				ResultsDir: "",
				LinterSpecific: &CltOptions{
					MountInfo: getTooling(),
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &LocalOptions{tt.options}
			args, err := options.GetCltOptions().computeCdnetArgs(tt.options, options, tt.yaml)
			logDir := options.LogDirPath()
			if platform.Contains(tt.expectedArgs, "--LogFolder=\"log\"") {
				for i, arg := range tt.expectedArgs {
					if arg == "--LogFolder=\"log\"" {
						tt.expectedArgs[i] = "--LogFolder=\"" + logDir + "\""
					}
				}

			}

			if tt.expectedErr != "" {
				assert.NotNil(t, err)
				assert.Equal(t, tt.expectedErr, err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedArgs, args)
			}
		})
	}
}

func getTooling() *platform.MountInfo {
	return &platform.MountInfo{
		CustomTools: map[string]string{"clt": "clt"},
	}
}

func TestGetArgsThirdPartyLinters(t *testing.T) {
	cases := []struct {
		name     string
		options  *platform.QodanaOptions
		expected []string
	}{
		{
			name: "not sending statistics",
			options: &platform.QodanaOptions{
				NoStatistics: true,
				Linter:       platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--no-statistics",
			},
		},
		{
			name: "(cdnet) solution",
			options: &platform.QodanaOptions{
				CdnetSolution: "solution.sln",
				Linter:        platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--solution", "solution.sln",
			},
		},
		{
			name: "(cdnet) project",
			options: &platform.QodanaOptions{
				CdnetProject: "project.csproj",
				Linter:       platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--project", "project.csproj",
			},
		},
		{
			name: "(cdnet) configuration",
			options: &platform.QodanaOptions{
				CdnetConfiguration: "Debug",
				Linter:             platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--configuration", "Debug",
			},
		},
		{
			name: "(cdnet) platform",
			options: &platform.QodanaOptions{
				CdnetPlatform: "x64",
				Linter:        platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--platform", "x64",
			},
		},
		{
			name: "(cdnet) no build",
			options: &platform.QodanaOptions{
				CdnetNoBuild: true,
				Linter:       platform.DockerImageMap[platform.QDNETC],
			},
			expected: []string{
				"--no-build",
			},
		},
		{
			name: "(clang) compile commands",
			options: &platform.QodanaOptions{
				ClangCompileCommands: "compile_commands.json",
				Linter:               platform.DockerImageMap[platform.QDCL],
			},
			expected: []string{
				"--compile-commands", "compile_commands.json",
			},
		},
		{
			name: "(clang) clang args",
			options: &platform.QodanaOptions{
				ClangArgs: "-I/usr/include",
				Linter:    platform.DockerImageMap[platform.QDCL],
			},
			expected: []string{
				"--clang-args", "-I/usr/include",
			},
		},
		{
			name: "using flag in non 3rd party linter",
			options: &platform.QodanaOptions{
				NoStatistics: true,
				Ide:          platform.QDNET,
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.options.Ide != "" {
				preparehost.Prod.Code = tt.options.Ide
			}

			actual := core.GetIdeArgs(&core.QodanaOptions{QodanaOptions: tt.options})
			if !reflect.DeepEqual(tt.expected, actual) {
				t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
			}
		})
	}
	t.Cleanup(func() {
		preparehost.Prod.Code = ""
	})
}
