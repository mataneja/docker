package memdbstore

import (
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container"
	"github.com/docker/docker/store"
)

func New() container.Store {
	return &memdbWrapper{store.NewMemoryStore()}
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
	_, err := s.Batch(func(batch *store.Batch) error {
		var (
			ls  []*container.Container
			err error
		)
		s.View(func(tx store.ReadTx) {
			ls, err = store.FindContainers(tx, store.All)
		})
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		wg.Add(len(ls))
		for _, c := range ls {
			go func() {
				err = batch.Update(func(tx store.Tx) error {
					r(c)
					return nil
				})
				if err != nil {
					logrus.Error(err)
				}
				wg.Done()
			}()
		}
		wg.Wait()
		return nil
	})
	if err != nil {
		logrus.Error(err)
	}
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
