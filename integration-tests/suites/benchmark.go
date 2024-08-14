package suites

import (
	"fmt"
	"os"
	"time"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stackrox/collector/integration-tests/pkg/config"
	"github.com/stackrox/collector/integration-tests/pkg/executor"
)

type BenchmarkBaselineTestSuite struct {
	BenchmarkTestSuiteBase
}

type BenchmarkCollectorTestSuite struct {
	BenchmarkTestSuiteBase
	workloads []string
}

type BenchmarkTestSuiteBase struct {
	IntegrationTestSuiteBase
	perfContainers []string
	loadContainers []string
}

func (b *BenchmarkTestSuiteBase) StartPerfTools() {
	benchmark_options := config.BenchmarksInfo()
	perf := benchmark_options.PerfCommand
	bpftrace := benchmark_options.BpftraceCommand
	bcc := benchmark_options.BccCommand

	skipInit := benchmark_options.SkipInit

	if skipInit && (perf == "" && bpftrace == "" && bcc == "") {
		fmt.Fprintf(os.Stderr, "COLLECTOR_SKIP_HEADERS_INIT set, but no performance tool requested - ignoring.")
		return
	}

	if !skipInit && (perf != "" || bpftrace != "" || bcc != "") {
		b.RunInitContainer()
	}

	image_store := config.Images()

	if perf != "" {
		perf_image := image_store.QaImageByKey("performance-perf")
		b.StartPerfContainer("perf", perf_image, perf)
	}

	if bpftrace != "" {
		bpftrace_image := image_store.QaImageByKey("performance-bpftrace")
		b.StartPerfContainer("bpftrace", bpftrace_image, bpftrace)
	}

	if bcc != "" {
		bcc_image := image_store.QaImageByKey("performance-bcc")
		b.StartPerfContainer("bcc", bcc_image, bcc)
	}
}

func (b *BenchmarkTestSuiteBase) StartPerfContainer(name string, image string, args string) {
	argsList, err := shlex.Split(args)
	require.NoError(b.T(), err)
	b.startContainer(executor.ContainerStartConfig{
		Name:    name,
		Image:   image,
		Command: argsList,
	})
}

func (b *BenchmarkTestSuiteBase) RunInitContainer() {
	init_image := config.Images().QaImageByKey("performance-init")
	mounts := map[string]string{
		"/lib/modules":     "/lib/modules",
		"/etc/os-release":  "/etc/os-release",
		"/etc/lsb-release": "/etc/lsb-release",
		"/usr/src":         "/usr/src",
		"/boot":            "/boot",
	}

	containerID, err := b.Executor().StartContainer(executor.ContainerStartConfig{
		Name:   "host-init",
		Image:  init_image,
		Mounts: mounts,
	})
	require.NoError(b.T(), err)

	if finished, _ := b.waitForContainerToExit("host-init", containerID, 5*time.Second, 0); !finished {
		logs, err := b.containerLogs("host-init")
		if err == nil {
			fmt.Println(logs)
		}
		assert.FailNow(b.T(), "Failed to initialize host for performance testing")
	}
	b.cleanupContainers(containerID)
}

func (b *BenchmarkTestSuiteBase) startContainer(config executor.ContainerStartConfig) {

	config.Mounts["/sys"] = "/sys"
	config.Mounts["/usr/src"] = "/usr/src"
	config.Mounts["/lib/modules"] = "/lib/modules"
	// by mounting /tmp we can allow tools to write stats/logs to a local path
	// for later processing
	config.Mounts["/tmp"] = "/tmp"
	config.Privileged = true

	containerID, err := b.Executor().StartContainer(config)
	require.NoError(b.T(), err)

	b.perfContainers = append(b.perfContainers, containerID)
}

func (b *BenchmarkTestSuiteBase) FetchWorkloadLogs() {
	fmt.Println("Berserker logs:")
	for _, container := range b.loadContainers {
		log, err := b.containerLogs(container)
		require.NoError(b.T(), err)

		fmt.Println(log)
	}

	b.loadContainers = nil
}

func (b *BenchmarkTestSuiteBase) StopPerfTools() {
	b.stopContainers(b.perfContainers...)

	for _, container := range b.perfContainers {
		log, err := b.containerLogs(container)
		require.NoError(b.T(), err)

		fmt.Println(log)
	}

	b.removeContainers(b.perfContainers...)
	b.perfContainers = nil
}

func (s *BenchmarkCollectorTestSuite) SetupSuite() {
	s.RegisterCleanup("perf", "bcc", "bpftrace", "init",
		"benchmark-processes", "benchmark-endpoints")
	s.StartContainerStats()

	s.StartPerfTools()

	s.StartCollector(false, nil)
}

func (s *BenchmarkTestSuiteBase) SpinBerserker(workload string) (string, error) {
	benchmarkName := fmt.Sprintf("benchmark-%s", workload)
	benchmarkImage := config.Images().QaImageByKey("performance-berserker")

	err := s.Executor().PullImage(benchmarkImage)
	if err != nil {
		return "", err
	}

	configFile := fmt.Sprintf("/etc/berserker/%s/workload.toml", workload)

	containerID, err := s.Executor().StartContainer(executor.ContainerStartConfig{
		Name:    benchmarkName,
		Image:   benchmarkImage,
		Command: []string{configFile},
	})
	if err != nil {
		return "", err
	}

	s.loadContainers = append(s.loadContainers, containerID)
	return containerID, nil
}

func (s *BenchmarkTestSuiteBase) RunCollectorBenchmark() {
	procContainerID, err := s.SpinBerserker("processes")
	s.Require().NoError(err)

	endpointsContainerID, err := s.SpinBerserker("endpoints")
	s.Require().NoError(err)

	s.start = time.Now().UTC()

	// The assumption is that the benchmark is short, and to get better
	// resolution into when relevant metrics start and stop, tick more
	// frequently
	waitTick := 1 * time.Second

	// Container name here is used only for reporting
	_, err = s.waitForContainerToExit("berserker", procContainerID, waitTick, 0)
	s.Require().NoError(err)

	_, err = s.waitForContainerToExit("berserker", endpointsContainerID, waitTick, 0)

	s.Require().NoError(err)

	s.stop = time.Now().UTC()
}

func (s *BenchmarkCollectorTestSuite) TestBenchmarkCollector() {
	s.RunCollectorBenchmark()
}

func (s *BenchmarkCollectorTestSuite) TearDownSuite() {
	s.StopPerfTools()
	s.FetchWorkloadLogs()

	s.StopCollector()

	s.cleanupContainers("benchmark")
	s.WritePerfResults()
}

func (s *BenchmarkBaselineTestSuite) SetupSuite() {
	s.RegisterCleanup("benchmark-processes", "benchmark-endpoints")
	s.StartContainerStats()
	s.StartPerfTools()
}

func (s *BenchmarkBaselineTestSuite) TestBenchmarkBaseline() {
	s.RunCollectorBenchmark()
}

func (s *BenchmarkBaselineTestSuite) TearDownSuite() {
	s.StopPerfTools()
	s.FetchWorkloadLogs()
	s.cleanupContainers("benchmark")
	s.WritePerfResults()
}
