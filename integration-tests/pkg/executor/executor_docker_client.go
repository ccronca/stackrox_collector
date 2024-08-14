package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/stackrox/collector/integration-tests/pkg/common"
	"github.com/stackrox/collector/integration-tests/pkg/log"
)

type DockerExecutor struct {
	client *client.Client
}

func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerExecutor{client: cli}, nil
}

// Options needed
// stats container
//  Mount docker socket -> args := []string{"-v", common.RuntimeSocket + ":/var/run/docker.sock", image}
// benchmark container
//  Env Var -> "--env", "FORCE_TIMES_TO_RUN=1",
//  Command
// collector container
//  --privileged
//  --network=host
//  --env vars
//  mounts

func convertMapToSlice(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

func (d *DockerExecutor) IsContainerFoundFiltered(containerID, filter string) (bool, error) {
	ctx := context.Background()
	defer d.client.Close()

	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("filter format is invalid")
	}
	filterKey, filterValue := parts[0], parts[1]

	filterArgs := filters.NewArgs()
	filterArgs.Add(filterKey, filterValue)

	if containerID != "" {
		filterArgs.Add("id", containerID)
	}

	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		Filters: filterArgs,
		All:     true,
	})
	if err != nil {
		return false, err
	}

	found := false
	for _, c := range containers {
		if common.ContainerShortID(containerID) == common.ContainerShortID(c.ID) {
			found = true
		}
	}
	log.Info("[dockerclient] checked %s for filter %s (%t)\n", common.ContainerShortID(containerID), filter, found)

	return found, nil
}

func (d *DockerExecutor) GetContainerHealthCheck(containerID string) (string, error) {
	ctx := context.Background()
	defer d.client.Close()

	container, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("error inspecting container: %w", err)
	}

	if container.Config.Healthcheck == nil {
		return "", fmt.Errorf("container %s does not have a health check", containerID)
	}

	log.Info("[dockerclient] %s has healthcheck: %s\n", containerID, strings.Join(container.Config.Healthcheck.Test, " "))
	return strings.Join(container.Config.Healthcheck.Test, " "), nil
}

func (d *DockerExecutor) StartContainer(startConfig ContainerStartConfig) (string, error) {
	ctx := context.Background()
	defer d.client.Close()

	binds := []string{}
	volumes := map[string]struct{}{}

	for containerPath, hostPath := range startConfig.Mounts {
		if hostPath == "" {
			volumes[containerPath] = struct{}{}
		} else {
			bind := fmt.Sprintf("%s:%s", hostPath, containerPath)
			binds = append(binds, bind)
			volumes[containerPath] = struct{}{}
		}
	}

	containerConfig := &container.Config{
		Image:      startConfig.Image,
		Env:        convertMapToSlice(startConfig.Env),
		Cmd:        startConfig.Command,
		Entrypoint: startConfig.EntryPoint,
		Volumes:    volumes,
	}

	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(startConfig.NetworkMode),
		Privileged:  startConfig.Privileged,
		Binds:       binds,
	}

	resp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, startConfig.Name)
	if err != nil {
		return "", err
	}

	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}

	log.Info("[dockerclient] container start %s with %s (%s)\n",
		startConfig.Name, startConfig.Image, common.ContainerShortID(resp.ID))
	return resp.ID, nil
}

func (d *DockerExecutor) ExecContainer(containerName string, command []string) (string, error) {
	ctx := context.Background()
	defer d.client.Close()
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          command,
	}

	resp, err := d.client.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", fmt.Errorf("error creating exec: %w", err)
	}

	execStartCheck := types.ExecStartCheck{Detach: false, Tty: false}
	attachResp, err := d.client.ContainerExecAttach(ctx, resp.ID, execStartCheck)
	if err != nil {
		return "", fmt.Errorf("error attaching to exec: %w", err)
	}
	defer attachResp.Close()

	var outBuf bytes.Buffer
	_, err = io.Copy(&outBuf, attachResp.Reader)
	if err != nil {
		return "", fmt.Errorf("error reading exec output: %w", err)
	}

	execInspect, err := d.client.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return "", fmt.Errorf("error inspecting exec: %w", err)
	}

	log.Info("[dockerclient] container exec %s %v (exitCode=%d, outBytes=%d)\n",
		containerName, command, execInspect.ExitCode, outBuf.Len())
	return outBuf.String(), nil
}

func (d *DockerExecutor) GetContainerLogs(containerID string) (string, error) {
	ctx := context.Background()
	defer d.client.Close()
	options := container.LogsOptions{ShowStdout: true, ShowStderr: true}
	logsReader, err := d.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("error getting container logs: %w", err)
	}
	defer logsReader.Close()

	// Demultiplex stderr/stdout
	var sbStdOut, sbStdErr strings.Builder
	if _, err := stdcopy.StdCopy(&sbStdOut, &sbStdErr, logsReader); err != nil {
		return "", fmt.Errorf("error copying logs: %w", err)
	}

	log.Info("[dockerclient] logs %s (%d bytes)\n", containerID, sbStdOut.Len())
	return sbStdOut.String(), nil
}

