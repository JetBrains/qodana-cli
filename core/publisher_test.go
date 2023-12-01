package core

import (
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFetchPublisher(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			t.Fatal(err)
		}
	}(tempDir) // clean up
	path := filepath.Join(tempDir, "publisher.jar")
	fetchPublisher(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("fetchPublisher() failed, expected %v to exists, got error: %v", path, err)
	}
}

func TestGetPublisherArgs(t *testing.T) {
	// Set up test options
	opts := &QodanaOptions{
		AnalysisId: "test-analysis-id",
		ProjectDir: "/path/to/project",
		ResultsDir: "/path/to/results",
		ReportDir:  "/path/to/report",
	}

	// Set up test environment variables
	err := os.Setenv(QodanaToolEnv, "test-tool")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Setenv(cloud.QodanaEndpoint, "test-endpoint")
	if err != nil {
		t.Fatal(err)
	}

	// Call the function being tested
	publisherArgs := getPublisherArgs(prod.jbrJava(), "test-publisher.jar", opts, "test-token", "test-endpoint")

	// Assert that the expected arguments are present
	expectedArgs := []string{
		prod.jbrJava(),
		"-jar",
		"test-publisher.jar",
		"--analysis-id", "test-analysis-id",
		"--sources-path", "/path/to/project",
		"--report-path", filepath.FromSlash("/path/to/report/results"),
		"--token", "test-token",
		"--tool", "test-tool",
		"--endpoint", "test-endpoint",
	}
	if !reflect.DeepEqual(publisherArgs, expectedArgs) {
		t.Errorf("getPublisherArgs returned incorrect arguments: got %v, expected %v", publisherArgs, expectedArgs)
	}
}
