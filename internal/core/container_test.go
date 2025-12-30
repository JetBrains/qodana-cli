package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/registry"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/stretchr/testify/assert"
)

func TestImageChecks(t *testing.T) {
	testCases := []struct {
		linter          string
		isUnofficial    bool
		hasExactVersion bool
		isCompatible    bool
	}{
		{
			"hadolint",
			true,
			false,
			false,
		},
		{
			"jetbrains/qodana",
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:latest",
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:2022.1",
			false,
			true,
			false,
		},
		{
			fmt.Sprintf("jetbrains/qodana:%s", product.ReleaseVersion),
			false,
			true,
			true,
		},
	}
	for _, tc := range testCases {
		t.Run(
			tc.linter, func(t *testing.T) {
				if isUnofficialLinter(tc.linter) != tc.isUnofficial {
					t.Errorf("isUnofficial: got %v, want %v", isUnofficialLinter(tc.linter), tc.isUnofficial)
				}
				if hasExactVersionTag(tc.linter) != tc.hasExactVersion {
					t.Errorf("hasExactVersion: got %v, want %v", hasExactVersionTag(tc.linter), tc.hasExactVersion)
				}
				if isCompatibleLinter(tc.linter) != tc.isCompatible {
					t.Errorf("isCompatible: got %v, want %v", isCompatibleLinter(tc.linter), tc.isCompatible)
				}
			},
		)
	}
}

