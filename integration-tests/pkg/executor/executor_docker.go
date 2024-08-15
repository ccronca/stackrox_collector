package executor

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/stackrox/collector/integration-tests/pkg/common"
	"github.com/stackrox/collector/integration-tests/pkg/config"
	"github.com/stackrox/collector/integration-tests/pkg/log"
)

var (
	RuntimeCommand = config.RuntimeInfo().Command
	RuntimeSocket  = config.RuntimeInfo().Socket
	RuntimeAsRoot  = config.RuntimeInfo().RunAsRoot
)

type containerProcessExecutor struct {
	builder CommandBuilder
}

type localCommandBuilder struct{}

func (e *localCommandBuilder) ExecCommand(execArgs ...string) *exec.Cmd {
	return exec.Command(execArgs[0], execArgs[1:]...)
}

func (e *localCommandBuilder) RemoteCopyCommand(remoteSrc string, localDst string) *exec.Cmd {
	if remoteSrc != localDst {
		return exec.Command("cp", remoteSrc, localDst)
	}
	return nil
}

func newLocalCommandBuilder() CommandBuilder {
	return &localCommandBuilder{}
}

func newContainerProcessExecutor() (*containerProcessExecutor, error) {
	// While this function can't fail, to conform to the
	// same construction API as other executors, we keep the
	// error return value.

	return &containerProcessExecutor{
		builder: newLocalCommandBuilder(),
	}, nil
}

// exec executes the provided command with retries on non-zero error from the command.
func (e *containerProcessExecutor) exec(args ...string) (string, error) {
	if args[0] == RuntimeCommand && RuntimeAsRoot {
		args = append([]string{"sudo"}, args...)
	}
	return Retry(func() (string, error) {
		return e.RunCommand(e.builder.ExecCommand(args...))
	})
}

// execWithErrorCheck executes the provided command, retrying if an error occurs
// and the command's output does not contain any of the accepted output contents.
func (e *containerProcessExecutor) execWithErrorCheck(errCheckFn func(string, error) error, args ...string) (string, error) {
	if args[0] == RuntimeCommand && RuntimeAsRoot {
		args = append([]string{"sudo"}, args...)
	}
	return RetryWithErrorCheck(errCheckFn, func() (string, error) {
		return e.RunCommand(e.builder.ExecCommand(args...))
	})
}

// execWithoutRetry executes provided command once, without retries.
func (e *containerProcessExecutor) execWithoutRetry(args ...string) (string, error) {
	if args[0] == RuntimeCommand && RuntimeAsRoot {
		args = append([]string{"sudo"}, args...)
	}
	return e.RunCommand(e.builder.ExecCommand(args...))
}

func (e *containerProcessExecutor) RunCommand(cmd *exec.Cmd) (string, error) {
	if cmd == nil {
		return "", nil
	}
	commandLine := strings.Join(cmd.Args, " ")
	log.Info("%s\n", commandLine)
	stdoutStderr, err := cmd.CombinedOutput()
	trimmed := strings.Trim(string(stdoutStderr), "\"\n")
	log.Debug("Run Output: %s\n", trimmed)
	if err != nil {
		err = errors.Wrapf(err, "Command Failed: %s\nOutput: %s\n", commandLine, trimmed)
	}
	return trimmed, err
}

func (e *containerProcessExecutor) PullImage(image string) error {
	_, err := e.execWithoutRetry(RuntimeCommand, "image", "inspect", image)
	if err == nil {
		return nil
	}
	_, err = e.exec(RuntimeCommand, "pull", image)
	return err
}

func (e *containerProcessExecutor) CheckContainerRunning(containerID string) (bool, error) {
	result, err := e.execWithoutRetry(RuntimeCommand, "inspect", containerID, "--format='{{.State.Running}}'")
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(strings.Trim(result, "\"'"))
}

