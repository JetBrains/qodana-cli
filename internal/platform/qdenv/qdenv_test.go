package qdenv

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockEnvProvider struct {
	envVars []string
}

func (m mockEnvProvider) Env() []string {
	return m.envVars
}

func TestEmptyEnvProvider(t *testing.T) {
	provider := EmptyEnvProvider()
	assert.Empty(t, provider.Env())
}

func TestGetEnv(t *testing.T) {
	provider := mockEnvProvider{envVars: []string{"FOO=bar", "BAZ=qux"}}

	assert.Equal(t, "bar", GetEnv(provider, "FOO"))
	assert.Equal(t, "qux", GetEnv(provider, "BAZ"))
	assert.Equal(t, "", GetEnv(provider, "MISSING"))
}

func TestGetEnvWithOsEnv(t *testing.T) {
	t.Run("from provider", func(t *testing.T) {
		provider := mockEnvProvider{envVars: []string{"TEST_VAR=from_provider"}}
		result := GetEnvWithOsEnv(provider, "TEST_VAR")
		assert.Equal(t, "from_provider", result)
	})

	t.Run("from os when not in provider", func(t *testing.T) {
		t.Setenv("OS_TEST_VAR", "from_os")
		provider := mockEnvProvider{envVars: []string{}}
		result := GetEnvWithOsEnv(provider, "OS_TEST_VAR")
		assert.Equal(t, "from_os", result)
	})

	t.Run("provider takes precedence", func(t *testing.T) {
		t.Setenv("PRIO_VAR", "from_os")
		provider := mockEnvProvider{envVars: []string{"PRIO_VAR=from_provider"}}
		result := GetEnvWithOsEnv(provider, "PRIO_VAR")
		assert.Equal(t, "from_provider", result)
	})
}

func TestSetEnv(t *testing.T) {
	key := "TEST_SET_ENV_KEY"
	defer func() {
		_ = os.Unsetenv(key)
	}()

	SetEnv(key, "test_value")
	assert.Equal(t, "test_value", os.Getenv(key))

	SetEnv(key, "new_value")
	assert.Equal(t, "test_value", os.Getenv(key))
}

