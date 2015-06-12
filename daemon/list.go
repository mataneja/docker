package daemon

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/directory"
	"github.com/docker/docker/pkg/graphdb"
	"github.com/docker/docker/pkg/nat"
	"github.com/docker/docker/pkg/parsers/filters"
	volumedrivers "github.com/docker/docker/volume/drivers"
)

// List returns an array of all containers registered in the daemon.
func (daemon *Daemon) List() []*Container {
	return daemon.containers.List()
}

type ContainersConfig struct {
	All     bool
	Since   string
	Before  string
	Limit   int
	Size    bool
	Filters string
}

func (daemon *Daemon) Containers(config *ContainersConfig) ([]*types.Container, error) {
	var (
		foundBefore bool
		displayed   int
		all         = config.All
		n           = config.Limit
		psFilters   filters.Args
		filtExited  []int
	)
	containers := []*types.Container{}

	psFilters, err := filters.FromParam(config.Filters)
	if err != nil {
		return nil, err
	}
	if i, ok := psFilters["exited"]; ok {
		for _, value := range i {
			code, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			filtExited = append(filtExited, code)
		}
	}

	if i, ok := psFilters["status"]; ok {
		for _, value := range i {
			if value == "exited" || value == "created" {
				all = true
			}
		}
	}
	names := map[string][]string{}
	daemon.ContainerGraph().Walk("/", func(p string, e *graphdb.Entity) error {
		names[e.ID()] = append(names[e.ID()], p)
		return nil
	}, 1)

	var beforeCont, sinceCont *Container
	if config.Before != "" {
		beforeCont, err = daemon.Get(config.Before)
		if err != nil {
			return nil, err
		}
	}

	if config.Since != "" {
		sinceCont, err = daemon.Get(config.Since)
		if err != nil {
			return nil, err
		}
	}

	errLast := errors.New("last container")
	writeCont := func(container *Container) error {
		container.Lock()
		defer container.Unlock()
		if !container.Running && !all && n <= 0 && config.Since == "" && config.Before == "" {
			return nil
		}
		if !psFilters.Match("name", container.Name) {
			return nil
		}

		if !psFilters.Match("id", container.ID) {
			return nil
		}

		if !psFilters.MatchKVList("label", container.Config.Labels) {
			return nil
		}

		if config.Before != "" && !foundBefore {
			if container.ID == beforeCont.ID {
				foundBefore = true
			}
			return nil
		}
		if n > 0 && displayed == n {
			return errLast
		}
		if config.Since != "" {
			if container.ID == sinceCont.ID {
				return errLast
			}
		}
		if len(filtExited) > 0 {
			shouldSkip := true
			for _, code := range filtExited {
				if code == container.ExitCode && !container.Running {
					shouldSkip = false
					break
				}
			}
			if shouldSkip {
				return nil
			}
		}

		if !psFilters.Match("status", container.State.StateString()) {
			return nil
		}
		displayed++
		newC := &types.Container{
			ID:    container.ID,
			Names: names[container.ID],
		}
		newC.Image = container.Config.Image
		if len(container.Args) > 0 {
			args := []string{}
			for _, arg := range container.Args {
				if strings.Contains(arg, " ") {
					args = append(args, fmt.Sprintf("'%s'", arg))
				} else {
					args = append(args, arg)
				}
			}
			argsAsString := strings.Join(args, " ")

			newC.Command = fmt.Sprintf("%s %s", container.Path, argsAsString)
		} else {
			newC.Command = fmt.Sprintf("%s", container.Path)
		}
		newC.Created = int(container.Created.Unix())
		newC.Status = container.State.String()
		newC.HostConfig.NetworkMode = string(container.HostConfig().NetworkMode)

		newC.Ports = []types.Port{}
		for port, bindings := range container.NetworkSettings.Ports {
			p, _ := nat.ParsePort(port.Port())
			if len(bindings) == 0 {
				newC.Ports = append(newC.Ports, types.Port{
					PrivatePort: p,
					Type:        port.Proto(),
				})
				continue
			}
			for _, binding := range bindings {
				h, _ := nat.ParsePort(binding.HostPort)
				newC.Ports = append(newC.Ports, types.Port{
					PrivatePort: p,
					PublicPort:  h,
					Type:        port.Proto(),
					IP:          binding.HostIp,
				})
			}
		}

		if config.Size {
			sizeRw, sizeRootFs := container.GetSize()
			newC.SizeRw = int(sizeRw)
			newC.SizeRootFs = int(sizeRootFs)
		}
		newC.Labels = container.Config.Labels
		containers = append(containers, newC)
		return nil
	}

	for _, container := range daemon.List() {
		if err := writeCont(container); err != nil {
			if err != errLast {
				return nil, err
			}
			break
		}
	}
	return containers, nil
}

func (daemon *Daemon) Volumes(filter string, quiet, size bool) (volumesOut []*types.Volume, warnings []string, err error) {
	volFilters, err := filters.FromParam(filter)
	if err != nil {
		return nil, nil, err
	}
	filterUsed := false
	usedVolumes := make(map[string]struct{})
	if i, ok := volFilters["dangling"]; ok {
		if len(i) > 1 {
			return nil, nil, fmt.Errorf("Conflict: cannot use more than 1 value for `dangling` filter")
		}

		filterValue := i[0]

		if strings.ToLower(filterValue) == "true" || filterValue == "1" {
			filterUsed = true
			containers := daemon.containers.List()
			for _, c := range containers {
				for _, mnt := range c.MountPoints {
					usedVolumes[mnt.Volume.Path()] = struct{}{}
				}
			}
		}
	}

	drivers := volumedrivers.List()
	for driverName, d := range drivers {
		volumes, err := d.List()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("error getting volumes from driver %s: %v", driverName, err))
			continue
		}

		for _, vol := range volumes {
			if filterUsed {
				if _, exists := usedVolumes[vol.Path()]; exists {
					continue
				}
			}
			v := &types.Volume{
				Name:   vol.Name(),
				Driver: driverName,
			}

			if size {
				v.Size, err = directory.Size(vol.Path())
				if err != nil {
					v.Size = -1
					warnings = append(warnings, fmt.Sprintf("error getting size for volume %s/%s: %v", v.Driver, v.Name, err))
				}
			}
			volumesOut = append(volumesOut, v)
		}
	}

	return volumesOut, warnings, nil
}
