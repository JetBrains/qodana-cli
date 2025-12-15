package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJdkTableXml(t *testing.T) {
	jdkPath := "/path/to/jdk"
	result := jdkTableXml(jdkPath)

	assert.Contains(t, result, "<application>")
	assert.Contains(t, result, "ProjectJdkTable")
	assert.Contains(t, result, jdkPath)
	assert.Contains(t, result, "<homePath")
	assert.True(t, strings.HasPrefix(result, "<application>"))
}

func TestAndroidProjectDefaultXml(t *testing.T) {
	sdkPath := "/path/to/android/sdk"
	result := androidProjectDefaultXml(sdkPath)

	assert.Contains(t, result, "<application>")
	assert.Contains(t, result, "android.sdk.path")
	assert.Contains(t, result, sdkPath)
	assert.Contains(t, result, "ProjectManager")
}

func TestWriteFileIfNew(t *testing.T) {
	tmpDir := t.TempDir()
	newFile := filepath.Join(tmpDir, "newfile.txt")
	content := "test content"

	writeFileIfNew(newFile, content)

	data, err := os.ReadFile(newFile)
	assert.NoError(t, err)
	assert.Equal(t, content, string(data))

	writeFileIfNew(newFile, "different content")

	data, err = os.ReadFile(newFile)
	assert.NoError(t, err)
	assert.Equal(t, content, string(data))
}

