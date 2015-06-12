package main

import (
	"encoding/json"

	"github.com/docker/docker/api/types"
	"github.com/go-check/check"
)

func (s *DockerSuite) TestVolumesGetAll(c *check.C) {
	dockerCmd(c, "run", "-d", "-v", "/foo", "busybox")

	status, b, err := sockRequest("GET", "/volumes", nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var volumes types.VolumesListResponse
	c.Assert(json.Unmarshal(b, &volumes), check.IsNil)

	c.Assert(len(volumes.Volumes), check.Equals, 1)
}
