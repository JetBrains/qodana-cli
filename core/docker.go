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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

const (
	QodanaSuccessExitCode       = 0
	QodanaFailThresholdExitCode = 255
	OfficialDockerPrefix        = "jetbrains/qodana"
)

var (
	unofficialLinter    = false
	notSupportedLinters = []string{
		"jetbrains/qodana-license-audit",
		"jetbrains/qodana-clone-finder",
	}
)

// ensureDockerInstalled checks if docker is installed.
func ensureDockerInstalled() {
	var what string
	if runtime.GOOS == "windows" {
		what = "where"
	} else {
		what = "which"
	}
	cmd := exec.Command(what, "docker")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			ErrorMessage(
				"Docker is not installed on your system, refer to https://www.docker.com/get-started for installing it",
			)
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// EnsureDockerRunning checks if docker daemon is running.
func EnsureDockerRunning() { // TODO: check if /var/run/docker.sock is a Unix domain socket (Linux images are supported)
	ensureDockerInstalled()
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			ErrorMessage(fmt.Sprintf(
				"Docker exited with exit code %d, perhaps docker daemon is not running?",
				exiterr.ExitCode(),
			))
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// getCmdOptions returns qodana command options.
func getCmdOptions(opts *QodanaOptions) []string {
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
	if opts.RunPromo {
		arguments = append(arguments, "--run-promo")
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
	if opts.Property != "" {
		arguments = append(arguments, "--property", opts.Property)
	}
	if opts.FailThreshold != "" {
		arguments = append(arguments, "--fail-threshold", opts.FailThreshold)
	}
	if opts.Changes {
		arguments = append(arguments, "--changes")
	}
	if opts.SendReport {
		arguments = append(arguments, "--send-report")
	}
	if opts.Token != "" {
		arguments = append(arguments, "--token", opts.Token)
	}
	if opts.AnalysisId != "" {
		arguments = append(arguments, "--analysis-id", opts.AnalysisId)
	}
	if DoNotTrack {
		arguments = append(arguments, "--property=idea.headless.enable.statistics=false")
	}
	return arguments
}

// getDockerOptions returns qodana docker container options.
func getDockerOptions(opts *QodanaOptions) *types.ContainerCreateConfig {
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

	// TODO: agree on the memory constraints, set the memory and disable OOM killer
	// https://docs.docker.com/config/containers/resource_constraints/#limit-a-containers-access-to-memory
	// or at least drop a warning when Docker is low on RAM

	return &types.ContainerCreateConfig{
		Name: "qodana-cli",
		Config: &container.Config{
			Image:        opts.Linter,
			Cmd:          getCmdOptions(opts),
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			Env:          append(opts.EnvVariables, "QODANA_ENV=cli"),
			User:         fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
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
			},
		},
	}
}

// tryRemoveContainer removes the container.
func tryRemoveContainer(ctx context.Context, client *client.Client, name string) {
	_ = client.ContainerRemove(ctx, name, types.ContainerRemoveOptions{Force: true})
}

// pullImage pulls docker image
func pullImage(ctx context.Context, client *client.Client, image string) {
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
	if _, err = io.Copy(ioutil.Discard, reader); err != nil {
		log.Fatal("couldn't read the image pull logs ", err)
	}
}

// getDockerExitCode returns the exit code of the docker container
func getDockerExitCode(ctx context.Context, client *client.Client, id string) int64 {
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

// runContainer runs the container
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

// stopContainer stops the container
func stopContainer(ctx context.Context, client *client.Client, id string) {
	_ = client.ContainerStop(ctx, id, nil)
}

// DockerCleanup cleans up Qodana containers
func DockerCleanup() {
	docker, _ := client.NewClientWithOpts()
	stopContainer(context.Background(), docker, "qodana-cli")
	tryRemoveContainer(context.Background(), docker, "qodana-cli")
}