func TestSelectUser(t *testing.T) {
	// auto implies selecting a user automatically for non-priveleged images
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "auto"), "")

	// Explicitly specified UIDs should not be overridden
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "0"), "0")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "0"), "0")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "1337"), "1337")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "1337"), "1337")

	// Internal registry is supported
	assert.Equal(t, selectUser("registry.jetbrains.team/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(t, selectUser("registry.jetbrains.team/qodana-cpp:2025.2-eap-clang18-privileged", "auto"), "")

	// User-specified images are unaffected
	assert.Equal(t, selectUser("myregistry.local/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(
		t,
		selectUser("myregistry.local/qodana-cpp:2025.2-eap-clang18-privileged", "auto"),
		utils.GetDefaultUser(),
	)
}

func TestEncodeAuthToBase64(t *testing.T) {
	tests := []struct {
		name    string
		auth    registry.AuthConfig
		wantErr bool
	}{
		{
			name: "basic auth",
			auth: registry.AuthConfig{
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name:    "empty auth",
			auth:    registry.AuthConfig{},
			wantErr: false,
		},
		{
			name: "auth with server",
			auth: registry.AuthConfig{
				Username:      "user",
				Password:      "pass",
				ServerAddress: "https://registry.example.com",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				result, err := encodeAuthToBase64(tt.auth)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				assert.NoError(t, err)
				assert.NotEmpty(t, result)

				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)

				var decodedAuth registry.AuthConfig
				err = json.Unmarshal(decoded, &decodedAuth)
				assert.NoError(t, err)
				assert.Equal(t, tt.auth.Username, decodedAuth.Username)
				assert.Equal(t, tt.auth.Password, decodedAuth.Password)
			},
		)
	}
}

func TestIsDockerUnauthorizedError(t *testing.T) {
	tests := []struct {
		errMsg   string
		expected bool
	}{
		{"unauthorized: authentication required", true},
		{"Unauthorized access", true},
		{"access denied", true},
		{"DENIED: permission denied", true},
		{"forbidden: access is forbidden", true},
		{"Forbidden", true},
		{"image not found", false},
		{"connection refused", false},
		{"timeout", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(
			tt.errMsg, func(t *testing.T) {
				result := isDockerUnauthorizedError(tt.errMsg)
				assert.Equal(t, tt.expected, result)
			},
		)
	}
}

func TestCheckImage(t *testing.T) {
	t.Run(
		"unofficial linter", func(t *testing.T) {
			CheckImage("hadolint:latest")
		},
	)

	t.Run(
		"no exact version", func(t *testing.T) {
			CheckImage("jetbrains/qodana-jvm:latest")
		},
	)

	t.Run(
		"incompatible version", func(t *testing.T) {
			CheckImage("jetbrains/qodana-jvm:2020.1")
		},
	)

	t.Run(
		"compatible version", func(t *testing.T) {
			CheckImage(fmt.Sprintf("jetbrains/qodana-jvm:%s", product.ReleaseVersion))
		},
	)
}

func TestRemovePortSocket(t *testing.T) {
	dir := t.TempDir()
	ideaDir := filepath.Join(dir, "idea")
	subdir := filepath.Join(ideaDir, "subdir")
	err := os.MkdirAll(subdir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	portFile := filepath.Join(subdir, ".port")
	err = os.WriteFile(portFile, []byte("12345"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = removePortSocket(dir)
	assert.NoError(t, err)

	_, err = os.Stat(portFile)
	assert.True(t, os.IsNotExist(err))
}

func TestFixDarwinCaches(t *testing.T) {
	dir := t.TempDir()
	fixDarwinCaches(dir)
}

func TestExtractDockerVolumes(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows expects volumes like C:\host\path:/container/path (3 parts when split by :)
		tests := []struct {
			name           string
			volume         string
			expectedSource string
			expectedTarget string
		}{
			{
				name:           "windows volume",
				volume:         "C:\\host\\path:/container/path",
				expectedSource: "C:\\host\\path",
				expectedTarget: "/container/path",
			},
			{
				name:           "empty volume",
				volume:         "",
				expectedSource: "",
				expectedTarget: "",
			},
			{
				name:           "missing target",
				volume:         "C:\\host\\path",
				expectedSource: "",
				expectedTarget: "",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				source, target := extractDockerVolumes(tt.volume)
				assert.Equal(t, tt.expectedSource, source)
				assert.Equal(t, tt.expectedTarget, target)
			})
		}
	} else {
		// Unix-style volumes
		tests := []struct {
			name           string
			volume         string
			expectedSource string
			expectedTarget string
		}{
			{
				name:           "simple volume",
				volume:         "/host/path:/container/path",
				expectedSource: "/host/path",
				expectedTarget: "/container/path",
			},
			{
				name:           "empty volume",
				volume:         "",
				expectedSource: "",
				expectedTarget: "",
			},
			{
				name:           "missing target",
				volume:         "/host/path",
				expectedSource: "",
				expectedTarget: "",
			},
			{
				name:           "with spaces in path",
				volume:         "/host/path with spaces:/container/path",
				expectedSource: "/host/path with spaces",
				expectedTarget: "/container/path",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				source, target := extractDockerVolumes(tt.volume)
				assert.Equal(t, tt.expectedSource, source)
				assert.Equal(t, tt.expectedTarget, target)
			})
		}
	}
}

func TestGenerateDebugDockerRunCommand(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *backend.ContainerCreateConfig
		contains []string
	}{
		{
			name: "basic config",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Cmd:   []string{"--analyze"},
				},
			},
			contains: []string{"docker run", "jetbrains/qodana-jvm:latest", "--analyze"},
		},
		{
			name: "with user",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					User:  "1000:1000",
					Cmd:   []string{},
				},
			},
			contains: []string{"-u 1000:1000"},
		},
		{
			name: "with environment variables",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Env:   []string{"MY_VAR=value", "ANOTHER=test"},
					Cmd:   []string{},
				},
			},
			contains: []string{"-e MY_VAR=value", "-e ANOTHER=test"},
		},
		{
			name: "with auto remove",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Cmd:   []string{},
				},
				HostConfig: &container.HostConfig{
					AutoRemove: true,
				},
			},
			contains: []string{"--rm"},
		},
		{
			name: "with mounts",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Cmd:   []string{},
				},
				HostConfig: &container.HostConfig{
					Mounts: []mount.Mount{
						{Source: "/host/path", Target: "/container/path"},
					},
				},
			},
			contains: []string{"-v /host/path:/container/path"},
		},
		{
			name: "with capabilities",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Cmd:   []string{},
				},
				HostConfig: &container.HostConfig{
					CapAdd: []string{"SYS_PTRACE"},
				},
			},
			contains: []string{"--cap-add SYS_PTRACE"},
		},
		{
			name: "with security opts",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image: "jetbrains/qodana-jvm:latest",
					Cmd:   []string{},
				},
				HostConfig: &container.HostConfig{
					SecurityOpt: []string{"seccomp=unconfined"},
				},
			},
			contains: []string{"--security-opt seccomp=unconfined"},
		},
		{
			name: "with attach stdout/stderr and tty",
			cfg: &backend.ContainerCreateConfig{
				Name: "test-container",
				Config: &container.Config{
					Image:        "jetbrains/qodana-jvm:latest",
					AttachStdout: true,
					AttachStderr: true,
					Tty:          true,
					Cmd:          []string{},
				},
			},
			contains: []string{"-a stdout", "-a stderr", "-it"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDebugDockerRunCommand(tt.cfg)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestGenerateDebugDockerRunCommand_FiltersTokens(t *testing.T) {
	cfg := &backend.ContainerCreateConfig{
		Name: "test-container",
		Config: &container.Config{
			Image: "jetbrains/qodana-jvm:latest",
			Env: []string{
				"SAFE_VAR=value",
				"QODANA_TOKEN=secret_token",
			},
			Cmd: []string{},
		},
	}

	result := generateDebugDockerRunCommand(cfg)
	assert.Contains(t, result, "-e SAFE_VAR=value")
	// QODANA_TOKEN should be filtered out
	assert.NotContains(t, result, "secret_token")
}
