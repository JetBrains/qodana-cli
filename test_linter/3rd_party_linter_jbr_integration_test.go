package main

import (
	"archive/tar"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/mock_server.py
var mockServerPy string

//go:embed testdata/mocked-results.tar.gz
var resultsTarGz []byte

const (
	testContainerImage      = "python:3.13-slim"
	testContainerPlatform   = "linux/amd64"
	mockServerStartupDelay  = 3 * time.Second
	mockServerLogFlushDelay = 2 * time.Second
	qodanaScanTimeout       = 5 * time.Minute
)

// TestQodanaCppWithMockedCloud tests qodana scan and send commands
// while mocking qodana.cloud API responses using a Python mock server
func TestQodanaCppWithMockedCloud(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("Skipping integration test on %s (linux only)", runtime.GOOS)
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "Failed to create Docker client")
	defer func(cli *client.Client) {
		err := cli.Close()
		if err != nil {
			t.Logf("Failed to close Docker client: %v", err)
		}
	}(cli)

	// Build qodana binary if not available
	qodanaBinaryPath := ensureQodanaBinary(t)
	t.Logf("Using qodana binary: %s", qodanaBinaryPath)

	// Start test container with mock server
	containerID := startTestContainer(t, ctx, cli)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cli.ContainerStop(stopCtx, containerID, container.StopOptions{}); err != nil {
			t.Logf("Failed to stop test container %s: %v", containerID, err)
		}
		if err := cli.ContainerRemove(stopCtx, containerID, container.RemoveOptions{Force: true}); err != nil {
			t.Logf("Failed to remove test container %s: %v", containerID, err)
		}
	}()

	// Setup workspace and copy files
	setupWorkspace(t, ctx, cli, containerID, qodanaBinaryPath)

	scanOutput := runQodanaScan(t, ctx, cli, containerID)

	// Give mock server a moment to flush logs
	time.Sleep(mockServerLogFlushDelay)

	// Print HTTPS traffic
	mockLog := execInContainer(
		t, ctx, cli, containerID, []string{
			"cat", "/tmp/mock_server.log",
		},
		false, true, // not detached, but silent
	)
	t.Log(mockLog)
	assert.NotEmpty(t, mockLog, "Should have captured some HTTPS traffic")

	// Verify
	verifyIntellijReportConverter(t, ctx, cli, containerID)
	verifyBaselineCliLogs(t, scanOutput)
	verifyUsedFailThresholdFromGlobalConfiguration(t, scanOutput)

	requests := getMockRequests(t, ctx, cli, containerID)
	require.NotEmpty(t, requests, "Should have captured mock requests")
	verifyPublisherCliCalls(t, requests)
	verifyQodanaFuserCalls(t, requests)
}

