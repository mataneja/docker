package main

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/go-check/check"
)

func (s *DockerSuite) TestVolumesApiGetAll(c *check.C) {
	dockerCmd(c, "run", "-d", "-v", "/foo", "busybox")

	status, b, err := sockRequest("GET", "/volumes", nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var volumes types.VolumesListResponse
	c.Assert(json.Unmarshal(b, &volumes), check.IsNil)

	c.Assert(len(volumes.Volumes), check.Equals, 1)
}

func (s *DockerSuite) TestVolumesApiInspect(c *check.C) {
	out, _ := dockerCmd(c, "run", "-d", "-v", "/foo", "busybox")
	id := strings.TrimSpace(out)

	volPath, err := inspectFieldMap(id, "Volumes", "/foo")
	c.Assert(err, check.IsNil)
	volID := path.Base(path.Dir(volPath))

	status, b, err := sockRequest("GET", "/volumes/"+volID, nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var volume types.Volume
	c.Assert(json.Unmarshal(b, &volume), check.IsNil)
	c.Assert(volume.Mountpoint, check.Equals, volPath)
}
