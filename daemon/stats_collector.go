// +build !solaris

package daemon

import (
	"bufio"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/container"
	"github.com/docker/docker/pkg/pubsub"
)

type statsSupervisor interface {
	// GetContainerStats collects all the stats related to a container
	GetContainerStats(container *container.Container) (*types.StatsJSON, error)
}

// newStatsCollector returns a new statsCollector that collections
// stats for a registered container at the specified interval.
// The collector allows non-running containers to be added
// and will start processing stats when they are started.
func (daemon *Daemon) newStatsCollector(interval time.Duration) *statsCollector {
	s := &statsCollector{
		interval:   interval,
		supervisor: daemon,
		publishers: make(map[string]*publishersPair),
		bufReader:  bufio.NewReaderSize(nil, 128),
	}
	platformNewStatsCollector(s)
	go s.run()
	return s
}

type publishersPair struct {
	container *container.Container
	publisher *pubsub.Publisher
}

// statsCollector manages and provides container resource stats
type statsCollector struct {
	m          sync.Mutex
	supervisor statsSupervisor
	interval   time.Duration
	publishers map[string]*publishersPair
	bufReader  *bufio.Reader

	// The following fields are not set on Windows currently.
	clockTicksPerSecond uint64
	machineMemory       uint64
}

// collect registers the container with the collector and adds it to
// the event loop for collection on the specified interval returning
// a channel for the subscriber to receive on.
func (s *statsCollector) collect(c *container.Container) chan interface{} {
	s.m.Lock()
	defer s.m.Unlock()
	pair, exists := s.publishers[c.ID]
	if !exists {
		publisher := pubsub.NewPublisher(100*time.Millisecond, 1024)
		pair = &publishersPair{c, publisher}
		s.publishers[c.ID] = pair
	}
	return pair.publisher.Subscribe()
}

// stopCollection closes the channels for all subscribers and removes
// the container from metrics collection.
func (s *statsCollector) stopCollection(c *container.Container) {
	s.m.Lock()
	if pair, exists := s.publishers[c.ID]; exists {
		pair.publisher.Close()
		delete(s.publishers, c.ID)
	}
	s.m.Unlock()
}

// unsubscribe removes a specific subscriber from receiving updates for a container's stats.
func (s *statsCollector) unsubscribe(c *container.Container, ch chan interface{}) {
	s.m.Lock()
	pair := s.publishers[c.ID]
	if pair != nil {
		pair.publisher.Evict(ch)
		if pair.publisher.Len() == 0 {
			delete(s.publishers, c.ID)
		}
	}
	s.m.Unlock()
}

func (s *statsCollector) run() {

	// we cannot determine the capacity here.
	// it will grow enough in first iteration
	var pairs []*publishersPair

	for range time.Tick(s.interval) {
		// it does not make sense in the first iteration,
		// but saves allocations in further iterations
		pairs = pairs[:0]

		s.m.Lock()
		for _, pair := range s.publishers {
			// copy pointers here to release the lock ASAP
			pairs = append(pairs, pair)
		}
		s.m.Unlock()
		if len(pairs) == 0 {
			continue
		}

		systemUsage, err := s.getSystemCPUUsage()
		if err != nil {
			logrus.Errorf("collecting system cpu usage: %v", err)
			continue
		}

		for _, pair := range pairs {
			stats, err := s.supervisor.GetContainerStats(pair.container)
			if err != nil {
				if _, ok := err.(errNotRunning); !ok {
					logrus.Errorf("collecting stats for %s: %v", pair.container.ID, err)
				}
				continue
			}
			// FIXME: move to containerd on Linux (not Windows)
			stats.CPUStats.SystemUsage = systemUsage

			pair.publisher.Publish(*stats)
		}
	}
}
