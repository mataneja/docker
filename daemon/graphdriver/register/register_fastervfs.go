// +build linux

package register

import (
	// register the fastervfs graphdriver
	_ "github.com/docker/docker/daemon/graphdriver/fastervfs"
)
