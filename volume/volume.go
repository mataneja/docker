package volume

import (
	"sync"

	"github.com/Sirupsen/logrus"
)

const DefaultDriverName = "local"

var Counter = &volCounter{c: make(map[string]map[string]int)}

type Driver interface {
	// Name returns the name of the volume driver.
	Name() string
	// Create makes a new volume with the given id.
	Create(name string, opts map[string]string) (Volume, error)
	// Remove deletes the volume.
	Remove(Volume) error
	// List all volumes of the driver.
	List() ([]Volume, error)
	// Get volume with given name
	Get(name string) (Volume, error)
}

type Volume interface {
	// Name returns the name of the volume
	Name() string
	// DriverName returns the name of the driver which owns this volume.
	DriverName() string
	// Path returns the absolute path to the volume.
	Path() string
	// Mount mounts the volume and returns the absolute path to
	// where it can be consumed.
	Mount() (string, error)
	// Unmount unmounts the volume when it is no longer in use.
	Unmount() error
}

type volCounter struct {
	c  map[string]map[string]int
	mu sync.Mutex
}

func (c *volCounter) Increment(driver, name string) {
	logrus.Debugf("VOLUMES incrementing %s - %s", driver, name)
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.c[driver]; !exists {
		c.c[driver] = make(map[string]int)
	}

	if _, exists := c.c[driver][name]; !exists {
		c.c[driver][name] = 1
		return
	}

	c.c[driver][name]++
}

func (c *volCounter) Decrement(driver, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.c[driver]; !exists {
		return
	}
	if _, exists := c.c[driver][name]; !exists {
		return
	}
	if i := c.c[driver][name]; i > 0 {
		c.c[driver][name]--
	}
}

func (c *volCounter) Count(driver, name string) int {
	if _, exists := c.c[driver]; !exists {
		return 0
	}
	return c.c[driver][name]
}
