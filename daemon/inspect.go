package daemon

import (
	"errors"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/directory"
	"github.com/docker/docker/pkg/parsers/filters"
	"github.com/docker/docker/volume"
	"github.com/docker/docker/volume/drivers"
)

func (daemon *Daemon) ContainerInspect(name string) (*types.ContainerJSON, error) {
	container, err := daemon.Get(name)
	if err != nil {
		return nil, err
	}

	container.Lock()
	defer container.Unlock()

	base, err := daemon.getInspectData(container)
	if err != nil {
		return nil, err
	}

	return &types.ContainerJSON{base, container.Config}, nil
}

func (daemon *Daemon) ContainerInspectRaw(name string) (*types.ContainerJSONRaw, error) {
	container, err := daemon.Get(name)
	if err != nil {
		return nil, err
	}

	container.Lock()
	defer container.Unlock()

	base, err := daemon.getInspectData(container)
	if err != nil {
		return nil, err
	}

	config := &types.ContainerConfig{
		container.Config,
		container.hostConfig.Memory,
		container.hostConfig.MemorySwap,
		container.hostConfig.CpuShares,
		container.hostConfig.CpusetCpus,
	}

	return &types.ContainerJSONRaw{base, config}, nil
}

func (daemon *Daemon) getInspectData(container *Container) (*types.ContainerJSONBase, error) {
	// make a copy to play with
	hostConfig := *container.hostConfig

	if children, err := daemon.Children(container.Name); err == nil {
		for linkAlias, child := range children {
			hostConfig.Links = append(hostConfig.Links, fmt.Sprintf("%s:%s", child.Name, linkAlias))
		}
	}
	// we need this trick to preserve empty log driver, so
	// container will use daemon defaults even if daemon change them
	if hostConfig.LogConfig.Type == "" {
		hostConfig.LogConfig = daemon.defaultLogConfig
	}

	containerState := &types.ContainerState{
		Running:    container.State.Running,
		Paused:     container.State.Paused,
		Restarting: container.State.Restarting,
		OOMKilled:  container.State.OOMKilled,
		Dead:       container.State.Dead,
		Pid:        container.State.Pid,
		ExitCode:   container.State.ExitCode,
		Error:      container.State.Error,
		StartedAt:  container.State.StartedAt,
		FinishedAt: container.State.FinishedAt,
	}

	volumes := make(map[string]string)
	volumesRW := make(map[string]bool)

	for _, m := range container.MountPoints {
		volumes[m.Destination] = m.Path()
		volumesRW[m.Destination] = m.RW
	}

	contJSONBase := &types.ContainerJSONBase{
		Id:              container.ID,
		Created:         container.Created,
		Path:            container.Path,
		Args:            container.Args,
		State:           containerState,
		Image:           container.ImageID,
		NetworkSettings: container.NetworkSettings,
		ResolvConfPath:  container.ResolvConfPath,
		HostnamePath:    container.HostnamePath,
		HostsPath:       container.HostsPath,
		LogPath:         container.LogPath,
		Name:            container.Name,
		RestartCount:    container.RestartCount,
		Driver:          container.Driver,
		ExecDriver:      container.ExecDriver,
		MountLabel:      container.MountLabel,
		ProcessLabel:    container.ProcessLabel,
		Volumes:         volumes,
		VolumesRW:       volumesRW,
		AppArmorProfile: container.AppArmorProfile,
		ExecIDs:         container.GetExecIDs(),
		HostConfig:      &hostConfig,
	}

	contJSONBase.GraphDriver.Name = container.Driver
	graphDriverData, err := daemon.driver.GetMetadata(container.ID)
	if err != nil {
		return nil, err
	}
	contJSONBase.GraphDriver.Data = graphDriverData

	return contJSONBase, nil
}

func (daemon *Daemon) ContainerExecInspect(id string) (*execConfig, error) {
	eConfig, err := daemon.getExecConfig(id)
	if err != nil {
		return nil, err
	}

	return eConfig, nil
}

func (daemon *Daemon) VolumeInspect(name, filter string, size bool) (*types.Volume, error) {
	inspectFilters, err := filters.FromParam(filter)
	if err != nil {
		return nil, err
	}

	filterDriverNames := make(map[string]struct{})
	if i, ok := inspectFilters["driver"]; ok {
		for _, value := range i {
			filterDriverNames[value] = struct{}{}
		}
	}
	filterByDriver := len(filterDriverNames) > 0

	if !filterByDriver {
		// try getting from the local driver first
		localDrv, err := volumedrivers.Lookup(volume.DefaultDriverName)
		if err != nil {
			return nil, err
		}
		v, err := localDrv.Get(name)
		if err == nil {
			return volumeToAPIType(v, size), nil
		}
	}

	drivers := volumedrivers.List()
	for drvName, d := range drivers {
		_, ok := filterDriverNames[drvName]
		if !filterByDriver || (filterByDriver && ok) {
			v, err := d.Get(name)
			if err == nil {
				return volumeToAPIType(v, size), nil
			}
		}
	}

	return nil, errors.New("No such volume with name: " + name)
}

func volumeToAPIType(v volume.Volume, size bool) *types.Volume {
	vv := &types.Volume{
		Name:       v.Name(),
		Driver:     v.DriverName(),
		Mountpoint: v.Path(),
	}
	var err error
	if size {
		vv.Size, err = directory.Size(vv.Mountpoint)
		if err != nil {
			vv.Size = -1
		}
	}

	return vv
}
