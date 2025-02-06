/*
 * Copyright 2021-2024 JetBrains s.r.o.
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
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/scan"
	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/go-connections/nat"
	"github.com/pterm/pterm"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	cliconfig "github.com/docker/cli/cli/config"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

const (
	// officialImagePrefix is the prefix of official Qodana images.
	officialImagePrefix      = "jetbrains/qodana"
	dockerSpecialCharsLength = 8
	containerJvmDebugPort    = "5005"
)

var (
	containerLogsOptions = container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	}
	containerName = "qodana-cli"
)

// runQodanaContainer runs the analysis in a Docker container from a Qodana image.
func runQodanaContainer(ctx context.Context, c scan.Context) int {
	docker := getContainerClient()
	info, err := docker.Info(ctx)
	if err != nil {
		log.Fatal("Couldn't retrieve Docker daemon information", err)
	}
	if info.OSType != "linux" {
		platform.ErrorMessage("Container engine is not running a Linux platform, other platforms are not supported by Qodana")
		return 1
	}
	fixDarwinCaches(c.CacheDir)

	scanStages := getScanStages()

	if c.SkipPull {
		checkImage(c.Linter)
	} else {
		PullImage(docker, c.Linter)
	}
	progress, _ := platform.StartQodanaSpinner(scanStages[0])

	dockerConfig := getDockerOptions(c)
	log.Debugf("docker command to run: %s", generateDebugDockerRunCommand(dockerConfig))

	platform.UpdateText(progress, scanStages[1])

	runContainer(ctx, docker, dockerConfig)
	go followLinter(docker, dockerConfig.Name, progress, scanStages)

	exitCode := getContainerExitCode(ctx, docker, dockerConfig.Name)

	fixDarwinCaches(c.CacheDir)

	if progress != nil {
		_ = progress.Stop()
	}
	checkImage(c.Linter)
	return int(exitCode)
}

// isUnofficialLinter checks if the linter is unofficial.
func isUnofficialLinter(linter string) bool {
	return !strings.HasPrefix(linter, officialImagePrefix)
}

// hasExactVersionTag checks if the linter has an exact version tag.
func hasExactVersionTag(linter string) bool {
	return strings.Contains(linter, ":") && !strings.Contains(linter, ":latest")
}

// isCompatibleLinter checks if the linter is compatible with the current CLI.
func isCompatibleLinter(linter string) bool {
	return strings.Contains(linter, platform.ReleaseVersion)
}

// checkImage checks the linter image and prints warnings if necessary.
func checkImage(linter string) {
	if strings.Contains(platform.Version, "nightly") || strings.Contains(platform.Version, "dev") {
		return
	}

	if isUnofficialLinter(linter) {
		platform.WarningMessageCI("You are using an unofficial Qodana linter: %s\n", linter)
	}

	if !hasExactVersionTag(linter) {
		platform.WarningMessageCI(
			"You are running a Qodana linter without an exact version tag: %s \n   Consider pinning the version in your configuration to ensure version compatibility: %s\n",
			linter,
			strings.Join([]string{strings.Split(linter, ":")[0], platform.ReleaseVersion}, ":"),
		)
	} else if !isCompatibleLinter(linter) {
		platform.WarningMessageCI(
			"You are using a non-compatible Qodana linter %s with the current CLI (%s) \n   Consider updating CLI or using a compatible linter %s \n",
			linter,
			platform.Version,
			strings.Join([]string{strings.Split(linter, ":")[0], platform.ReleaseVersion}, ":"),
		)
	}
}

func fixDarwinCaches(cacheDir string) {
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		err := removePortSocket(cacheDir)
		if err != nil {
			log.Warnf("Could not remove .port from %s: %s", cacheDir, err)
		}
	}
}

// removePortSocket removes .port from the system dir to resolve QD-7383.
func removePortSocket(systemDir string) error {
	ideaDir := filepath.Join(systemDir, "idea")
	files, err := os.ReadDir(ideaDir)
	if err != nil {
		return nil
	}
	for _, file := range files {
		if file.IsDir() {
			dotPort := filepath.Join(ideaDir, file.Name(), ".port")
			if _, err = os.Stat(dotPort); err == nil {
				err = os.Remove(dotPort)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// encodeAuthToBase64 serializes the auth configuration as JSON base64 payload
func encodeAuthToBase64(authConfig registry.AuthConfig) (string, error) {
	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func checkRequiredToolInstalled(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// PrepareContainerEnvSettings checks if the host is ready to run Qodana container images.
func PrepareContainerEnvSettings() {
	var tool string
	if os.Getenv(platform.QodanaCliUsePodman) == "" && checkRequiredToolInstalled("docker") {
		tool = "docker"
	} else if checkRequiredToolInstalled("podman") {
		tool = "podman"
	} else {
		platform.ErrorMessage(
			"Docker (or podman) is not installed on the system or can't be found in PATH, refer to https://www.docker.com/get-started for installing it",
		)
		os.Exit(1)
	}
	cmd := exec.Command(tool, "ps")
	if err := cmd.Run(); err != nil {
		var exiterr *exec.ExitError
		if errors.As(err, &exiterr) {
			if strings.Contains(string(exiterr.Stderr), "permission denied") {
				platform.ErrorMessage(
					"Qodana container can't be run by the current user. Please fix the container engine configuration.",
				)
				platform.WarningMessage("https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user")
				os.Exit(1)
			} else {
				platform.ErrorMessage(
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
	checkImage(image)
	platform.PrintProcess(
		func(_ *pterm.SpinnerPrinter) {
			pullImage(context.Background(), client, image)
		},
		fmt.Sprintf("Pulling the image %s", platform.PrimaryBold(image)),
		"pulling the latest version of linter",
	)
}

func isDockerUnauthorizedError(errMsg string) bool {
	errMsg = platform.Lower(errMsg)
	return strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "denied") || strings.Contains(
		errMsg,
		"forbidden",
	)
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
		encodedAuth, err := encodeAuthToBase64(registry.AuthConfig(a))
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
		containers, err := docker.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			log.Fatal("couldn't get the running containers ", err)
		}
		for _, c := range containers {
			if c.Names[0] == fmt.Sprintf("/%s", containerName) {
				err = docker.ContainerStop(context.Background(), c.Names[0], container.StopOptions{})
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
		platform.WarningMessage(
			`The container daemon is running with less than 4GB of RAM.
   If you experience issues, consider increasing the container runtime memory limit.
   Refer to %s for more information.
`,
			helpUrl,
		)
	}
}

// getDockerOptions returns qodana docker container options.
func getDockerOptions(c scan.Context) *backend.ContainerCreateConfig {
	cmdOpts := GetIdeArgs(c)

	updateScanContextEnv := func(key string, value string) { c = c.WithEnvNoOverride(key, value) }
	platform.ExtractQodanaEnvironment(updateScanContextEnv)

	cachePath, err := filepath.Abs(c.CacheDir)
	if err != nil {
		log.Fatal("couldn't get abs path for cache", err)
	}
	projectPath, err := filepath.Abs(c.ProjectDir)
	if err != nil {
		log.Fatal("couldn't get abs path for project", err)
	}
	resultsPath, err := filepath.Abs(c.ResultsDir)
	if err != nil {
		log.Fatal("couldn't get abs path for results", err)
	}
	containerName = os.Getenv(platform.QodanaCliContainerName)
	if containerName == "" {
		containerName = fmt.Sprintf("qodana-cli-%s", c.Id)
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
	for _, volume := range c.Volumes() {
		source, target := extractDockerVolumes(volume)
		if source != "" && target != "" {
			volumes = append(
				volumes, mount.Mount{
					Type:   mount.TypeBind,
					Source: source,
					Target: target,
				},
			)
		} else {
			log.Fatal("couldn't parse volume ", volume)
		}
	}
	log.Debugf("image: %s", c.Linter)
	log.Debugf("container name: %s", containerName)
	log.Debugf("user: %s", c.User)
	log.Debugf("volumes: %v", volumes)
	log.Debugf("cmd: %v", cmdOpts)

	portBindings := make(nat.PortMap)
	exposedPorts := make(nat.PortSet)

	if c.JvmDebugPort > 0 {
		log.Infof("Enabling JVM debug on port %d", c.JvmDebugPort)
		portBindings = nat.PortMap{
			containerJvmDebugPort: []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strconv.Itoa(c.JvmDebugPort),
				},
			},
		}
		exposedPorts = nat.PortSet{
			containerJvmDebugPort: struct{}{},
		}
	}
	var hostConfig *container.HostConfig
	if strings.Contains(c.Linter, "dotnet") {
		hostConfig = &container.HostConfig{
			AutoRemove:   os.Getenv(platform.QodanaCliContainerKeep) == "",
			Mounts:       volumes,
			CapAdd:       []string{"SYS_PTRACE"},
			SecurityOpt:  []string{"seccomp=unconfined"},
			PortBindings: portBindings,
		}
	} else {
		hostConfig = &container.HostConfig{
			AutoRemove:   os.Getenv(platform.QodanaCliContainerKeep) == "",
			Mounts:       volumes,
			PortBindings: portBindings,
		}
	}

	return &backend.ContainerCreateConfig{
		Name: containerName,
		Config: &container.Config{
			Image:        c.Linter,
			Cmd:          cmdOpts,
			Tty:          platform.IsInteractive(),
			AttachStdout: true,
			AttachStderr: true,
			Env:          c.Env(),
			User:         c.User,
			ExposedPorts: exposedPorts,
		},
		HostConfig: hostConfig,
	}
}

func generateDebugDockerRunCommand(cfg *backend.ContainerCreateConfig) string {
	var cmdBuilder strings.Builder
	cmdBuilder.WriteString("docker run ")
	if cfg.HostConfig != nil && cfg.HostConfig.AutoRemove {
		cmdBuilder.WriteString("--rm ")
	}
	if cfg.Config.AttachStdout {
		cmdBuilder.WriteString("-a stdout ")
	}
	if cfg.Config.AttachStderr {
		cmdBuilder.WriteString("-a stderr ")
	}
	if cfg.Config.Tty {
		cmdBuilder.WriteString("-it ")
	}
	if cfg.Config.User != "" {
		cmdBuilder.WriteString(fmt.Sprintf("-u %s ", cfg.Config.User))
	}
	for _, env := range cfg.Config.Env {
		if !strings.Contains(env, platform.QodanaToken) || strings.Contains(
			env,
			platform.QodanaLicense,
		) || strings.Contains(env, platform.QodanaLicenseOnlyToken) {
			cmdBuilder.WriteString(fmt.Sprintf("-e %s ", env))
		}
	}
	if cfg.HostConfig != nil {
		for _, m := range cfg.HostConfig.Mounts {
			cmdBuilder.WriteString(fmt.Sprintf("-v %s:%s ", m.Source, m.Target))
		}
		for _, capAdd := range cfg.HostConfig.CapAdd {
			cmdBuilder.WriteString(fmt.Sprintf("--cap-add %s ", capAdd))
		}
		for _, secOpt := range cfg.HostConfig.SecurityOpt {
			cmdBuilder.WriteString(fmt.Sprintf("--security-opt %s ", secOpt))
		}
	}
	cmdBuilder.WriteString(cfg.Config.Image + " ")
	for _, arg := range cfg.Config.Cmd {
		cmdBuilder.WriteString(fmt.Sprintf("%s ", arg))
	}

	return cmdBuilder.String()
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
func runContainer(ctx context.Context, client *client.Client, opts *backend.ContainerCreateConfig) {
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
	if err = client.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
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
