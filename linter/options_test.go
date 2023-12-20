package linter

import (
	"github.com/JetBrains/qodana-cli/v2023/platform"
	"github.com/stretchr/testify/assert"
	"qodana-platform/core"
	"reflect"
	"testing"
)

func createDefaultYaml(sln string, prj string, cfg string, plt string) core.QodanaYaml {
	return core.QodanaYaml{
		DotNet: core.DotNet{
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
		options      *core.Options
		yaml         core.QodanaYaml
		expectedArgs []string
		expectedErr  string
	}{
		{
			name: "No solution/project specified",
			options: &core.Options{
				Property:       []string{},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: nil,
			expectedErr:  "solution/project relative file path is not specified. Use --solution or --project flags or create qodana.yaml file with respective fields",
		},
		{
			name: "project specified",
			options: &core.Options{
				Property:   []string{},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					Project: "project",
				},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "project", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "project specified",
			options: &core.Options{
				Property:       []string{},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
			},
			yaml:         createDefaultYaml("", "project", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "project", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "solution specified",
			options: &core.Options{
				Property:   []string{},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					Solution: "solution",
				},
			},
			yaml:         createDefaultYaml("", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "solution specified",
			options: &core.Options{
				Property:       []string{},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\""},
			expectedErr:  "",
		},
		{
			name: "configuration specified",
			options: &core.Options{
				Property:       []string{},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
			},
			yaml:         createDefaultYaml("solution", "", "cfg", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Configuration=cfg"},
			expectedErr:  "",
		},
		{
			name: "configuration specified",
			options: &core.Options{
				Property:   []string{},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					Configuration: "cfg",
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Configuration=cfg"},
			expectedErr:  "",
		},
		{
			name: "platform specified",
			options: &core.Options{
				Property:       []string{},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
			},
			yaml:         createDefaultYaml("solution", "", "", "x64"),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "platform specified",
			options: &core.Options{
				Property:   []string{},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					Platform: "x64",
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "many options",
			options: &core.Options{
				Property:   []string{"prop1=val1", "prop2=val2"},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					Platform:      "x64",
					Configuration: "Debug",
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--properties:prop1=val1;prop2=val2;Configuration=Debug;Platform=x64"},
			expectedErr:  "",
		},
		{
			name: "no-build",
			options: &core.Options{
				Property:   []string{},
				ResultsDir: "",
				Tooling:    getTooling(),
				LinterSpecific: &CltOptions{
					NoBuild: true,
				},
			},
			yaml:         createDefaultYaml("solution", "", "", ""),
			expectedArgs: []string{"dotnet", "clt", "inspectcode", "solution", "-o=\"qodana.sarif.json\"", "-f=\"Qodana\"", "--LogFolder=\"log\"", "--no-build"},
			expectedErr:  "",
		},
		{
			name: "TeamCity args ignored",
			options: &core.Options{
				Property:       []string{"log.project.structure.changes=true", "idea.log.config.file=warn.xml", "qodana.default.file.suspend.threshold=100000", "qodana.default.module.suspend.threshold=100000", "qodana.default.project.suspend.threshold=100000", "idea.diagnostic.opentelemetry.file=/data/results/log/open-telemetry.json", "jetbrains.security.package-checker.synchronizationTimeout=1000"},
				ResultsDir:     "",
				Tooling:        getTooling(),
				LinterSpecific: &CltOptions{},
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

func getTooling() *core.MountInfo {
	return &core.MountInfo{
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
				Linter:       DockerImageMap[QDNETC],
			},
			expected: []string{
				"--no-statistics",
			},
		},
		{
			name: "(cdnet) solution",
			options: &platform.QodanaOptions{
				Solution: "solution.sln",
				Linter:   DockerImageMap[QDNETC],
			},
			expected: []string{
				"--solution", "solution.sln",
			},
		},
		{
			name: "(cdnet) project",
			options: &platform.QodanaOptions{
				Project: "project.csproj",
				Linter:  DockerImageMap[QDNETC],
			},
			expected: []string{
				"--project", "project.csproj",
			},
		},
		{
			name: "(cdnet) configuration",
			options: &platform.QodanaOptions{
				Configuration: "Debug",
				Linter:        DockerImageMap[QDNETC],
			},
			expected: []string{
				"--configuration", "Debug",
			},
		},
		{
			name: "(cdnet) platform",
			options: &platform.QodanaOptions{
				Platform: "x64",
				Linter:   DockerImageMap[QDNETC],
			},
			expected: []string{
				"--platform", "x64",
			},
		},
		{
			name: "(cdnet) no build",
			options: &platform.QodanaOptions{
				NoBuild: true,
				Linter:  DockerImageMap[QDNETC],
			},
			expected: []string{
				"--no-build",
			},
		},
		{
			name: "(clang) compile commands",
			options: &platform.QodanaOptions{
				CompileCommands: "compile_commands.json",
				Linter:          DockerImageMap[QDCL],
			},
			expected: []string{
				"--compile-commands", "compile_commands.json",
			},
		},
		{
			name: "(clang) clang args",
			options: &platform.QodanaOptions{
				ClangArgs: "-I/usr/include",
				Linter:    DockerImageMap[QDCL],
			},
			expected: []string{
				"--clang-args", "-I/usr/include",
			},
		},
		{
			name: "using flag in non 3rd party linter",
			options: &platform.QodanaOptions{
				NoStatistics: true,
				Ide:          QDNET,
			},
			expected: []string{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.options.Ide != "" {
				Prod.Code = tt.options.Ide
			}

			actual := getIdeArgs(tt.options)
			if !reflect.DeepEqual(tt.expected, actual) {
				t.Fatalf("expected \"%s\" got \"%s\"", tt.expected, actual)
			}
		})
	}
	t.Cleanup(func() {
		Prod.Code = ""
	})
}
