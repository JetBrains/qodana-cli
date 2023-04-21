/*
 * Copyright 2021-2022 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	cliconfig "github.com/docker/cli/cli/config"

	"github.com/cucumber/ci-environment/go"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

const (
	// QodanaSuccessExitCode is Qodana exit code when the analysis is successfully completed.
	QodanaSuccessExitCode = 0
	// QodanaFailThresholdExitCode same as QodanaSuccessExitCode, but the threshold is set and exceeded.
	QodanaFailThresholdExitCode = 255
	// QodanaOutOfMemoryExitCode reports an interrupted process, sometimes because of an OOM.
	QodanaOutOfMemoryExitCode = 137
	// QodanaEapLicenseExpiredExitCode reports an expired license.
	QodanaEapLicenseExpiredExitCode = 7
	// officialImagePrefix is the prefix of official Qodana images.
	officialImagePrefix      = "jetbrains/qodana"
	dockerSpecialCharsLength = 8
)

var (
	containerLogsOptions = types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	}
	containerName = "qodana-cli"
)

const (
	QodanaEnv              = "QODANA_ENV"
	QodanaToken            = "QODANA_TOKEN"
	QodanaJobUrl           = "QODANA_JOB_URL"
	QodanaRemoteUrl        = "QODANA_REMOTE_URL"
	QodanaBranch           = "QODANA_BRANCH"
	QodanaRevision         = "QODANA_REVISION"
	qodanaCliContainerName = "QODANA_CLI_CONTAINER_NAME"
	qodanaCliContainerKeep = "QODANA_CLI_CONTAINER_KEEP"
	qodanaCliUsePodman     = "QODANA_CLI_USE_PODMAN"
)

// encodeAuthToBase64 serializes the auth configuration as JSON base64 payload
func encodeAuthToBase64(authConfig types.AuthConfig) (string, error) {
	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

// ExtractQodanaEnvironment extracts Qodana env variables QODANA_* to the given environment array.
func ExtractQodanaEnvironment(opts *QodanaOptions) {
	opts.Setenv(QodanaToken, os.Getenv(QodanaToken))
	ci := cienvironment.DetectCIEnvironment()
	qEnv := "cli"
	if ci != nil {
		qEnv = strings.ReplaceAll(strings.ToLower(ci.Name), " ", "-")
		opts.Setenv(QodanaJobUrl, validateCiUrl(ci.URL, qEnv))
		if ci.Git != nil {
			opts.Setenv(QodanaRemoteUrl, ci.Git.Remote)
			opts.Setenv(QodanaBranch, ci.Git.Branch)
			opts.Setenv(QodanaRevision, ci.Git.Revision)
		}
	}
	opts.Setenv(QodanaEnv, fmt.Sprintf("%s:%s", qEnv, Version))
}

func validateCiUrl(ciUrl string, qEnv string) string {
	if strings.HasPrefix(qEnv, "azure") { // temporary workaround for Azure Pipelines
		return getAzureJobUrl()
	}
	_, err := url.ParseRequestURI(ciUrl)
	if err != nil {
		return ""
	}
	return ciUrl
}

func checkRequiredToolInstalled(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// CheckContainerHost checks if the host is ready to run Qodana container images.
func CheckContainerHost() {
	var tool string
	if os.Getenv(qodanaCliUsePodman) == "" && checkRequiredToolInstalled("docker") {
		tool = "docker"
	} else if checkRequiredToolInstalled("podman") {
		tool = "podman"
	} else {
		ErrorMessage(
			"Docker (or podman) is not installed on your system or can't be found in PATH, refer to https://www.docker.com/get-started for installing it",
		)
		os.Exit(1)
	}
	cmd := exec.Command(tool, "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exiterr.Stderr), "permission denied") {
				ErrorMessage(
					"Qodana container can't be run by the current user. Please fix your container engine configuration.",
				)
				WarningMessage("https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user")
				os.Exit(1)
			} else {
				ErrorMessage(
					"'%s ps' exited with exit code %d, perhaps docker daemon is not running?",
					tool,
					exiterr.ExitCode(),
				)
			}
			os.Exit(1)
		}
		log.Fatal(err)
	}
	CheckContainerEngineMemory()
}

// PullImage pulls docker image and prints the process.
func PullImage(client *client.Client, image string) {
	printProcess(
		func() {
			pullImage(context.Background(), client, image)
		},
		fmt.Sprintf("Pulling the image %s", PrimaryBold(image)),
		"pulling the latest version of linter",
	)
}

func isDockerUnauthorizedError(errMsg string) bool {
	errMsg = lower(errMsg)
	return strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "denied") || strings.Contains(errMsg, "forbidden")
}

// PullImage pulls docker image.
func pullImage(ctx context.Context, client *client.Client, image string) {
	reader, err := client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil && isDockerUnauthorizedError(err.Error()) {
		cfg, err := cliconfig.Load("")
		if err != nil {
			log.Fatal(err)
		}
		registryHostname := strings.Split(image, "/")[0]
		a, err := cfg.GetAuthConfig(registryHostname)
		if err != nil {
			log.Fatal("can't load the auth config", err)
		}
		encodedAuth, err := encodeAuthToBase64(types.AuthConfig(a))
		if err != nil {
			log.Fatal("can't encode auth to base64", err)
		}
		reader, err = client.ImagePull(ctx, image, types.ImagePullOptions{RegistryAuth: encodedAuth})
		if err != nil {
			log.Fatal("can't pull image from the private registry", err)
		}
	} else if err != nil {
		log.Fatal("can't pull image ", err)
	}
	defer func(pull io.ReadCloser) {
		err := pull.Close()
		if err != nil {
			log.Fatal("can't pull image ", err)
		}
	}(reader)
	if _, err = io.Copy(io.Discard, reader); err != nil {
		log.Fatal("couldn't read the image pull logs ", err)
	}
}

// ContainerCleanup cleans up Qodana containers.
func ContainerCleanup() {
	if containerName != "qodana-cli" { // if containerName is not set, it means that the container was not created!
		docker := getContainerClient()
		ctx := context.Background()
		containers, err := docker.ContainerList(ctx, types.ContainerListOptions{})
		if err != nil {
			log.Fatal("couldn't get the running containers ", err)
		}
		for _, c := range containers {
			if c.Names[0] == fmt.Sprintf("/%s", containerName) {
				err = docker.ContainerStop(context.Background(), c.Names[0], nil)
				if err != nil {
					log.Fatal("couldn't stop the container ", err)
				}
			}
		}
	}
}

// CheckContainerEngineMemory applicable only for Docker Desktop,
// (has the default limit of 2GB which can be not enough when Gradle runs inside a container).
func CheckContainerEngineMemory() {
	docker := getContainerClient()
	goos := runtime.GOOS
	if //goland:noinspection GoBoolExpressions
	goos != "windows" && goos != "darwin" {
		return
	}
	info, err := docker.Info(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	var helpUrl string
	switch goos {
	case "windows":
		helpUrl = "https://docs.docker.com/desktop/settings/windows/#advanced"
	case "darwin":
		helpUrl = "https://docs.docker.com/desktop/settings/mac/#advanced-1"
	}
	log.Debug("Docker memory limit is set to ", info.MemTotal/1024/1024, " MB")

	if info.MemTotal < 4*1024*1024*1024 {
		WarningMessage(`Your container daemon is running with less than 4GB of RAM.
   If you experience issues, consider increasing the container runtime memory limit.
   Refer to %s for more information.
`,
			helpUrl,
		)
	}
}

// GetCmdOptions returns qodana command options.
func GetCmdOptions(opts *QodanaOptions) []string {
	arguments := make([]string, 0)
	if opts.SaveReport {
		arguments = append(arguments, "--save-report")
	}
	if opts.SourceDirectory != "" {
		arguments = append(arguments, "--source-directory", opts.SourceDirectory)
	}
	if opts.DisableSanity {
		arguments = append(arguments, "--disable-sanity")
	}
	if opts.ProfileName != "" {
		arguments = append(arguments, "--profile-name", opts.ProfileName)
	}
	if opts.ProfilePath != "" {
		arguments = append(arguments, "--profile-path", opts.ProfilePath)
	}
	if opts.RunPromo != "" {
		arguments = append(arguments, "--run-promo", opts.RunPromo)
	}
	if opts.Script != "default" {
		arguments = append(arguments, "--script", opts.Script)
	}
	if opts.StubProfile != "" {
		arguments = append(arguments, "--stub-profile", opts.StubProfile)
	}
	if opts.Baseline != "" {
		arguments = append(arguments, "--baseline", opts.Baseline)
	}
	if opts.BaselineIncludeAbsent {
		arguments = append(arguments, "--baseline-include-absent")
	}
	if opts.FailThreshold != "" {
		arguments = append(arguments, "--fail-threshold", opts.FailThreshold)
	}
	if opts.GitReset && opts.Commit != "" && opts.Script == "default" {
		arguments = append(arguments, "--script", "local-changes")
	}
	if opts.AnalysisId != "" {
		arguments = append(arguments, "--analysis-id", opts.AnalysisId)
	}
	for _, property := range opts.Property {
		arguments = append(arguments, "--property="+property)
	}
	return arguments
}

// getDockerOptions returns qodana docker container options.
func getDockerOptions(opts *QodanaOptions) *types.ContainerCreateConfig {
	cmdOpts := GetCmdOptions(opts)
	ExtractQodanaEnvironment(opts)
	cachePath, err := filepath.Abs(opts.CacheDir)
	if err != nil {
		log.Fatal("couldn't get abs path for cache", err)
	}
	projectPath, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		log.Fatal("couldn't get abs path for project", err)
	}
	resultsPath, err := filepath.Abs(opts.ResultsDir)
	if err != nil {
		log.Fatal("couldn't get abs path for results", err)
	}
	containerName = os.Getenv(qodanaCliContainerName)
	if containerName == "" {
		containerName = fmt.Sprintf("qodana-cli-%s", getId(projectPath))
	}
	volumes := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: cachePath,
			Target: "/data/cache",
		},
		{
			Type:   mount.TypeBind,
			Source: projectPath,
			Target: "/data/project",
		},
		{
			Type:   mount.TypeBind,
			Source: resultsPath,
			Target: "/data/results",
		},
	}
	for _, volume := range opts.Volumes {
		source, target := extractDockerVolumes(volume)
		if source != "" && target != "" {
			volumes = append(volumes, mount.Mount{
				Type:   mount.TypeBind,
				Source: source,
				Target: target,
			})
		} else {
			log.Fatal("couldn't parse volume ", volume)
		}
	}
	log.Debugf("image: %s", opts.Linter)
	log.Debugf("container name: %s", containerName)
	log.Debugf("user: %s", opts.User)
	log.Debugf("env: %v", opts.Env)
	log.Debugf("volumes: %v", volumes)
	log.Debugf("cmd: %v", cmdOpts)
	log.Debugf("docker command to debug: docker run --rm -it -u %s -v %s:/data/cache -v %s:/data/project -v %s:/data/results %s %s", opts.User, cachePath, projectPath, resultsPath, opts.Linter, strings.Join(cmdOpts, " "))
	return &types.ContainerCreateConfig{
		Name: containerName,
		Config: &container.Config{
			Image:        opts.Linter,
			Cmd:          cmdOpts,
			Tty:          IsInteractive(),
			AttachStdout: true,
			AttachStderr: true,
			Env:          opts.Env,
			User:         opts.User,
		},
		HostConfig: &container.HostConfig{
			AutoRemove: os.Getenv(qodanaCliContainerKeep) == "",
			Mounts:     volumes,
		},
	}
}

// getContainerExitCode returns the exit code of the docker container.
func getContainerExitCode(ctx context.Context, client *client.Client, id string) int64 {
	statusCh, errCh := client.ContainerWait(ctx, id, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal("container hasn't finished ", err)
		}
	case status := <-statusCh:
		return status.StatusCode
	}
	return 0
}

// runContainer runs the container.
func runContainer(ctx context.Context, client *client.Client, opts *types.ContainerCreateConfig) {
	createResp, err := client.ContainerCreate(
		ctx,
		opts.Config,
		opts.HostConfig,
		nil,
		nil,
		opts.Name,
	)
	if err != nil {
		log.Fatal("couldn't create the container ", err)
	}
	if err = client.ContainerStart(ctx, createResp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatal("couldn't bootstrap the container ", err)
	}
}

// getContainerClient returns a docker client.
func getContainerClient() *client.Client {
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal("couldn't create container client ", err)
	}
	return docker
}

// extractDockerVolumes extracts the source and target of the volume to mount.
func extractDockerVolumes(volume string) (string, string) {
	split := strings.Split(volume, ":")
	if len(split) == 2 {
		return split[0], split[1]
	} else if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		return fmt.Sprintf("%s:%s", split[0], split[1]), split[2]
	}
	return "", ""
}
