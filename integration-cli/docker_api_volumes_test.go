package main

import (
	"encoding/json"
	"path"

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

func (s *DockerSuite) TestVolumesApiCreate(c *check.C) {
	config := types.VolumeCreateRequest{
		Name: "test",
	}
	status, b, err := sockRequest("POST", "/volumes", config)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var vol types.Volume
	err = json.Unmarshal(b, &vol)
	c.Assert(err, check.IsNil)

	c.Assert(path.Base(path.Dir(vol.Mountpoint)), check.Equals, config.Name)
}

func (s *DockerSuite) TestVolumesApiRemove(c *check.C) {
	dockerCmd(c, "run", "-d", "-v", "/foo", "busybox")

	status, b, err := sockRequest("GET", "/volumes", nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var volumes types.VolumesListResponse
	c.Assert(json.Unmarshal(b, &volumes), check.IsNil)
	c.Assert(len(volumes.Volumes), check.Equals, 1)

	v := volumes.Volumes[0]
	status, _, err = sockRequest("DELETE", "/volumes/"+v.Driver+"/"+v.Name, nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)
}

func (s *DockerSuite) TestVolumesApiInspect(c *check.C) {
	config := types.VolumeCreateRequest{
		Name: "test",
	}
	status, b, err := sockRequest("POST", "/volumes", config)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	status, b, err = sockRequest("GET", "/volumes", nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)

	var volumes types.VolumesListResponse
	c.Assert(json.Unmarshal(b, &volumes), check.IsNil)
	c.Assert(len(volumes.Volumes), check.Equals, 1)

	var vol types.Volume
	status, b, err = sockRequest("GET", "/volumes/"+volumes.Volumes[0].Driver+"/"+config.Name, nil)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, 200)
	c.Assert(json.Unmarshal(b, &vol), check.IsNil)
	c.Assert(vol.Name, check.Equals, config.Name)
}
