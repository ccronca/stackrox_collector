package suites

import (
	"fmt"

	"github.com/stackrox/collector/integration-tests/pkg/config"
)

type UdpNetworkFlow struct {
	IntegrationTestSuiteBase
}

const (
	UDP_CLIENT   = "udp-client"
	UDP_SERVER   = "udp-server"
	TEST_NETWORK = "udp-tests"
)

func (s *UdpNetworkFlow) SetupSuite() {
	s.RegisterCleanup(UDP_CLIENT, UDP_SERVER)
	s.StartContainerStats()
	s.StartCollector(false, nil)

	image_store := config.Images()

	image := image_store.QaImageByKey("udp")

	err := s.Executor().PullImage(image)
	s.Require().NoError(err)

	// Create a network for the UDP containers
	s.Executor().CreateNetwork(TEST_NETWORK)
	s.T().Cleanup(func() {
		s.Executor().RemoveNetwork(TEST_NETWORK)
	})

	// Run the server
	s.launchContainer(UDP_SERVER, "--network", TEST_NETWORK, image)

	// Run the client
	client_cmd := []string{
		"--entrypoint", "udp-client",
		"--network", TEST_NETWORK,
		image,
		fmt.Sprintf("%s:9090", UDP_SERVER),
	}
	s.launchContainer(UDP_CLIENT, client_cmd...)
}

func (s *UdpNetworkFlow) TearDownSuite() {
	s.WritePerfResults()
}

func (s *UdpNetworkFlow) TestRecvfrom() {
	// TODO
}
