package daemon

import (
	"fmt"

	"github.com/docker/docker/container"
	"github.com/docker/docker/libcontainerd"
)

func (daemon *Daemon) getLibcontainerdCreateOptions(container *container.Container) (*[]libcontainerd.CreateOption, error) {
	createOptions := []libcontainerd.CreateOption{}

	// Ensure a runtime has been assigned to this container
	if container.HostConfig.Runtime == "" {
		container.Lock()
		container.HostConfig.Runtime = stockRuntimeName
		container.ToDisk()
		container.Unlock()
	}

	rt := daemon.configStore.GetRuntime(container.HostConfig.Runtime)
	if rt == nil {
		return nil, fmt.Errorf("no such runtime '%s'", container.HostConfig.Runtime)
	}
	createOptions = append(createOptions, libcontainerd.WithRuntime(rt.Path, rt.Args))

	return &createOptions, nil
}
