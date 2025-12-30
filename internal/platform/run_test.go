package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
)

func TestQodanaLogo(t *testing.T) {
	t.Run("with EAP", func(t *testing.T) {
		logo := qodanaLogo("Test Tool", "1.0.0", true)
		assert.Contains(t, logo, "Test Tool")
		assert.Contains(t, logo, "1.0.0")
		assert.Contains(t, logo, "EAP")
		assert.Contains(t, logo, "https://jb.gg/qodana-docs")
	})

	t.Run("without EAP", func(t *testing.T) {
		logo := qodanaLogo("Test Tool", "2.0.0", false)
		assert.Contains(t, logo, "Test Tool")
		assert.Contains(t, logo, "2.0.0")
		assert.NotContains(t, logo, "EAP")
	})
}

func TestPrintQodanaLogo(t *testing.T) {
	t.Run("print logo", func(t *testing.T) {
		logDir := t.TempDir()
		cacheDir := t.TempDir()
		linterInfo := thirdpartyscan.LinterInfo{
			LinterPresentableName: "Test Linter",
			LinterVersion:         "1.0.0",
			IsEap:                 true,
		}
		printQodanaLogo(logDir, cacheDir, linterInfo)
	})
}

func TestPrintLinterLicense(t *testing.T) {
	t.Run("community license", func(t *testing.T) {
		linterInfo := thirdpartyscan.LinterInfo{
			LinterPresentableName: "Test",
			IsEap:                 false,
		}
		printLinterLicense("community", linterInfo)
	})

	t.Run("EAP license", func(t *testing.T) {
		linterInfo := thirdpartyscan.LinterInfo{
			LinterPresentableName: "Test",
			IsEap:                 true,
		}
		printLinterLicense("", linterInfo)
	})
}

func TestCopyQodanaYamlToLogDir(t *testing.T) {
	t.Run("copy existing file", func(t *testing.T) {
		srcDir := t.TempDir()
		logDir := t.TempDir()

		yamlPath := filepath.Join(srcDir, "qodana.yaml")
		err := os.WriteFile(yamlPath, []byte("test: value"), 0644)
		assert.NoError(t, err)

		err = copyQodanaYamlToLogDir(yamlPath, logDir)
		assert.NoError(t, err)

		copiedPath := filepath.Join(logDir, "qodana.yaml")
		content, err := os.ReadFile(copiedPath)
		assert.NoError(t, err)
		assert.Equal(t, "test: value", string(content))
	})

	t.Run("non-existent file", func(t *testing.T) {
		logDir := t.TempDir()
		err := copyQodanaYamlToLogDir("/nonexistent/qodana.yaml", logDir)
		assert.NoError(t, err)
	})
}

