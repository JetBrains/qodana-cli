package pkg

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const QodanaSuccessExitCode = 0
const QodanaFailThresholdExitCode = 255
const OfficialDockerPrefix = "jetbrains/qodana"

// ensureDockerInstalled checks if docker is installed
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
			Error.Println(
				"Docker is not installed on your system, ",
				"refer to https://www.docker.com/get-started for installing it",
			)
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// EnsureDockerRunning checks if docker daemon is running
func EnsureDockerRunning() {
	ensureDockerInstalled()
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			Error.Println(fmt.Sprintf(
				"Docker exited with exit code %d, perhaps docker daemon is not running?",
				exiterr.ExitCode(),
			))
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// getCmdOptions returns qodana command options
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

// getDockerOptions returns qodana docker container options
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

	return &types.ContainerCreateConfig{
		Name: "qodana-cli",
		Config: &container.Config{
			Image:        opts.Linter,
			Cmd:          getCmdOptions(opts),
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			Env:          opts.EnvVariables,
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

// RemoveContainer removes the container
func tryRemoveContainer(ctx context.Context, client *client.Client, name string) {
	_ = client.ContainerRemove(ctx, name, types.ContainerRemoveOptions{Force: true})
}

// PullImage pulls docker image
func PullImage(ctx context.Context, client *client.Client, image string) {
	reader, err := client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return
	}
	defer func(pull io.ReadCloser) {
		err := pull.Close()
		if err != nil {
			log.Fatal("can't pull image", err)
		}
	}(reader)
	if _, err = io.Copy(ioutil.Discard, reader); err != nil {
		log.Fatal("couldn't read the image pull logs", err)
	}
}

func waitContainerExited(ctx context.Context, client *client.Client, id string) int64 {
	statusCh, errCh := client.ContainerWait(ctx, id, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal("container hasn't finished", err)
		}
	case status := <-statusCh:
		return status.StatusCode
	}
	return 0
}

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
		log.Fatal("couldn't create the container", err)
	}
	if err = client.ContainerStart(ctx, createResp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatal("couldn't bootstrap the container", err)
	}
}

// RunLinter runs the linter container and waits until it's finished
func RunLinter(ctx context.Context, client *client.Client, opts *QodanaOptions) int64 {
	dockerOpts := getDockerOptions(opts)
	tryRemoveContainer(ctx, client, dockerOpts.Name)
	runContainer(ctx, client, dockerOpts)
	exitCode := waitContainerExited(ctx, client, dockerOpts.Name)
	tryRemoveContainer(ctx, client, dockerOpts.Name)
	return exitCode
}
