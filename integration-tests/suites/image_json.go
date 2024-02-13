package suites

import (
	"github.com/stackrox/collector/integration-tests/suites/common"
	"github.com/stackrox/collector/integration-tests/suites/config"
)

type ImageLabelJSONTestSuite struct {
	IntegrationTestSuiteBase
}

func (s *ImageLabelJSONTestSuite) SetupSuite() {
	s.RegisterCleanup()
	s.StartCollector(false, nil)
}

func (s *ImageLabelJSONTestSuite) TestRunImageWithJSONLabel() {
	name := "jsonlabel"
	image := config.Images().QaImageByKey("performance-json-label")

	err := s.Executor().PullImage(image)
	s.Require().NoError(err)

	containerID, err := s.startContainer(common.ContainerStartConfig{
		Name:  name,
		Image: image})
	s.Require().NoError(err)

	_, err = s.waitForContainerToExit(name, containerID, defaultWaitTickSeconds, 0)
	s.Require().NoError(err)
}

func (s *ImageLabelJSONTestSuite) TearDownSuite() {
	s.cleanupContainers("jsonlabel")
}
