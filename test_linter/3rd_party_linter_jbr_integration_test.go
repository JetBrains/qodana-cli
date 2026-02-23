package main

import (
	"archive/tar"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	composeFile             = "testdata/docker-compose.test.yml"
	composeProjectName      = "qodana-test"
	composeServiceName      = "qodana-test-mock"
	mockServerLogFlushDelay = 2 * time.Second
	qodanaScanTimeout       = 1 * time.Minute
)

var bgCtx = context.Background()

func composeCmd(args ...string) *exec.Cmd {
	baseArgs := []string{"compose", "-f", composeFile, "-p", composeProjectName}
	return exec.Command("docker", append(baseArgs, args...)...)
}

// TestQodana3rdPartyLinterWithMockedCloud test mock 3rd party linter
// with mocked qodana.cloud and FUS endpoints
// verifies usage of all libraries run with qodana-jbr
func TestQodana3rdPartyLinterWithMockedCloud(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if os.Getenv("CI") == "true" && runtime.GOOS != "linux" {
		t.Skip("Skipping container test on non-linux CI")
	}

	cli := createDockerClient(t)
	defer closeDockerClient(t, cli)

	startDockerCompose(t)
	defer stopDockerCompose(t)

	containerID := getComposeContainerID(t)

	waitForMockServer(t, cli, containerID)

	setupWorkspace(t, cli, containerID)

	scanOutput := runQodanaScan(t, cli, containerID)

	// Verify
	mockLog := getMockedTrafficLogs(t, cli, containerID)
	assert.NotEmpty(t, mockLog, "Should have captured some HTTPS traffic")

	verifyIntellijReportConverter(t, cli, containerID)
	verifyBaselineCliLogs(t, scanOutput)
	verifyUsedFailThresholdFromGlobalConfiguration(t, scanOutput)

	mockRequests := getMockRequests(t, cli, containerID)
	require.NotEmpty(t, mockRequests, "Should have captured mock requests")
	verifyPublisherCliCalls(t, mockRequests)
	verifyQodanaFuserCalls(t, mockRequests)
}

func createDockerClient(t *testing.T) *client.Client {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "Failed to create Docker client")
	return cli
}

func closeDockerClient(t *testing.T, cli *client.Client) {
	func() {
		if err := cli.Close(); err != nil {
			t.Logf("Failed to close Docker client: %v", err)
		}
	}()
}

func getMockedTrafficLogs(t *testing.T, cli *client.Client, containerID string) string {
	time.Sleep(mockServerLogFlushDelay)
	mockLog := execInContainer(
		t, cli, containerID, []string{
			"cat", "/tmp/mock_server.log",
		},
		true, // silent
	)
	t.Log(mockLog)
	return mockLog
}

// buildQodanaBinary builds the test linter qodana binary for Linux if it doesn't exist
func buildQodanaBinary(t *testing.T) string {
	t.Helper()
	t.Log("Building qodana-cli Linux binary...")

	projectRoot, err := filepath.Abs("..")
	require.NoError(t, err)

	testDir := filepath.Join(projectRoot, "test_linter")
	distPath := filepath.Join(testDir, "dist", "qodana-cli_linux_amd64_v1", "qodana-cli")

	require.NoError(t, os.MkdirAll(filepath.Dir(distPath), 0755))

	cmd := exec.Command("go", "build", "-o", distPath, ".")
	cmd.Dir = testDir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run(), "Failed to build qodana binary")
	require.FileExists(t, distPath)

	t.Logf("Using qodana binary: %s", distPath)
	return distPath
}

func startDockerCompose(t *testing.T) {
	t.Helper()
	t.Log("Starting docker-compose stack...")

	cmd := composeCmd("up", "-d", "--build")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("docker-compose up output: %s", string(output))
	}
	require.NoError(t, err, "Failed to start docker-compose")
}

func stopDockerCompose(t *testing.T) {
	t.Helper()
	t.Log("Stopping docker-compose stack...")

	cmd := composeCmd("down", "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("docker-compose down output: %s", string(output))
		t.Logf("Failed to stop docker-compose: %v", err)
	}
}

func getComposeContainerID(t *testing.T) string {
	t.Helper()

	cmd := composeCmd("ps", "-q", composeServiceName)
	output, err := cmd.Output()
	require.NoError(t, err, "Failed to get container ID from docker-compose")

	containerID := strings.TrimSpace(string(output))
	require.NotEmpty(t, containerID, "Container ID is empty")

	t.Logf("Test container started: %s", containerID)
	return containerID
}

func waitForMockServer(t *testing.T, cli *client.Client, containerID string) {
	t.Helper()
	t.Log("Waiting for mock server to start...")

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for mock server health check")
		case <-ticker.C:
			inspect, err := cli.ContainerInspect(bgCtx, containerID)
			if err != nil {
				continue
			}
			if inspect.State.Health != nil && inspect.State.Health.Status == "healthy" {
				t.Log("Mock server has started")
				return
			}
		}
	}
}