func (d *DockerExecutor) KillContainer(containerID string) error {
	ctx := context.Background()
	defer d.client.Close()

	timeoutContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := d.client.ContainerKill(timeoutContext, containerID, "KILL")
	if err != nil {
		return fmt.Errorf("error killing container: %w", err)
	}

	log.Info("[dockerclient] kill %s\n", containerID)
	return nil
}

func (d *DockerExecutor) StopContainer(containerID string) error {
	ctx := context.Background()
	defer d.client.Close()

	stopOptions := container.StopOptions{
		Signal:  "SIGTERM",
		Timeout: nil, // 10 secs
	}
	timeoutContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := d.client.ContainerStop(timeoutContext, containerID, stopOptions)
	if err != nil {
		return fmt.Errorf("error stopping container: %w", err)
	}
	log.Info("[dockerclient] stop %s\n", containerID)
	return nil
}

func (d *DockerExecutor) RemoveContainer(containerID string) error {
	ctx := context.Background()
	defer d.client.Close()

	removeOptions := container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}
	timeoutDuration := 10 * time.Second
	timeoutContext, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()
	err := d.client.ContainerRemove(timeoutContext, containerID, removeOptions)
	if err != nil {
		return fmt.Errorf("error removing container: %w", err)
	}

	log.Info("[dockerclient] remove %s\n", containerID)
	return nil
}

func (d *DockerExecutor) inspectContainer(containerID string) (types.ContainerJSON, error) {
	ctx := context.Background()
	defer d.client.Close()
	containerJSON, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("error inspecting container: %w", err)
	}
	log.Debug("[dockerclient] inspect %s\n", containerID)
	return containerJSON, nil
}

func (d *DockerExecutor) GetContainerExitCode(containerID string) (int, error) {
	container, err := d.inspectContainer(containerID)
	if err != nil {
		return -1, err
	}
	log.Info("[dockerclient] %s exitcode=%s\n", containerID, container.State.ExitCode)
	return container.State.ExitCode, nil
}

func (d *DockerExecutor) GetContainerIP(containerID string) (string, error) {
	container, err := d.inspectContainer(containerID)
	if err != nil {
		return "", err
	}
	log.Info("[dockerclient] container IP for %s is %s\n", containerID,
		container.NetworkSettings.DefaultNetworkSettings.IPAddress)
	return container.NetworkSettings.DefaultNetworkSettings.IPAddress, nil
}

func (d *DockerExecutor) GetContainerPort(containerID string) (string, error) {
	containerJSON, err := d.inspectContainer(containerID)
	if err != nil {
		return "-1", err
	}

	containerPort := "-1"
	if len(containerJSON.NetworkSettings.Ports) > 0 {
		for portStr := range containerJSON.NetworkSettings.Ports {
			portSplit := strings.Split(string(portStr), "/")
			if len(portSplit) > 0 {
				containerPort = portSplit[0]
			}
		}
	}

	log.Info("[dockerclient] container port for %s is %s\n", containerID, containerPort)
	return containerPort, nil
}

func (d *DockerExecutor) CheckContainerHealthy(containerID string) (bool, error) {
	containerJSON, err := d.inspectContainer(containerID)
	if err != nil {
		return false, err
	}
	log.Info("[dockerclient] container %s is %v\n", containerID, containerJSON.State.Health.Status)
	return containerJSON.State.Health.Status == "healthy", nil
}

func (d *DockerExecutor) CheckContainerRunning(containerID string) (bool, error) {
	containerJSON, err := d.inspectContainer(containerID)
	if err != nil {
		return false, err
	}
	log.Info("[dockerclient] %s running: %v\n", containerID, containerJSON.State.Running)
	return containerJSON.State.Running, nil
}

func (d *DockerExecutor) CheckContainerExists(containerID string) (bool, error) {
	_, err := d.inspectContainer(containerID)
	if err != nil {
		log.Info("[dockerclient] %s does not exist\n", containerID)
		return false, err
	}

	log.Info("[dockerclient] %s exists\n", containerID)
	return true, nil
}

func (d *DockerExecutor) CheckImageExists(imageName string) (bool, error) {
	ctx := context.Background()
	defer d.client.Close()

	_, _, err := d.client.ImageInspectWithRaw(ctx, imageName)
	log.Info("[dockerclient] image %s exists (%t)\n", imageName, err == nil)
	if err == nil {
		return true, nil
	} else if client.IsErrNotFound(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("error inspecting image: %w", err)
	}
}
