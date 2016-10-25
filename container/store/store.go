package store

import (
	"context"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container"
	"github.com/docker/docker/store"
)

// New returns a new container Store using the memdbWrapper
func New(s store.Store) container.Store {
	return &memdbWrapper{s}
}

type memdbWrapper struct {
	store.Store
}

func (s *memdbWrapper) Add(id string, c *container.Container) error {
	return s.Update(func(tx store.Tx) error {
		return store.CreateContainer(tx, c)
	})
}

func (s *memdbWrapper) Get(id string) *container.Container {
	var c *container.Container
	s.View(func(tx store.ReadTx) {
		c = store.GetContainer(tx, id)
	})
	return c
}

func (s *memdbWrapper) Delete(id string) error {
	return s.Update(func(tx store.Tx) error {
		return store.DeleteContainer(tx, id)
	})
}

func (s *memdbWrapper) List() []*container.Container {
	var ls []*container.Container
	var err error
	s.View(func(tx store.ReadTx) {
		ls, err = store.FindContainers(tx, store.All)
	})
	if err != nil {
		logrus.Errorf("Error listing containers: %v", err)
	}
	return ls
}

func (s *memdbWrapper) Size() int {
	return len(s.List())
}

func (s *memdbWrapper) First(f container.StoreFilter) *container.Container {
	for _, c := range s.List() {
		if f(c) {
			return c
		}
	}
	return nil
}

func (s *memdbWrapper) ApplyAll(r container.StoreReducer) {
	var (
		ls  []*container.Container
		err error
	)
	s.View(func(tx store.ReadTx) {
		ls, err = store.FindContainers(tx, store.All)
	})
	if err != nil {
		logrus.Errorf("Error getting container list: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(len(ls))
	for _, c := range ls {
		go func(c *container.Container) {
			r(c)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

func (s *memdbWrapper) Commit(c *container.Container) error {
	err := s.Update(func(tx store.Tx) error {
		return store.UpdateContainer(tx, c)
	})
	if err != nil {
		return err
	}
	return c.ToDisk()
}

// MatchContainerStop matches when the watched container is set to not running.
// Automatically checks for matching ID's since it would not make sense to use
// this with mismatching IDs.
func matchContainerStop(i, j *container.Container) bool {
	if !store.MatchContainerID(i, j) {
		return false
	}
	// TODO(cpuguy83): Does this really work in every case?
	return !j.Running
}

// WaitStop waits for a container update event where the container for the
// observed event matches the passed in container, and the updated container is
// set to not running.
// If the container is already stopped it returns immediately
// It returns the container from the update event
func (s *memdbWrapper) WaitStop(ctx context.Context, c *container.Container) (*container.Container, error) {
	if !c.Running {
		return c, nil
	}

	// Check if the stored container is already in a stopped state
	if c := s.Get(c.ID); c != nil && !c.Running {
		return c, nil
	}
	if !c.Running {
		return c, nil
	}
	return s.WaitAttachStop(ctx, c)
}

// WaitAttachStop waits for a container update event where the container for the
// observed event matches the passed in container, and the updated container is
// set to not running.
// It does not return if if the container is already stopped. This is primarly used
// for the attach API which allows users to attach to a stopped container.
// It returns the container from the update event
func (s *memdbWrapper) WaitAttachStop(ctx context.Context, c *container.Container) (*container.Container, error) {
	// Use a buffer of 1 in case there is a change from now until we start reading the wait chan
	// Call this early so we don't miss any events
	wait, cancel := s.SubscribeEvents(1, store.EventContainerUpdate{
		Container: c,
		Checks:    []store.ContainerCheckFunc{matchContainerStop},
	})
	defer cancel()

	select {
	case e := <-wait:
		updateEvent, ok := e.(store.EventContainerUpdate)
		if !ok {
			// if we get here, there is something wrong with events
			panic(fmt.Sprintf("got unexpected event: %v", e))
		}
		return updateEvent.Container, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