// ensureQodanaBinary builds the test linter qodana binary for Linux if it doesn't exist
func ensureQodanaBinary(t *testing.T) string {
	t.Helper()

	// We need Linux binary for the debian container
	goos := "linux"
	goarch := "amd64"
	archSuffix := "v1"

	// Get project root (go up from test to project root)
	projectRoot, err := filepath.Abs(filepath.Join(".."))
	require.NoError(t, err)

	// Build from test_linter directory to test_linter/dist/qodana-cli_linux_amd64_v1/qodana-cli
	testDir := filepath.Join(projectRoot, "test_linter")
	distPath := filepath.Join(
		testDir,
		"dist",
		fmt.Sprintf("qodana-cli_%s_%s_%s", goos, goarch, archSuffix),
		"qodana-cli",
	)

	// Check if binary already exists
	if _, err := os.Stat(distPath); err == nil {
		t.Logf("Using existing Linux binary: %s", distPath)
		return distPath
	}

	// Build Linux binary using go build directly from test directory
	t.Log("Building qodana-cli Linux binary from test directory...")

	// Ensure dist directory exists
	err = os.MkdirAll(filepath.Dir(distPath), 0755)
	require.NoError(t, err, "Failed to create dist directory")

	cmd := exec.Command("go", "build", "-o", distPath, ".")
	cmd.Dir = testDir
	cmd.Env = append(
		os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	require.NoError(t, err, "Failed to build qodana binary from test directory")

	// Verify binary was created
	_, err = os.Stat(distPath)
	require.NoError(t, err, "Binary not found after build: %s", distPath)

	return distPath
}

// startTestContainer starts a Debian container with mock_server.py for mocking qodana.cloud
func startTestContainer(t *testing.T, ctx context.Context, cli *client.Client) string {
	t.Helper()

	// Pull Python slim image for amd64 (has Python and bash pre-installed)
	// Must use amd64 explicitly because qodana-cpp IDE is x86_64 only
	t.Logf("Pulling %s (%s) image...", testContainerImage, testContainerPlatform)
	reader, err := cli.ImagePull(
		ctx, testContainerImage, image.PullOptions{
			Platform: testContainerPlatform,
		},
	)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, reader)
	require.NoError(t, err)
	err = reader.Close()
	require.NoError(t, err)

	// Create container
	amd64Platform := &ocispec.Platform{
		Architecture: "amd64",
		OS:           "linux",
	}
	resp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image: testContainerImage,
			Cmd:   []string{"tail", "-f", "/dev/null"},
			Env:   getContainerEnv(),
		},
		&container.HostConfig{
			AutoRemove: false,
		},
		&network.NetworkingConfig{},
		amd64Platform,
		"",
	)
	require.NoError(t, err)

	// Start container
	err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	require.NoError(t, err)

	// Verify Java is not installed in the container
	verifyNoJavaInstalled(t, ctx, cli, resp)

	// Setup mock server
	t.Log("Setting up mock server...")
	copyFileToContainer(t, ctx, cli, resp.ID, "/tmp/mock_server.py", []byte(mockServerPy))
	execInContainer(
		t, ctx, cli, resp.ID, []string{
			"sh",
			"-c",
			"printf '127.0.0.1 mocked.qodana.cloud\n127.0.0.1 resources.jetbrains.com\n127.0.0.1 analytics.services.jetbrains.com\n' >> /etc/hosts",
		},
	)

	// Start mock server in detached mode
	execInContainer(
		t, ctx, cli, resp.ID, []string{
			"sh", "-c",
			"python3 /tmp/mock_server.py > /tmp/mock_server_stdout.log 2>&1 &",
		},
		true,
	)
	time.Sleep(mockServerStartupDelay)
	generateTruststore(t, ctx, cli, resp.ID)

	t.Logf("Test container started: %s", resp.ID)
	return resp.ID
}

func verifyNoJavaInstalled(t *testing.T, ctx context.Context, cli *client.Client, resp container.CreateResponse) {
	t.Log("Verifying Java is not installed...")
	javaHomeCheck := execInContainer(t, ctx, cli, resp.ID, []string{"sh", "-c", "echo $JAVA_HOME"}, false, true)
	assert.Empty(t, strings.TrimSpace(javaHomeCheck), "JAVA_HOME should not be set")

	javaCheck := execInContainer(
		t,
		ctx,
		cli,
		resp.ID,
		[]string{"sh", "-c", "which java 2>/dev/null || true"},
		false,
		true,
	)
	assert.Empty(t, strings.TrimSpace(javaCheck), "java executable should not be found")
}

func generateTruststore(t *testing.T, ctx context.Context, cli *client.Client, containerID string) {
	t.Helper()

	output := execInContainer(
		t, ctx, cli, containerID, []string{
			"sh",
			"-c",
			"set -e; test -s /tmp/mock_server.crt; test -s /tmp/mock_server.key; " +
				"openssl pkcs12 -export -out /tmp/test-truststore.p12 " +
				"-inkey /tmp/mock_server.key -in /tmp/mock_server.crt " +
				"-passout pass:changeit -name mock-server; " +
				"test -s /tmp/test-truststore.p12; echo ok",
		},
		false, true,
	)
	require.Contains(t, output, "ok", "failed to generate /tmp/test-truststore.p12")
}

