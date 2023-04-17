package suites

import (
	"time"

	"github.com/stackrox/collector/integration-tests/suites/common"
	"github.com/stretchr/testify/assert"
)

type ConnectionsAndEndpointsTestSuite struct {
	IntegrationTestSuiteBase
	ListenContainer  string
	ListenContainer2 string
	SendtoContainer  string
}

func makeLongMessage() string {
	var result string

	for i := 0; i < 100; i++ {
		result += "hello "
	}

	return result
}

func (s *ConnectionsAndEndpointsTestSuite) SetupSuite() {

	s.metrics = map[string]float64{}
	s.executor = common.NewExecutor()
	s.StartContainerStats()
	s.collector = common.NewCollectorManager(s.executor, s.T().Name())

	s.collector.Env["COLLECTOR_CONFIG"] = `{"logLevel":"debug","turnOffScrape":false,"scrapeInterval":2}`
	s.collector.Env["ROX_PROCESSES_LISTENING_ON_PORT"] = "true"
	s.collector.Env["ROX_AFTERGLOW_PERIOD"] = "0"

	err := s.collector.Setup()
	s.Require().NoError(err)

	err = s.collector.Launch()
	s.Require().NoError(err)
	time.Sleep(30 * time.Second)

	socatImage := common.QaImage("quay.io/rhacs-eng/qa", "socat")

	containerListenID, err := s.launchContainer("socat-listen", socatImage, "/bin/sh", "-c", "/bin/sleep 3000")
	s.Require().NoError(err)

	containerSendtoID, err := s.launchContainer("socat-send", socatImage, "/bin/sh", "-c", "/bin/sleep 3000")
	s.Require().NoError(err)
	time.Sleep(5 * time.Second)

	listenIP, err := s.getIPAddress("socat-listen")
	s.Require().NoError(err)

	_, err = s.execContainer("socat-listen", []string{"/bin/sh", "-c", "socat -d -d -v - TCP-LISTEN:80,reuseaddr >> /socat-log.txt &"}) // Without fork endpoint is opened and closed. Test fails
	s.Require().NoError(err)
	time.Sleep(5 * time.Second)

	_, err = s.execContainer("socat-send", []string{"/bin/sh", "-c", "while [ true ]; do echo hello ; sleep 1; done | socat -d -d -v - TCP:" + listenIP + ":80 &> /socat-log.txt &"})
	s.Require().NoError(err)

	s.ListenContainer = common.ContainerShortID(containerListenID)
	s.SendtoContainer = common.ContainerShortID(containerSendtoID)

	time.Sleep(6 * time.Second)

	err = s.collector.TearDown()
	s.Require().NoError(err)

	s.db, err = s.collector.BoltDB()
	s.Require().NoError(err)
}

func (s *ConnectionsAndEndpointsTestSuite) TearDownSuite() {
	//s.cleanupContainer([]string{"socat-listen", "socat-send", "collector"})
	stats := s.GetContainerStats()
	s.PrintContainerStats(stats)
	s.WritePerfResults("ConnectionsAndEndpoints", stats, s.metrics)
}

func (s *ConnectionsAndEndpointsTestSuite) TestConnectionsAndEndpoints() {
	processes, err := s.GetProcesses(s.ListenContainer)
	s.Require().NoError(err)

	assert.Equal(s.T(), 4, len(processes))

	sendProcesses, err := s.GetProcesses(s.SendtoContainer)
	s.Require().NoError(err)
	sendEndpoints, err := s.GetEndpoints(s.SendtoContainer)
	s.Require().Error(err)

	// There should not be any UDP listening endpoints in the container sending UDP messages
	assert.Equal(s.T(), 0, len(sendEndpoints))
	assert.Equal(s.T(), 4, len(sendProcesses))

	endpoints, err := s.GetEndpoints(s.ListenContainer)
	s.Require().NoError(err)

	if !assert.Equal(s.T(), 1, len(endpoints)) {
		// We can't continue if this is not the case, so panic immediately.
		// It indicates an internal issue with this test and the non-deterministic
		// way in which endpoints are reported.
		assert.FailNowf(s.T(), "", "retrieved %d endpoints (expect 1)", len(endpoints))
	}

	//assert.Equal(s.T(), "L4_PROTOCOL_UDP", endpoints[0].Protocol)
	//assert.Equal(s.T(), endpoints[0].Originator.ProcessName, processes[3].Name)
	//assert.Equal(s.T(), endpoints[0].Originator.ProcessExecFilePath, processes[3].ExePath)
	//assert.Equal(s.T(), endpoints[0].Originator.ProcessArgs, processes[3].Args)
	//assert.Equal(s.T(), 80, endpoints[0].Address.Port)

	//endpoints2, err := s.GetEndpoints(s.ListenContainer2)
	//s.Require().NoError(err)

	//assert.Equal(s.T(), 1, len(endpoints2))
}
