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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	qodanaEnv       = "QODANA_ENV"
	qodanaToken     = "QODANA_TOKEN"
	qodanaJobUrl    = "QODANA_JOB_URL"
	qodanaRemoteUrl = "QODANA_REMOTE_URL"
	qodanaBranch    = "QODANA_BRANCH"
	qodanaRevision  = "QODANA_REVISION"
)

// extractQodanaEnvironment extracts Qodana env variables QODANA_* to the given environment array.
func extractQodanaEnvironment(opts *QodanaOptions) {
	opts.setEnv(qodanaToken, os.Getenv(qodanaToken))
	ci := cienvironment.DetectCIEnvironment()
	qEnv := "cli"
	if ci != nil {
		qEnv = strings.ReplaceAll(strings.ToLower(ci.Name), " ", "-")
		opts.setEnv(qodanaJobUrl, ci.URL)
		if ci.Git != nil {
			opts.setEnv(qodanaRemoteUrl, ci.Git.Remote)
			opts.setEnv(qodanaBranch, ci.Git.Branch)
			opts.setEnv(qodanaRevision, ci.Git.Revision)
		}
	}
	opts.setEnv(qodanaEnv, fmt.Sprintf("%s:%s", qEnv, Version))
}

func checkRequiredToolInstalled(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// CheckContainerHost checks if the host is ready to run Qodana container images.
func CheckContainerHost() {
	var tool string
	if checkRequiredToolInstalled("docker") {
		tool = "docker"
	} else if checkRequiredToolInstalled("podman") {
		tool = "podman"
	} else {
		ErrorMessage(
			"Docker is not installed on your system or can't be found in PATH, refer to https://www.docker.com/get-started for installing it",
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

// PullImage pulls docker image.
func PullImage(ctx context.Context, client *client.Client, image string) {
	reader, err := client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return
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
		helpUrl = "https://docs.docker.com/docker-for-windows/about/"
	case "darwin":
		helpUrl = "https://docs.docker.com/docker-for-mac/about/"
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
	if opts.Changes || (opts.GitReset && opts.Commit != "") {
		arguments = append(arguments, "--changes")
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
	extractQodanaEnvironment(opts)
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
	containerName = fmt.Sprintf("qodana-cli-%s", getProjectId(projectPath))
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
		volumes = append(volumes, mount.Mount{
			Type:   mount.TypeBind,
			Source: strings.Split(volume, ":")[0],
			Target: strings.Split(volume, ":")[1],
		})
	}
	log.Debugf("image: %s", opts.Linter)
	log.Debugf("container name: %s", containerName)
	log.Debugf("user: %s", opts.User)
	log.Debugf("env: %v", opts.Env)
	log.Debugf("volumes: %v", volumes)
	log.Debugf("cmd: %v", cmdOpts)
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
			AutoRemove: true,
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
