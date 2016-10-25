package daemon

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/container"
	"github.com/docker/docker/layer"
	volumestore "github.com/docker/docker/volume/store"
)

// ContainerRm removes the container id from the filesystem. An error
// is returned if the container is not found, or if the remove
// fails. If the remove succeeds, the container name is released, and
// network links are removed.
func (daemon *Daemon) ContainerRm(name string, config *types.ContainerRmConfig) error {
	start := time.Now()
	container, err := daemon.GetContainer(name)
	if err != nil {
		return err
	}

	daemon.stateLock.Lock(container.ID)
	defer daemon.stateLock.Unlock(container.ID)

	if config.RemoveLink {
		return daemon.rmLink(container, name)
	}

	err = daemon.cleanupContainer(container, config.ForceRemove, config.RemoveVolume)
	containerActions.WithValues("delete").UpdateSince(start)
	return err
}

func (daemon *Daemon) rmLink(container *container.Container, name string) error {
	if name[0] != '/' {
		name = "/" + name
	}
	parent, n := path.Split(name)
	if parent == "/" {
		return fmt.Errorf("Conflict, cannot remove the default name of the container")
	}

	parent = strings.TrimSuffix(parent, "/")
	pe, err := daemon.nameIndex.Get(parent)
	if err != nil {
		return fmt.Errorf("Cannot get parent %s for name %s", parent, name)
	}

	daemon.releaseName(name)
	parentContainer, _ := daemon.GetContainer(pe)
	if parentContainer != nil {
		daemon.linkIndex.unlink(name, container, parentContainer)
		if err := daemon.updateNetwork(parentContainer); err != nil {
			logrus.Debugf("Could not update network to remove link %s: %v", n, err)
		}
	}
	return nil
}

// cleanupContainer unregisters a container from the daemon, stops stats
// collection and cleanly removes contents and metadata from the filesystem.
func (daemon *Daemon) cleanupContainer(c *container.Container, forceRemove, removeVolume bool) (err error) {
	// Container state RemovalInProgress should be used to avoid races.
	if inProgress := c.SetRemovalInProgress(); inProgress {
		err := fmt.Errorf("removal of container %s is already in progress", c.Name)
		return errors.NewBadRequestError(err)
	}

	if c.IsRunning() {
		if !forceRemove {
			err := fmt.Errorf("You cannot remove a running container %s. Stop the container before attempting removal or use -f", c.ID)
			return errors.NewRequestConflictError(err)
		}
		if err := daemon.Kill(c); err != nil {
			return fmt.Errorf("Could not kill running container %s, cannot remove - %v", c.ID, err)
		}
		// Make sure we have the most recent container updates
		// TODO(@cpuguy83): this is a race condition since the container monitor may
		// itself update the container in another goroutine but we really don't know
		// when this will happen
		c = daemon.containers.Get(c.ID)
	}

	// stop collection of stats for the container regardless
	// if stats are currently getting collected.
	daemon.statsCollector.stopCollection(c)

	if err = daemon.containerStop(c, 3); err != nil {
		return err
	}

	// Mark container dead. We don't want anybody to be restarting it.
	c.SetDead()

	// Save container state to disk. So that if error happens before
	// container meta file got removed from disk, then a restart of
	// docker should not make a dead container alive.
	if err := daemon.containers.Commit(c); err != nil {
		logrus.Errorf("Error saving dying container to disk: %+v", err)
	}

	// If force removal is required, delete container from various
	// indexes even if removal failed.
	defer func() {
		if err == nil || forceRemove {
			daemon.nameIndex.Delete(c.ID)
			daemon.linkIndex.delete(c)
			selinuxFreeLxcContexts(c.ProcessLabel)
			daemon.containers.Delete(c.ID)
			if e := daemon.removeMountPoints(c, removeVolume); e != nil {
				logrus.Error(e)
			}
			container.RemoveRunState(c)
			daemon.LogContainerEvent(c, "destroy")
		} else {
			c.ResetRemovalInProgress()
			daemon.containers.Commit(c)
		}
	}()

	if err = os.RemoveAll(c.Root); err != nil {
		return fmt.Errorf("Unable to remove filesystem for %v: %v", c.ID, err)
	}

	// When container creation fails and `RWLayer` has not been created yet, we
	// do not call `ReleaseRWLayer`
	if c.RWLayer != nil {
		metadata, err := daemon.layerStore.ReleaseRWLayer(c.RWLayer)
		layer.LogReleaseMetadata(metadata)
		if err != nil && err != layer.ErrMountDoesNotExist {
			return fmt.Errorf("Driver %s failed to remove root filesystem %s: %s", daemon.GraphDriverName(), c.ID, err)
		}
	}

	return nil
}

// VolumeRm removes the volume with the given name.
// If the volume is referenced by a container it is not removed
// This is called directly from the remote API
func (daemon *Daemon) VolumeRm(name string, force bool) error {
	err := daemon.volumeRm(name)
	if err == nil || force {
		daemon.volumes.Purge(name)
		return nil
	}
	return err
}

func (daemon *Daemon) volumeRm(name string) error {
	v, err := daemon.volumes.Get(name)
	if err != nil {
		return err
	}

	if err := daemon.volumes.Remove(v); err != nil {
		if volumestore.IsInUse(err) {
			err := fmt.Errorf("Unable to remove volume, volume still in use: %v", err)
			return errors.NewRequestConflictError(err)
		}
		return fmt.Errorf("Error while removing volume %s: %v", name, err)
	}
	daemon.LogVolumeEvent(v.Name(), "destroy", map[string]string{"driver": v.DriverName()})
	return nil
}