func (e *containerProcessExecutor) CheckContainerExists(cf ContainerFilter) (bool, error) {
	_, err := e.execWithoutRetry(RuntimeCommand, "inspect", cf.Name)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (e *containerProcessExecutor) GetContainerExitCode(cf ContainerFilter) (int, error) {
	result, err := e.exec(RuntimeCommand, "inspect", cf.Name, "--format='{{.State.GetContainerExitCode}}'")
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(strings.Trim(result, "\"'"))
}

// checkContainerCommandError returns nil if the output of the container
// command indicates retries are not needed.
func checkContainerCommandError(name string, cmd string, output string, err error) error {
	for _, str := range []string{
		"no such container",
		"cannot " + cmd + " container",
		"can only " + cmd + " running containers",
	} {
		if strings.Contains(strings.ToLower(output), strings.ToLower(str)) {
			return nil
		}
	}
	return err
}

func containerErrorCheckFunction(name string, cmd string) func(string, error) error {
	return func(stdout string, err error) error {
		return checkContainerCommandError(name, cmd, stdout, err)
	}
}

// KillContainer runs the kill operation on the provided container name
func (e *containerProcessExecutor) KillContainer(name string) (string, error) {
	return e.execWithErrorCheck(containerErrorCheckFunction(name, "kill"), RuntimeCommand, "kill", name)
}

// RemoveContainer runs the remove operation on the provided container name
func (e *containerProcessExecutor) RemoveContainer(cf ContainerFilter) error {
	_, err := e.execWithErrorCheck(containerErrorCheckFunction(cf.Name, "remove"), RuntimeCommand, "rm", cf.Name)
	return err
}

// StopContainer runs the stop operation on the provided container name
func (e *containerProcessExecutor) StopContainer(name string) error {
	_, err := e.execWithErrorCheck(containerErrorCheckFunction(name, "stop"), RuntimeCommand, "stop", name)
	return err
}

func (e *containerProcessExecutor) ExecContainer(containerName string, command []string) (string, error) {
	cmd := []string{RuntimeCommand, "exec", containerName}
	cmd = append(cmd, command...)
	return e.exec(cmd...)
}

func (e *containerProcessExecutor) StartContainer(startConfig ContainerStartConfig) (string, error) {
	var cmd []string
	cmd = append(cmd, RuntimeCommand, "run", "-d")

	if startConfig.Name != "" {
		cmd = append(cmd, "--name", startConfig.Name)
	}
	if startConfig.Privileged {
		cmd = append(cmd, "--privileged")
	}
	if startConfig.NetworkMode != "" {
		cmd = append(cmd, "--network", startConfig.NetworkMode)
	}

	for target, source := range startConfig.Mounts {
		var mountFlag = ""
		if source == "" {
			mountFlag = fmt.Sprintf("%s", target)
		} else {
			mountFlag = fmt.Sprintf("%s:%s", source, target)
		}
		cmd = append(cmd, "-v", mountFlag)
	}

	for k, v := range startConfig.Env {
		envFlag := fmt.Sprintf("%s=%s", k, v)
		cmd = append(cmd, "-e", envFlag)
	}

	for _, ep := range startConfig.Entrypoint {
		cmd = append(cmd, "--entrypoint", ep)
	}

	cmd = append(cmd, startConfig.Image)
	cmd = append(cmd, startConfig.Command...)

	output, err := e.exec(cmd...)
	if err != nil {
		return "", fmt.Errorf("error running docker run: %s\nOutput: %s", err, output)
	}

	containerID := strings.TrimSpace(string(output))

	return containerID, nil
}

func (e *containerProcessExecutor) GetContainerLogs(containerName string) (string, error) {
	logs, err := e.exec(RuntimeCommand, "logs", containerName)
	if err != nil {
		log.Error("error getting logs for container %s: %v\n", containerName, err)
		return "", err
	}
	return logs, nil
}

func (e *containerProcessExecutor) IsContainerFoundFiltered(containerID, filter string) (bool, error) {
	cmd := []string{
		RuntimeCommand, "ps", "-qa",
		"--filter", "id=" + containerID,
		"--filter", filter,
	}

	output, err := e.exec(cmd...)
	if err != nil {
		return false, err
	}
	outLines := strings.Split(output, "\n")
	lastLine := outLines[len(outLines)-1]
	if lastLine == common.ContainerShortID(containerID) {
		return true, nil
	}

	return false, nil
}

func (e *containerProcessExecutor) GetContainerHealthCheck(containerName string) (string, error) {
	cmd := []string{RuntimeCommand, "inspect", "-f", "'{{ .Config.Healthcheck }}'", containerName}
	output, err := e.exec(cmd...)
	if err != nil {
		return "", err
	}

	outLines := strings.Split(output, "\n")
	lastLine := outLines[len(outLines)-1]

	// Clearly no HealthCheck section
	if lastLine == "<nil>" {
		return "", nil
	}
	return lastLine, nil
}

func (e *containerProcessExecutor) GetContainerIP(containerName string) (string, error) {
	stdoutStderr, err := e.exec(RuntimeCommand, "inspect", "--format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'", containerName)
	return strings.Replace(string(stdoutStderr), "'", "", -1), err
}

func (e *containerProcessExecutor) GetContainerPort(containerName string) (string, error) {
	stdoutStderr, err := e.exec(RuntimeCommand, "inspect", "--format='{{json .NetworkSettings.Ports}}'", containerName)
	if err != nil {
		return "", err
	}
	rawString := strings.Trim(string(stdoutStderr), "'\n")
	var portMap map[string]interface{}
	err = json.Unmarshal([]byte(rawString), &portMap)
	if err != nil {
		return "", err
	}

	for k := range portMap {
		return strings.Split(k, "/")[0], nil
	}

	return "", fmt.Errorf("no port mapping found: %v %v", rawString, portMap)
}
