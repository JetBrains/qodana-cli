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
	"github.com/JetBrains/qodana-cli/v2024/core/corescan"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func createDefaultYaml(sln string, prj string, cfg string, plt string) qdyaml.QodanaYaml {
	return qdyaml.QodanaYaml{
		DotNet: qdyaml.DotNet{
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
		cb           thirdpartyscan.ContextBuilder
		expectedArgs []string
		expectedErr  string
	}{
		{
			name: "No solution/project specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:   []string{},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("", "", "", ""),
			},
			expectedArgs: nil,
			expectedErr:  "solution/project relative file path is not specified. Use --solution or --project flags or create qodana.yaml file with respective fields",
		},
		{
			name: "project specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:     []string{},
				ResultsDir:   "",
				CdnetProject: "project",
				QodanaYaml:   createDefaultYaml("", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"project",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
			},
			expectedErr: "",
		},
		{
			name: "project specified in yaml",
			cb: thirdpartyscan.ContextBuilder{
				Property:   []string{},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("", "project", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"project",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
			},
			expectedErr: "",
		},
		{
			name: "solution specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:      []string{},
				ResultsDir:    "",
				CdnetSolution: "solution",
				QodanaYaml:    createDefaultYaml("", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
			},
			expectedErr: "",
		},
		{
			name: "solution specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:   []string{},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
			},
			expectedErr: "",
		},
		{
			name: "configuration specified in yaml",
			cb: thirdpartyscan.ContextBuilder{
				Property:   []string{},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("solution", "", "cfg", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--properties:Configuration=cfg",
			},
			expectedErr: "",
		},
		{
			name: "configuration specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:           []string{},
				ResultsDir:         "",
				CdnetConfiguration: "cfg",
				QodanaYaml:         createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--properties:Configuration=cfg",
			},
			expectedErr: "",
		},
		{
			name: "platform specified in cfg",
			cb: thirdpartyscan.ContextBuilder{
				Property:   []string{},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("solution", "", "", "x64"),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--properties:Platform=x64",
			},
			expectedErr: "",
		},
		{
			name: "platform specified",
			cb: thirdpartyscan.ContextBuilder{
				Property:      []string{},
				ResultsDir:    "",
				CdnetPlatform: "x64",
				QodanaYaml:    createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--properties:Platform=x64",
			},
			expectedErr: "",
		},
		{
			name: "many options",
			cb: thirdpartyscan.ContextBuilder{
				Property:           []string{"prop1=val1", "prop2=val2"},
				ResultsDir:         "",
				CdnetPlatform:      "x64",
				CdnetConfiguration: "Debug",
				QodanaYaml:         createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--properties:prop1=val1;prop2=val2;Configuration=Debug;Platform=x64",
			},
			expectedErr: "",
		},
		{
			name: "no-build",
			cb: thirdpartyscan.ContextBuilder{
				Property:     []string{},
				ResultsDir:   "",
				CdnetNoBuild: true,
				QodanaYaml:   createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
				"--no-build",
			},
			expectedErr: "",
		},
		{
			name: "TeamCity args ignored",
			cb: thirdpartyscan.ContextBuilder{
				Property: []string{
					"log.project.structure.changes=true",
					"idea.log.config.file=warn.xml",
					"qodana.default.file.suspend.threshold=100000",
					"qodana.default.module.suspend.threshold=100000",
					"qodana.default.project.suspend.threshold=100000",
					"idea.diagnostic.opentelemetry.file=/data/results/log/open-telemetry.json",
					"jetbrains.security.package-checker.synchronizationTimeout=1000",
				},
				ResultsDir: "",
				QodanaYaml: createDefaultYaml("solution", "", "", ""),
			},
			expectedArgs: []string{
				"dotnet",
				"clt",
				"inspectcode",
				"solution",
				"-o=\"qodana.sarif.json\"",
				"-f=\"Qodana\"",
				"--LogFolder=\"log\"",
			},
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				logDir := "logDir"

				tt.cb.LogDir = logDir
				tt.cb.MountInfo = getTooling()
				context := tt.cb.Build()
				args, err := CdnetLinter{}.computeCdnetArgs(context)

				if utils.Contains(tt.expectedArgs, "--LogFolder=\"log\"") {
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
			},
		)
	}
}

func getTooling() thirdpartyscan.MountInfo {
	return thirdpartyscan.MountInfo{
		CustomTools: map[string]string{"clt": "clt"},
	}
}

func TestGetArgsThirdPartyLinters(t *testing.T) {
	cases := []struct {
		name     string
		cb       corescan.ContextBuilder
		expected []string
	}{
		{
			name: "not sending statistics",
			cb: corescan.ContextBuilder{
				NoStatistics: true,
				Linter:       product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--no-statistics",
			},
		},
		{
			name: "(cdnet) solution",
			cb: corescan.ContextBuilder{
				CdnetSolution: "solution.sln",
				Linter:        product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--solution", "solution.sln",
			},
		},
		{
			name: "(cdnet) project",
			cb: corescan.ContextBuilder{
				CdnetProject: "project.csproj",
				Linter:       product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--project", "project.csproj",
			},
		},
		{
			name: "(cdnet) configuration",
			cb: corescan.ContextBuilder{
				CdnetConfiguration: "Debug",
				Linter:             product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--configuration", "Debug",
			},
		},
		{
			name: "(cdnet) platform",
			cb: corescan.ContextBuilder{
				CdnetPlatform: "x64",
				Linter:        product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--platform", "x64",
			},
		},
		{
			name: "(cdnet) no build",
			cb: corescan.ContextBuilder{
				CdnetNoBuild: true,
				Linter:       product.DockerImageMap[product.QDNETC],
			},
			expected: []string{
				"--no-build",
			},
		},
		{
			name: "(clang) compile commands",
			cb: corescan.ContextBuilder{
				ClangCompileCommands: "compile_commands.json",
				Linter:               product.DockerImageMap[product.QDCL],
			},
			expected: []string{
				"--compile-commands", "compile_commands.json",
			},
		},
		{
			name: "(clang) clang args",
			cb: corescan.ContextBuilder{
				ClangArgs: "-I/usr/include",
				Linter:    product.DockerImageMap[product.QDCL],
			},
			expected: []string{
				"--clang-args", "-I/usr/include",
			},
		},
		{
			name: "using flag in non 3rd party linter",
			cb: corescan.ContextBuilder{
				NoStatistics: true,
				Ide:          product.QDNET,
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(
			tt.name, func(t *testing.T) {
				contextBuilder := tt.cb
				if contextBuilder.Ide != "" {
					contextBuilder.Prod.Code = contextBuilder.Ide
				}

				context := contextBuilder.Build()
				actual := core.GetIdeArgs(context)
				if !reflect.DeepEqual(tt.expected, actual) {
					t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
				}
			},
		)
	}
}
