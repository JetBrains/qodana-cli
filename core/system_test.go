package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareNugetConfig(t *testing.T) {
	_ = os.Setenv(qodanaNugetName, "qdn")
	_ = os.Setenv(qodanaNugetUrl, "test_url")
	_ = os.Setenv(qodanaNugetUser, "test_user")
	_ = os.Setenv(qodanaNugetPassword, "test_password")

	// create temp dir
	tmpDir, _ := os.MkdirTemp("", "test")
	defer func(tmpDir string) {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
	}(tmpDir)

	prepareNugetConfig(tmpDir)

	expected := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
    <add key="qdn" value="test_url" />
  </packageSources>
  <packageSourceCredentials>
    <qdn>
      <add key="Username" value="test_user" />
      <add key="ClearTextPassword" value="test_password" />
    </qdn>
  </packageSourceCredentials>
</configuration>`

	file, err := os.Open(filepath.Join(tmpDir, ".nuget", "NuGet", "NuGet.Config"))
	if err != nil {
		t.Fatal(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	var text string
	for scanner.Scan() {
		text += scanner.Text() + "\n"
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	text = strings.TrimSuffix(text, "\n")
	if text != expected {
		t.Fatalf("got:\n%s\n\nwant:\n%s", text, expected)
	}
}