// execInContainer executes a command in a container
// When detach is false (default): streams output to console and returns it
// When detach is true: runs in background without TTY (for long-running processes)
// When silent is true: captures output but doesn't print to console
func execInContainer(
	t *testing.T,
	ctx context.Context,
	cli *client.Client,
	containerID string,
	cmd []string,
	detach ...bool,
) string {
	t.Helper()

	shouldDetach := false
	silent := false
	if len(detach) > 0 {
		shouldDetach = detach[0]
	}
	if len(detach) > 1 {
		silent = detach[1]
	}

	// Create exec with appropriate options based on mode
	execOpts := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: !shouldDetach,
		AttachStderr: !shouldDetach,
		Tty:          !shouldDetach,
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execOpts)
	require.NoError(t, err)

	// Detached mode: start and return immediately
	if shouldDetach {
		require.NoError(t, cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{Detach: true}))
		return ""
	}

	// Normal mode: attach and capture output

	resp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{Tty: true})
	require.NoError(t, err)
	defer resp.Close()

	var output bytes.Buffer
	buf := make([]byte, 4096)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			n, err := resp.Reader.Read(buf)
			if n > 0 {
				if !silent {
					_, err := os.Stdout.Write(buf[:n])
					if err != nil {
						return
					}
				}
				output.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Log("Command timed out")
	}

	return output.String()
}

// MockRequest represents a captured request/response from the mock server
type MockRequest struct {
	Timestamp      string      `json:"timestamp"`
	Method         string      `json:"method"`
	Path           string      `json:"path"`
	RequestBody    interface{} `json:"request_body"`
	ResponseStatus int         `json:"response_status"`
	ResponseBody   interface{} `json:"response_body"`
}

// getMockRequests retrieves and parses the JSON lines file with captured requests
func getMockRequests(t *testing.T, ctx context.Context, cli *client.Client, containerID string) []MockRequest {
	t.Helper()

	// Read JSON lines file from container
	jsonlContent := execInContainer(
		t, ctx, cli, containerID, []string{
			"sh", "-c", "cat /tmp/mock_requests.jsonl 2>/dev/null || echo ''",
		},
		false, true, // not detached, but silent
	)

	var requests []MockRequest
	for _, line := range strings.Split(jsonlContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var req MockRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			t.Logf("Warning: failed to parse request line: %v", err)
			continue
		}
		requests = append(requests, req)
	}

	return requests
}

// findRequest finds the first request matching the given method and path
func findRequest(requests []MockRequest, method, path string) *MockRequest {
	for i := range requests {
		if requests[i].Method == method && requests[i].Path == path {
			return &requests[i]
		}
	}
	return nil
}

// copyFileToContainer copies a single file to the container
func copyFileToContainer(
	t *testing.T,
	ctx context.Context,
	cli *client.Client,
	containerID, destPath string,
	content []byte,
) {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := tw.WriteHeader(
		&tar.Header{
			Name: strings.TrimPrefix(destPath, "/"),
			Mode: 0755,
			Size: int64(len(content)),
		},
	)
	require.NoError(t, err)
	_, err = tw.Write(content)
	require.NoError(t, err)
	err = tw.Close()
	require.NoError(t, err)

	require.NoError(t, cli.CopyToContainer(ctx, containerID, "/", &buf, container.CopyToContainerOptions{}))
}

// getContainerEnv returns environment variables for the test container
func getContainerEnv() []string {
	return []string{
		"QODANA_TOKEN=Test123",
		"QODANA_ENDPOINT=https://mocked.qodana.cloud",
		"NONINTERACTIVE=1",
		"CI=true",
		"JAVA_TOOL_OPTIONS=-Djavax.net.ssl.trustStore=/tmp/test-truststore.p12 -Djavax.net.ssl.trustStorePassword=changeit -Djavax.net.ssl.trustStoreType=PKCS12",
		"SSL_CERT_FILE=/tmp/mock_server.crt",
	}
}

