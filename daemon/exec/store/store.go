package store

import (
	"github.com/docker/docker/daemon/exec"
	"github.com/docker/docker/store"
)

func New(s store.Store) exec.Store {
	return &memdbWrapper{s}
}

type memdbWrapper struct {
	store.Store
}

func (s *memdbWrapper) Commands() []*exec.Config {
	var ls []*exec.Config
	s.View(func(tx store.ReadTx) {
		ls, _ = store.FindExecs(tx, store.All)
	})
	return ls
}

func (s *memdbWrapper) Add(c *exec.Config) error {
	return s.Update(func(tx store.Tx) error {
		return store.CreateExec(tx, c)
	})
}

func (s *memdbWrapper) Get(id string) *exec.Config {
	var c *exec.Config
	s.View(func(tx store.ReadTx) {
		c = store.GetExec(tx, id)
	})
	return c
}

func (s *memdbWrapper) CommandsByContainerID(id string) []*exec.Config {
	var ls []*exec.Config
	s.View(func(tx store.ReadTx) {
		ls, _ = store.FindExecs(tx, store.ByContainerID(id))
	})
	return ls
}

func (s *memdbWrapper) Commit(c *exec.Config) error {
	return s.Update(func(tx store.Tx) error {
		return store.UpdateExec(tx, c)
	})
}

func (s *memdbWrapper) Delete(id string) error {
	return s.Update(func(tx store.Tx) error {
		return store.DeleteExec(tx, id)
	})
}

func (s *memdbWrapper) List(containerID string) []string {
	var ls []string
	for _, cmd := range s.CommandsByContainerID(containerID) {
		ls = append(ls, cmd.ID)
	}
	return ls
}
