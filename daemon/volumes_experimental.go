// +build experimental

package daemon

import (
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/volume"
	"github.com/docker/docker/volume/drivers"
)

func getVolumeDriver(name string) (volume.Driver, error) {
	logrus.Debugf(name)
	if name == "" {
		name = volume.DefaultDriverName
	}
	return volumedrivers.Lookup(name)
}

func parseVolumeSource(spec string) (string, string, error) {
	if !filepath.IsAbs(spec) {
		return spec, "", nil
	}

	return "", spec, nil
}
