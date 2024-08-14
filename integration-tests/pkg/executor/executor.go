package executor

import (
	"os/exec"
)

type ContainerFilter struct {
	Name      string
	Namespace string
}

type Executor interface {
	CopyFromHost(src string, dst string) (string, error)
	PullImage(image string) error
	IsContainerRunning(container string) (bool, error)
	ContainerExists(filter ContainerFilter) (bool, error)
	ContainerID(filter ContainerFilter) string
	ExitCode(filter ContainerFilter) (int, error)
	Exec(args ...string) (string, error)
	ExecWithErrorCheck(errCheckFn func(string, error) error, args ...string) (string, error)
	ExecWithStdin(pipedContent string, args ...string) (string, error)
	ExecWithoutRetry(args ...string) (string, error)
	KillContainer(name string) (string, error)
	RemoveContainer(filter ContainerFilter) (string, error)
	StopContainer(name string) (string, error)
	ExecContainer(containerName string, command []string) (string, error)
	GetContainerPort(containerID string) (string, error)
	GetContainerIP(containerID string) (string, error)
	GetContainerLogs(containerID string) (string, error)
	StartContainer(config ContainerStartConfig) (string, error)
	GetContainerHealthCheck(containerID string) (string, error)
	IsContainerFoundFiltered(containerID, filter string) (bool, error)
}

type ContainerStartConfig struct {
	Name        string
	Image       string
	Privileged  bool
	NetworkMode string
	Mounts      map[string]string
	Env         map[string]string
	Command     []string
	Entrypoint  []string
}

type CommandBuilder interface {
	ExecCommand(args ...string) *exec.Cmd
	RemoteCopyCommand(remoteSrc string, localDst string) *exec.Cmd
}

func New() (Executor, error) {
	return newDockerExecutor()
}
