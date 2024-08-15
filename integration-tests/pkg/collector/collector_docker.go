package collector

import (
	"encoding/json"
	"strings"

	"golang.org/x/exp/maps"

	"github.com/hashicorp/go-multierror"
	"github.com/stackrox/collector/integration-tests/pkg/common"
	"github.com/stackrox/collector/integration-tests/pkg/config"
	"github.com/stackrox/collector/integration-tests/pkg/executor"
	"github.com/stackrox/collector/integration-tests/pkg/log"
)

type DockerCollectorManager struct {
	executor      executor.Executor
	mounts        map[string]string
	env           map[string]string
	config        map[string]any
	bootstrapOnly bool
	testName      string

	CollectorOutput string
	containerID     string
}

func NewDockerCollectorManager(e executor.Executor, name string) *DockerCollectorManager {
	collectorOptions := config.CollectorInfo()

	collectionMethod := config.CollectionMethod()

	collectorConfig := map[string]any{
		"logLevel":       collectorOptions.LogLevel,
		"turnOffScrape":  true,
		"scrapeInterval": 2,
	}

	env := map[string]string{
		"GRPC_SERVER":             "localhost:9999",
		"COLLECTION_METHOD":       collectionMethod,
		"COLLECTOR_PRE_ARGUMENTS": collectorOptions.PreArguments,
		"ENABLE_CORE_DUMP":        "true",
	}

	mounts := map[string]string{
		"/host/proc:ro":             "/proc",
		"/host/etc:ro":              "/etc",
		"/host/usr/lib:ro":          "/usr/lib",
		"/host/sys/kernel/debug:ro": "/sys/kernel/debug",
	}

	return &DockerCollectorManager{
		executor:      e,
		bootstrapOnly: false,
		env:           env,
		mounts:        mounts,
		config:        collectorConfig,
		testName:      name,
	}
}

func (c *DockerCollectorManager) Setup(options *StartupOptions) error {
	if options == nil {
		// default to empty, if no options are provided (i.e. use the
		// default values)
		options = &StartupOptions{}
	}

	if options.Env != nil {
		maps.Copy(c.env, options.Env)
	}

	if options.Mounts != nil {
		maps.Copy(c.mounts, options.Mounts)
	}

	if options.Config != nil {
		maps.Copy(c.config, options.Config)
	}

	return c.executor.PullImage(config.Images().CollectorImage())
}

func (c *DockerCollectorManager) Launch() error {
	return c.launchCollector()
}

func (c *DockerCollectorManager) TearDown() error {
	isRunning, err := c.IsRunning()
	if err != nil {
		return log.Error("Unable to check if container is running: %s", err)
	}

	if !isRunning {
		c.captureLogs("collector")
		// Check if collector container segfaulted or exited with error
		exitCode, err := c.executor.ExitCode(executor.ContainerFilter{
			Name: "collector",
		})
		if err != nil {
			return log.Error("Failed to get container exit code: %s", err)
		}
		if exitCode != 0 {
			return log.Error("Collector container has non-zero exit code (%d)", exitCode)
		}
	} else {
		c.stopContainer("collector")
		c.captureLogs("collector")
		c.killContainer("collector")
	}

	return nil
}

func (c *DockerCollectorManager) IsRunning() (bool, error) {
	return c.executor.IsContainerRunning("collector")
}

func (c *DockerCollectorManager) createCollectorStartConfig() (executor.ContainerStartConfig, error) {
	startConfig := executor.ContainerStartConfig{
		Name:        "collector",
		Image:       config.Images().CollectorImage(),
		Privileged:  true,
		NetworkMode: "host",
		Mounts:      c.mounts,
		Env:         c.env,
	}

	configJson, err := json.Marshal(c.config)
	if err != nil {
		return executor.ContainerStartConfig{}, err
	}
	startConfig.Env["COLLECTOR_CONFIG"] = string(configJson)

	if c.bootstrapOnly {
		startConfig.Command = []string{"exit", "0"}
	}
	return startConfig, nil
}

func (c *DockerCollectorManager) launchCollector() error {
	startConfig, err := c.createCollectorStartConfig()
	if err != nil {
		return err
	}
	output, err := c.executor.StartContainer(startConfig)
	c.CollectorOutput = output
	if err != nil {
		return err
	}
	outLines := strings.Split(output, "\n")
	c.containerID = common.ContainerShortID(string(outLines[len(outLines)-1]))
	return err
}

func (c *DockerCollectorManager) captureLogs(containerName string) (string, error) {
	logs, err := c.executor.GetContainerLogs(containerName)
	if err != nil {
		return "", log.Error(executor.RuntimeCommand+" logs error (%v) for container %s\n", err, containerName)
	}

	logFile, err := common.PrepareLog(c.testName, "collector.log")
	if err != nil {
		return "", err
	}
	defer logFile.Close()

	_, err = logFile.WriteString(logs)
	return logs, nil
}

func (c *DockerCollectorManager) killContainer(name string) error {
	_, err1 := c.executor.KillContainer(name)
	_, err2 := c.executor.RemoveContainer(executor.ContainerFilter{
		Name: name,
	})

	var result error
	if err1 != nil {
		result = multierror.Append(result, err1)
	}
	if err2 != nil {
		result = multierror.Append(result, err2)
	}

	return result
}

func (c *DockerCollectorManager) stopContainer(name string) error {
	_, err := c.executor.StopContainer(name)
	return err
}

func (c *DockerCollectorManager) ContainerID() string {
	return c.containerID
}

func (c *DockerCollectorManager) TestName() string {
	return c.testName
}