// execInContainer executes a command in a container and returns output
func execInContainer(
	t *testing.T,
	cli *client.Client,
	containerID string,
	cmd []string,
	silent bool,
	ctx ...context.Context,
) string {
	t.Helper()

	execCtx := bgCtx
	if len(ctx) > 0 && ctx[0] != nil {
		execCtx = ctx[0]
	}

	execOpts := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}

	execID, err := cli.ContainerExecCreate(execCtx, containerID, execOpts)
	require.NoError(t, err)

	resp, err := cli.ContainerExecAttach(execCtx, execID.ID, container.ExecStartOptions{Tty: true})
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
	case <-execCtx.Done():
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
// Returns an indexed map for O(1) lookups by method+path
func getMockRequests(t *testing.T, cli *client.Client, containerID string) map[string]*MockRequest {
	t.Helper()

	jsonlContent := execInContainer(
		t, cli, containerID, []string{
			"sh", "-c", "cat /tmp/mock_requests.jsonl 2>/dev/null || echo ''",
		},
		true, // silent
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

	reqMap := make(map[string]*MockRequest)
	for i := range requests {
		key := requests[i].Method + " " + requests[i].Path
		reqMap[key] = &requests[i]
	}
	return reqMap
}

// findRequest finds the first request matching the given method and path from indexed map
func findRequest(reqMap map[string]*MockRequest, method, path string) *MockRequest {
	return reqMap[method+" "+path]
}

// copyFileToContainer copies a single file to the container
func copyFileToContainer(
	t *testing.T,
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

	require.NoError(t, cli.CopyToContainer(bgCtx, containerID, "/", &buf, container.CopyToContainerOptions{}))
}

// setupWorkspace copies qodana binary to container (test results already copied by init container)
func setupWorkspace(t *testing.T, cli *client.Client, containerID string) {
	t.Helper()

	// Build qodana binary
	qodanaBinaryPath := buildQodanaBinary(t)

	// Copy qodana binary
	qodanaBinary, err := os.ReadFile(qodanaBinaryPath)
	require.NoError(t, err)
	copyFileToContainer(t, cli, containerID, "/qodana", qodanaBinary)

	// Verify test results exist from init container
	output := execInContainer(
		t, cli, containerID,
		[]string{"sh", "-c", "test -f /workspace/results/qodana.sarif.json && echo ok"},
		true, // silent
	)
	require.Contains(t, output, "ok", "init container should have copied test result files")
}

// runQodanaScan executes a qodana command with timeout and returns the output
func runQodanaScan(
	t *testing.T,
	cli *client.Client,
	containerID string,
) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(bgCtx, qodanaScanTimeout)
	defer cancel()

	args := []string{
		"/qodana", "scan",
		"--project-dir", "/workspace",
		"--results-dir", "/workspace/results",
		"--baseline", "/workspace/results/qodana.sarif-baseline.json",
	}

	return execInContainer(t, cli, containerID, args, false, ctx)
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

func verifyPublisherCliCalls(t *testing.T, reqMap map[string]*MockRequest) {
	t.Helper()
	t.Log("Verifying publisher CLI calls")

	// 1. Verify initial POST call to /api/v1/reports/
	reportsReq := findRequest(reqMap, "POST", "/api/v1/reports")
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
		putReq := findRequest(reqMap, "PUT", expectedPath)
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
	finishReq := findRequest(reqMap, "POST", "/api/v1/reports/mock-report-12345/finish")
	require.NotNil(t, finishReq, "Should have POST request to /api/v1/reports/mock-report-12345/finish")

	t.Log("✓ Publisher-cli report upload workflow verified")
}

func verifyQodanaFuserCalls(t *testing.T, reqMap map[string]*MockRequest) {
	t.Helper()
	t.Log("Verifying qodana-fuser calls")

	// 1. Verify GET call to /storage/fus/config/v4/FUS/QDTEST.json
	fusReq := findRequest(reqMap, "GET", "/storage/fus/config/v4/FUS/QDTEST.json")
	require.NotNil(t, fusReq, "FUS config request was not captured")

	// 2. Verify GET call to /storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json
	groupsReq := findRequest(reqMap, "GET", "/storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json")
	require.NotNil(t, groupsReq, "FUS groups request was not captured")

	// 3. Verify POST call to /fus/v5/send/
	sendReq := findRequest(reqMap, "POST", "/fus/v5/send")
	if sendReq == nil {
		sendReq = findRequest(reqMap, "POST", "/fus/v5/send/")
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

func verifyIntellijReportConverter(t *testing.T, cli *client.Client, containerID string) {
	t.Helper()
	t.Log("Verifying IntelliJ report converter output...")
	converterReportResultsDir := "/workspace/results/report/results"
	verifyFileExists := func(filename string) {
		output := execInContainer(
			t, cli, containerID,
			[]string{
				"sh", "-c", fmt.Sprintf(
					"test -f %s/%s && echo ok || echo missing",
					converterReportResultsDir, filename,
				),
			},
			true, // silent
		)
		assert.Contains(t, output, "ok", "%s should exist in %s", filename, converterReportResultsDir)
	}

	verifyFileExists("result-allProblems.json")
	verifyFileExists("metaInformation.json")
	verifyFileExists("coverageInformation.json")
	verifyFileExists("sanity.json")
	verifyFileExists("promo.json")

	t.Log("✓ IntelliJ report converter output verified")
}
