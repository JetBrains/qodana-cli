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
	"os"
	"os/exec"
	"path/filepath"
)

// ensureDockerInstalled checks if docker is installed
func ensureDockerInstalled() {
	cmd := exec.Command("docker", "--version")
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
//goland:noinspection GoUnusedParameter
func getCmdOptions(opts *LinterOptions) []string {
	arguments := make([]string, 0)
	arguments = append(arguments, "--save-report")
	return arguments
}

// getDockerOptions returns qodana docker container options
func getDockerOptions(opts *LinterOptions, linter string) *types.ContainerCreateConfig {
	cachePath, err := filepath.Abs(opts.CachePath)
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
			Image:        linter,
			Cmd:          getCmdOptions(opts),
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
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

// pullImage pulls docker image
func pullImage(ctx context.Context, client *client.Client, image string) {
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
}

func waitContainerExited(ctx context.Context, client *client.Client, id string) {
	statusCh, errCh := client.ContainerWait(ctx, id, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal("container hasn't finished", err)
		}
	case <-statusCh:
	}
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

// PullImage pulls qodana image and returns image with version
func PullImage(ctx context.Context, client *client.Client, image string) string {
	pullImage(ctx, client, image)
	// TODO: Parse version from tags when ready
	return fmt.Sprintf("%s:%s", image, "latest")
}

// RunLinter runs the linter container and waits until it's finished
func RunLinter(ctx context.Context, client *client.Client, opts *LinterOptions, image string) {
	dockerOpts := getDockerOptions(opts, image)
	tryRemoveContainer(ctx, client, dockerOpts.Name)
	runContainer(ctx, client, dockerOpts)
	waitContainerExited(ctx, client, dockerOpts.Name)
	tryRemoveContainer(ctx, client, dockerOpts.Name)
}