// setupWorkspace creates workspace directory and copies files
func setupWorkspace(t *testing.T, ctx context.Context, cli *client.Client, containerID, qodanaBinaryPath string) {
	t.Helper()

	execInContainer(t, ctx, cli, containerID, []string{"mkdir", "-p", "/workspace"})

	// Copy and unpack results tar.gz
	copyFileToContainer(t, ctx, cli, containerID, "/tmp/results.tar.gz", resultsTarGz)
	execInContainer(t, ctx, cli, containerID, []string{"mkdir", "-p", "/workspace/results"})
	execInContainer(
		t,
		ctx,
		cli,
		containerID,
		[]string{"tar", "-xzf", "/tmp/results.tar.gz", "-C", "/workspace/results"},
	)

	qodanaBinary, err := os.ReadFile(qodanaBinaryPath)
	require.NoError(t, err)
	copyFileToContainer(t, ctx, cli, containerID, "/qodana", qodanaBinary)
}

// runQodanaScan executes a qodana command with timeout and returns the output
func runQodanaScan(
	t *testing.T,
	ctx context.Context,
	cli *client.Client,
	containerID string,
) string {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(ctx, qodanaScanTimeout)
	defer cancel()

	args := []string{
		"/qodana", "scan",
		"--project-dir", "/workspace",
		"--results-dir", "/workspace/results",
		"--baseline", "/workspace/results/qodana.sarif-baseline.json",
	}

	return execInContainer(t, cmdCtx, cli, containerID, args)
}

func verifyBaselineCliLogs(t *testing.T, scanOutput string) {
	t.Helper()
	t.Log("Verifying baseline CLI logs...")
	assert.Regexp(t, `Type can be replaced with auto\s+NEW\s+note\s+1`, scanOutput)
	assert.Regexp(t, `Local variable can be made const\s+UNCHANGED\s+note\s+4`, scanOutput)
	t.Log("✓ Baseline CLI logs verified")
}

func verifyUsedFailThresholdFromGlobalConfiguration(t *testing.T, scanOutput string) {
	t.Helper()
	t.Log("Verifying config-loader-cli applied global configuration file...")
	assert.Contains(t, scanOutput, `The number of problems exceeds the fail threshold`)
	t.Log("✓ Config-loader-cli global configuration config verified")
}

func verifyPublisherCliCalls(t *testing.T, requests []MockRequest) {
	t.Helper()
	t.Log("Verifying publisher CLI calls")

	// 1. Verify initial POST call to /api/v1/reports/
	reportsReq := findRequest(requests, "POST", "/api/v1/reports")
	require.NotNil(t, reportsReq, "POST /api/v1/reports was not made - scan may have failed or timed out")

	assert.Equal(t, 200, reportsReq.ResponseStatus, "POST /api/v1/reports should return 200")

	requestBody, ok := reportsReq.RequestBody.(map[string]interface{})
	require.True(t, ok, "Request body should be a JSON object")

	reportType, ok := requestBody["type"].(string)
	require.True(t, ok, "Request body should contain 'type' field")
	assert.Equal(t, "sarif", reportType, "Report type should be 'sarif'")

	files, ok := requestBody["files"].([]interface{})
	require.True(t, ok, "Request body should contain 'files' list")
	require.Greater(t, len(files), 10, "Request body should contain at least 10 files")
	t.Logf("POST /api/v1/reports request contains %d files", len(files))

	// 2. Verify 3 PUT calls to /api/v1/s3mock/{filepath} with file uploads
	expectedPutPaths := []string{
		"/api/v1/s3mock/qodana.sarif.json",
		"/api/v1/s3mock/qodana-short.sarif.json",
		"/api/v1/s3mock/log/idea.log",
	}

	for _, expectedPath := range expectedPutPaths {
		putReq := findRequest(requests, "PUT", expectedPath)
		require.NotNil(t, putReq, "Should have PUT request to %s", expectedPath)
		assert.Equal(t, 200, putReq.ResponseStatus, "PUT %s should return 200", expectedPath)

		if putReq.RequestBody == "<binary>" {
			t.Logf("PUT %s: binary content uploaded", expectedPath)
		} else {
			if bodyStr, ok := putReq.RequestBody.(string); ok {
				require.Greater(t, len(bodyStr), 1024, "PUT %s body should be > 1kB", expectedPath)
				t.Logf("PUT %s: %d bytes uploaded", expectedPath, len(bodyStr))
			}
		}
	}

	// 3. Verify POST call to /api/v1/reports/mock-report-12345/finish
	finishReq := findRequest(requests, "POST", "/api/v1/reports/mock-report-12345/finish")
	require.NotNil(t, finishReq, "Should have POST request to /api/v1/reports/mock-report-12345/finish")

	t.Log("✓ Publisher-cli report upload workflow verified")
}

