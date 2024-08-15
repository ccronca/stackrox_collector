package executor

import (
	"os/exec"

	"github.com/stackrox/collector/integration-tests/pkg/config"
)

type ContainerFilter struct {
	Name      string
	Namespace string
}

type Executor interface {
	ContainerExists(filter ContainerFilter) (bool, error)
	IsContainerRunning(container string) (bool, error)
	ExecContainer(containerName string, command []string) (string, error)
	ExitCode(filter ContainerFilter) (int, error)
	GetContainerHealthCheck(containerID string) (string, error)
	GetContainerIP(containerID string) (string, error)
	GetContainerLogs(containerID string) (string, error)
	GetContainerPort(containerID string) (string, error)
	IsContainerFoundFiltered(containerID, filter string) (bool, error)
	KillContainer(name string) (string, error)
	PullImage(image string) error
	RemoveContainer(filter ContainerFilter) error
	StartContainer(config ContainerStartConfig) (string, error)
	StopContainer(name string) error
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
	if config.ContainerExecutor() == config.DockerAPIContainerExecutor {
		return newDockerAPIExecutor()
	}
	return newDockerExecutor()
}