func TestIsContainer(t *testing.T) {
	key := QodanaDockerEnv
	original := os.Getenv(key)
	defer func() {
		if original != "" {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	}()

	_ = os.Unsetenv(key)
	assert.False(t, IsContainer())

	_ = os.Setenv(key, "true")
	assert.True(t, IsContainer())
}

func TestInitializeAndGetQodanaGlobalEnv(t *testing.T) {
	provider := mockEnvProvider{envVars: []string{QodanaEndpointEnv + "=https://test.endpoint"}}
	InitializeQodanaGlobalEnv(provider)

	assert.Equal(t, "https://test.endpoint", GetQodanaGlobalEnv(QodanaEndpointEnv))
	assert.Equal(t, "", GetQodanaGlobalEnv("UNKNOWN_KEY"))
}

func TestIsGitLab(t *testing.T) {
	original := os.Getenv("GITLAB_CI")
	defer func() {
		if original != "" {
			_ = os.Setenv("GITLAB_CI", original)
		} else {
			_ = os.Unsetenv("GITLAB_CI")
		}
	}()

	_ = os.Unsetenv("GITLAB_CI")
	assert.False(t, IsGitLab())

	_ = os.Setenv("GITLAB_CI", "false")
	assert.False(t, IsGitLab())

	_ = os.Setenv("GITLAB_CI", "true")
	assert.True(t, IsGitLab())
}

func TestIsBitBucket(t *testing.T) {
	original := os.Getenv("BITBUCKET_PIPELINE_UUID")
	defer func() {
		if original != "" {
			_ = os.Setenv("BITBUCKET_PIPELINE_UUID", original)
		} else {
			_ = os.Unsetenv("BITBUCKET_PIPELINE_UUID")
		}
	}()

	_ = os.Unsetenv("BITBUCKET_PIPELINE_UUID")
	assert.False(t, IsBitBucket())

	_ = os.Setenv("BITBUCKET_PIPELINE_UUID", "{some-uuid}")
	assert.True(t, IsBitBucket())
}

func TestIsBitBucketPipe(t *testing.T) {
	originalStorage := os.Getenv("BITBUCKET_PIPE_STORAGE_DIR")
	originalShared := os.Getenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR")
	defer func() {
		if originalStorage != "" {
			_ = os.Setenv("BITBUCKET_PIPE_STORAGE_DIR", originalStorage)
		} else {
			_ = os.Unsetenv("BITBUCKET_PIPE_STORAGE_DIR")
		}
		if originalShared != "" {
			_ = os.Setenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR", originalShared)
		} else {
			_ = os.Unsetenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR")
		}
	}()

	_ = os.Unsetenv("BITBUCKET_PIPE_STORAGE_DIR")
	_ = os.Unsetenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR")
	assert.False(t, IsBitBucketPipe())

	_ = os.Setenv("BITBUCKET_PIPE_STORAGE_DIR", "/path")
	assert.True(t, IsBitBucketPipe())

	_ = os.Unsetenv("BITBUCKET_PIPE_STORAGE_DIR")
	_ = os.Setenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR", "/shared")
	assert.True(t, IsBitBucketPipe())
}

func TestGetBitBucketJobUrl(t *testing.T) {
	originalRepo := os.Getenv("BITBUCKET_REPO_FULL_NAME")
	originalBuild := os.Getenv("BITBUCKET_BUILD_NUMBER")
	defer func() {
		if originalRepo != "" {
			_ = os.Setenv("BITBUCKET_REPO_FULL_NAME", originalRepo)
		} else {
			_ = os.Unsetenv("BITBUCKET_REPO_FULL_NAME")
		}
		if originalBuild != "" {
			_ = os.Setenv("BITBUCKET_BUILD_NUMBER", originalBuild)
		} else {
			_ = os.Unsetenv("BITBUCKET_BUILD_NUMBER")
		}
	}()

	_ = os.Setenv("BITBUCKET_REPO_FULL_NAME", "owner/repo")
	_ = os.Setenv("BITBUCKET_BUILD_NUMBER", "123")

	expected := "https://bitbucket.org/owner/repo/pipelines/results/123"
	assert.Equal(t, expected, GetBitBucketJobUrl())
}

func TestGetBitBucketCommit(t *testing.T) {
	original := os.Getenv("BITBUCKET_COMMIT")
	defer func() {
		if original != "" {
			_ = os.Setenv("BITBUCKET_COMMIT", original)
		} else {
			_ = os.Unsetenv("BITBUCKET_COMMIT")
		}
	}()

	_ = os.Setenv("BITBUCKET_COMMIT", "abc123")
	assert.Equal(t, "abc123", GetBitBucketCommit())
}

func TestGetBitBucketRepoFunctions(t *testing.T) {
	original := os.Getenv("BITBUCKET_REPO_FULL_NAME")
	defer func() {
		if original != "" {
			_ = os.Setenv("BITBUCKET_REPO_FULL_NAME", original)
		} else {
			_ = os.Unsetenv("BITBUCKET_REPO_FULL_NAME")
		}
	}()

	_ = os.Setenv("BITBUCKET_REPO_FULL_NAME", "myowner/myrepo")
	assert.Equal(t, "myowner/myrepo", GetBitBucketRepoFullName())
	assert.Equal(t, "myowner", GetBitBucketRepoOwner())
	assert.Equal(t, "myrepo", GetBitBucketRepoName())
}

func TestUnsetRubyVariables(t *testing.T) {
	_ = os.Setenv(GemHome, "/gem/home")
	_ = os.Setenv(BundleAppConfig, "/bundle/config")

	UnsetRubyVariables()

	assert.Equal(t, "", os.Getenv(GemHome))
	assert.Equal(t, "", os.Getenv(BundleAppConfig))
}

func TestValidateBranch(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		env      string
		envVar   string
		envValue string
		expected string
	}{
		{
			name:     "branch provided",
			branch:   "main",
			env:      "github-actions",
			expected: "main",
		},
		{
			name:     "github fallback",
			branch:   "",
			env:      "github-actions",
			envVar:   "GITHUB_REF_NAME",
			envValue: "feature-branch",
			expected: "feature-branch",
		},
		{
			name:     "azure fallback",
			branch:   "",
			env:      "azure-pipelines",
			envVar:   "BUILD_SOURCEBRANCHNAME",
			envValue: "develop",
			expected: "develop",
		},
		{
			name:     "jenkins fallback",
			branch:   "",
			env:      "jenkins",
			envVar:   "GIT_BRANCH",
			envValue: "release",
			expected: "release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				_ = os.Setenv(tt.envVar, tt.envValue)
				defer func() {
					_ = os.Unsetenv(tt.envVar)
				}()
			}
			result := validateBranch(tt.branch, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateRemoteUrl(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		env      string
		expected string
	}{
		{
			name:     "valid url",
			remote:   "https://github.com/org/repo",
			env:      "github-actions",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "empty url",
			remote:   "",
			env:      "github-actions",
			expected: "",
		},
		{
			name:     "invalid url",
			remote:   "not-a-url",
			env:      "github-actions",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateRemoteUrl(tt.remote, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateJobUrl(t *testing.T) {
	tests := []struct {
		name     string
		ciUrl    string
		env      string
		expected string
	}{
		{
			name:     "valid url",
			ciUrl:    "https://ci.example.com/build/123",
			env:      "github-actions",
			expected: "https://ci.example.com/build/123",
		},
		{
			name:     "invalid url",
			ciUrl:    "not-valid",
			env:      "jenkins",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateJobUrl(tt.ciUrl, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAzureJobUrl(t *testing.T) {
	origServer := os.Getenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI")
	origProject := os.Getenv("SYSTEM_TEAMPROJECT")
	origBuild := os.Getenv("BUILD_BUILDID")
	defer func() {
		if origServer != "" {
			_ = os.Setenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", origServer)
		} else {
			_ = os.Unsetenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI")
		}
		if origProject != "" {
			_ = os.Setenv("SYSTEM_TEAMPROJECT", origProject)
		} else {
			_ = os.Unsetenv("SYSTEM_TEAMPROJECT")
		}
		if origBuild != "" {
			_ = os.Setenv("BUILD_BUILDID", origBuild)
		} else {
			_ = os.Unsetenv("BUILD_BUILDID")
		}
	}()

	_ = os.Unsetenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI")
	assert.Equal(t, "", getAzureJobUrl())

	_ = os.Setenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", "https://dev.azure.com/org/")
	_ = os.Setenv("SYSTEM_TEAMPROJECT", "MyProject")
	_ = os.Setenv("BUILD_BUILDID", "456")

	expected := "https://dev.azure.com/org/MyProject/_build/results?buildId=456"
	assert.Equal(t, expected, getAzureJobUrl())
}

func TestGetSpaceRemoteUrl(t *testing.T) {
	origApi := os.Getenv("JB_SPACE_API_URL")
	origProject := os.Getenv("JB_SPACE_PROJECT_KEY")
	origRepo := os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME")
	defer func() {
		if origApi != "" {
			_ = os.Setenv("JB_SPACE_API_URL", origApi)
		} else {
			_ = os.Unsetenv("JB_SPACE_API_URL")
		}
		if origProject != "" {
			_ = os.Setenv("JB_SPACE_PROJECT_KEY", origProject)
		} else {
			_ = os.Unsetenv("JB_SPACE_PROJECT_KEY")
		}
		if origRepo != "" {
			_ = os.Setenv("JB_SPACE_GIT_REPOSITORY_NAME", origRepo)
		} else {
			_ = os.Unsetenv("JB_SPACE_GIT_REPOSITORY_NAME")
		}
	}()

	_ = os.Unsetenv("JB_SPACE_API_URL")
	assert.Equal(t, "", getSpaceRemoteUrl())

	_ = os.Setenv("JB_SPACE_API_URL", "myorg.jetbrains.space")
	_ = os.Setenv("JB_SPACE_PROJECT_KEY", "PROJ")
	_ = os.Setenv("JB_SPACE_GIT_REPOSITORY_NAME", "myrepo")

	expected := "ssh://git@git.myorg.jetbrains.space/PROJ/myrepo.git"
	assert.Equal(t, expected, getSpaceRemoteUrl())
}