func verifyQodanaFuserCalls(t *testing.T, requests []MockRequest) {
	t.Helper()
	t.Log("Verifying qodana-fuser calls")

	// 1. Verify GET call to /storage/fus/config/v4/FUS/QDTEST.json
	fusReq := findRequest(requests, "GET", "/storage/fus/config/v4/FUS/QDTEST.json")
	require.NotNil(t, fusReq, "FUS config request was not captured")

	// 2. Verify GET call to /storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json
	groupsReq := findRequest(requests, "GET", "/storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json")
	require.NotNil(t, groupsReq, "FUS groups request was not captured")

	// 3. Verify POST call to /fus/v5/send/
	sendReq := findRequest(requests, "POST", "/fus/v5/send")
	if sendReq == nil {
		sendReq = findRequest(requests, "POST", "/fus/v5/send/")
	}
	require.NotNil(t, sendReq, "FUS send request was not captured")

	body, ok := sendReq.RequestBody.(map[string]interface{})
	require.True(t, ok, "sendReq.requestBody should be a JSON object")

	assert.Equal(t, "FUS", body["recorder"])
	assert.Equal(t, "QDTEST", body["product"])

	records, ok := body["records"].([]interface{})
	require.True(t, ok, "requestBody.records should be an array")
	require.NotEmpty(t, records, "requestBody.records should not be empty")

	firstRec, ok := records[0].(map[string]interface{})
	require.True(t, ok, "requestBody.records[0] should be an object")

	events, ok := firstRec["events"].([]interface{})
	require.True(t, ok, "requestBody.records[0].events should be an array")
	require.GreaterOrEqual(t, len(events), 2, "requestBody.records[0].events should have at least 2 elements")
	t.Logf("✓ Final FUS send request contained %d events", len(events))
}

func verifyIntellijReportConverter(t *testing.T, ctx context.Context, cli *client.Client, containerID string) {
	t.Helper()
	t.Log("Verifying IntelliJ report converter output...")
	reportDir := "/workspace/results/report"
	verifyFileExists := func(filename string) {
		output := execInContainer(
			t, ctx, cli, containerID,
			[]string{"sh", "-c", fmt.Sprintf("test -f %s/%s && echo ok || echo missing", reportDir, filename)},
			false, true,
		)
		assert.Contains(t, output, "ok", "%s should exist in %s", filename, reportDir)
	}

	verifyDirectoryExists := func(dirname string) {
		output := execInContainer(
			t, ctx, cli, containerID,
			[]string{"sh", "-c", fmt.Sprintf("test -d %s/%s && echo ok || echo missing", reportDir, dirname)},
			false, true,
		)
		assert.Contains(t, output, "ok", "%s directory should exist in %s", dirname, reportDir)
	}

	verifyFileExists("idea.html")
	verifyFileExists("index.html")
	verifyDirectoryExists("js")
	verifyDirectoryExists("css")

	t.Log("✓ IntelliJ report converter output verified")
}
