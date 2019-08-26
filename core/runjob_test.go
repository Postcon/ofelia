package core

import (
	"archive/tar"
	"bytes"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	. "gopkg.in/check.v1"
)

const ImageFixture = "test-image"

type SuiteRunJob struct {
	server *testing.DockerServer
	client *docker.Client
}

var _ = Suite(&SuiteRunJob{})

func (s *SuiteRunJob) SetUpTest(c *C) {
	var err error
	s.server, err = testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, IsNil)

	s.client, err = docker.NewClient(s.server.URL())
	c.Assert(err, IsNil)

	s.buildImage(c)
	s.createNetwork(c)
}

func (s *SuiteRunJob) TestRun(c *C) {
	job := &RunJob{Client: s.client}
	job.Image = ImageFixture
	job.Command = `echo -a "foo bar"`
	job.User = "foo"
	job.TTY = true
	job.Delete = true
	job.Network = "foo"

	e := NewExecution()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		time.Sleep(time.Millisecond * 200)

		containers, err := s.client.ListContainers(docker.ListContainersOptions{})
		c.Assert(err, IsNil)
		c.Assert(containers[0].Command, Equals, "echo -a foo bar")
		c.Assert(containers[0].Status[:2], Equals, "Up")

		err = s.client.StopContainer(containers[0].ID, 0)
		c.Assert(err, IsNil)
		wg.Done()
	}()

	err := job.Run(&Context{Execution: e})
	c.Assert(err, IsNil)
	wg.Wait()

	containers, err := s.client.ListContainers(docker.ListContainersOptions{
		All: true,
	})
	c.Assert(err, IsNil)
	c.Assert(containers, HasLen, 0)
}

func (s *SuiteRunJob) TestBuildPullImageOptionsBareImage(c *C) {
	o, _ := buildPullOptions("foo", "")
	c.Assert(o.Repository, Equals, "foo")
	c.Assert(o.Tag, Equals, "latest")
	c.Assert(o.Registry, Equals, "")
}

func (s *SuiteRunJob) TestBuildPullImageOptionsVersion(c *C) {
	o, _ := buildPullOptions("foo:qux", "")
	c.Assert(o.Repository, Equals, "foo")
	c.Assert(o.Tag, Equals, "qux")
	c.Assert(o.Registry, Equals, "")
}

func (s *SuiteRunJob) TestBuildPullImageOptionsRegistry(c *C) {
	o, _ := buildPullOptions("srcd/rest:qux", "docker-registry.company.de:5000")
	c.Assert(o.Repository, Equals, "docker-registry.company.de:5000/srcd/rest")
	c.Assert(o.Tag, Equals, "qux")
	c.Assert(o.Registry, Equals, "docker-registry.company.de:5000")
}

func (s *SuiteRunJob) buildImage(c *C) {
	inputbuf := bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{Name: "Dockerfile"})
	tr.Write([]byte("FROM base\n"))
	tr.Close()

	err := s.client.BuildImage(docker.BuildImageOptions{
		Name:         ImageFixture,
		InputStream:  inputbuf,
		OutputStream: bytes.NewBuffer(nil),
	})
	c.Assert(err, IsNil)
}

func (s *SuiteRunJob) createNetwork(c *C) {
	_, err := s.client.CreateNetwork(docker.CreateNetworkOptions{
		Name:   "foo",
		Driver: "bridge",
	})
	c.Assert(err, IsNil)
}
