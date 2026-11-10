package tokenloader

import (
	"os"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"
)

// saveTestCloudToken saves a token to the real keyring and removes it after the test.
func saveTestCloudToken(t *testing.T, id string, token string) {
	t.Helper()
	if err := saveCloudToken(id, token); err != nil {
		t.Skipf("Keyring not available: %v", err)
	}
	t.Cleanup(func() { _ = keyring.Delete(keyringDefaultService, id) })
}

type mockTokenLoader struct {
	qodanaToken    string
	id             string
	analyzer       product.Analyzer
	repositoryRoot string
	projectDir     string
	logDir         string
}

func (m *mockTokenLoader) GetQodanaToken() string {
	return m.qodanaToken
}

func (m *mockTokenLoader) GetId() string {
	return m.id
}

func (m *mockTokenLoader) GetAnalyzer() product.Analyzer {
	return m.analyzer
}

func (m *mockTokenLoader) GetRepositoryRoot() string {
	return m.repositoryRoot
}

func (m *mockTokenLoader) GetProjectDir() string {
	return m.projectDir
}

func (m *mockTokenLoader) GetLogDir() string {
	return m.logDir
}

func TestIsCloudTokenRequired(t *testing.T) {
	t.Run("token already provided", func(t *testing.T) {
		loader := &mockTokenLoader{
			qodanaToken: "test-token",
		}
		assert.True(t, IsCloudTokenRequired(loader))
	})

	t.Run("license-only token set", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaLicenseOnlyToken, "license-token")
		defer func() { _ = os.Unsetenv(qdenv.QodanaLicenseOnlyToken) }()

		loader := &mockTokenLoader{}
		assert.True(t, IsCloudTokenRequired(loader))
	})

	t.Run("free analyzer", func(t *testing.T) {
		loader := &mockTokenLoader{
			analyzer: &product.NativeAnalyzer{
				Linter: product.Linter{
					IsPaid: false,
				},
			},
		}
		assert.False(t, IsCloudTokenRequired(loader))
	})

	t.Run("EAP analyzer", func(t *testing.T) {
		loader := &mockTokenLoader{
			analyzer: &product.NativeAnalyzer{
				Eap: true,
			},
		}
		assert.False(t, IsCloudTokenRequired(loader))
	})

	t.Run("paid analyzer without license", func(t *testing.T) {
		loader := &mockTokenLoader{
			analyzer: &product.NativeAnalyzer{
				Linter: product.Linter{
					IsPaid: true,
				},
			},
		}
		assert.True(t, IsCloudTokenRequired(loader))
	})
}

func TestLoadCloudUploadToken(t *testing.T) {
	t.Run("token from loader", func(t *testing.T) {
		loader := &mockTokenLoader{
			qodanaToken: "test-token",
			id:          "test-id",
			projectDir:  "/test/project",
			logDir:      "/test/log",
		}
		token := LoadCloudUploadToken(loader, false, false, false)
		assert.Equal(t, "test-token", token)
	})

	t.Run("no token available", func(t *testing.T) {
		loader := &mockTokenLoader{
			id:         "nonexistent-id",
			projectDir: "/test/project",
			logDir:     "/test/log",
		}
		token := LoadCloudUploadToken(loader, false, false, false)
		assert.Empty(t, token)
	})

	t.Run("keyring is used when QODANA_ORG_TOKEN is set but not exchanged", func(t *testing.T) {
		// e.g. qodana init: the global env holds QODANA_ORG_TOKEN, but no exchange happens
		t.Setenv(qdenv.QodanaClearKeyring, "")
		qdenv.InitializeQodanaGlobalEnv(envProvider{"QODANA_ORG_TOKEN=org"})
		saveTestCloudToken(t, "keyring-id", "keyring-token")

		token := LoadCloudUploadToken(&mockTokenLoader{id: "keyring-id"}, false, true, false)
		assert.Equal(t, "keyring-token", token)
	})
}

func TestGetTokenFromKeychain(t *testing.T) {
	t.Run("refresh token", func(t *testing.T) {
		token := getTokenFromKeychain(true, "test-id")
		assert.Empty(t, token)
	})

	t.Run("clear keyring env set", func(t *testing.T) {
		_ = os.Setenv(qdenv.QodanaClearKeyring, "true")
		defer func() { _ = os.Unsetenv(qdenv.QodanaClearKeyring) }()

		token := getTokenFromKeychain(false, "test-id")
		assert.Empty(t, token)
	})

	t.Run("nonexistent token", func(t *testing.T) {
		token := getTokenFromKeychain(false, "nonexistent-token-id")
		assert.Empty(t, token)
	})
}

func TestSaveAndGetCloudToken(t *testing.T) {
	t.Run("save and get token", func(t *testing.T) {
		testID := "test-token-id-12345"
		testToken := "test-token-value"

		saveTestCloudToken(t, testID, testToken)

		retrievedToken, err := getCloudToken(testID)
		assert.NoError(t, err)
		assert.Equal(t, testToken, retrievedToken)
	})

	t.Run("get nonexistent token", func(t *testing.T) {
		_, err := getCloudToken("definitely-nonexistent-id")
		assert.Error(t, err)
	})
}
