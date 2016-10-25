package container

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container/stream"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/restartmanager"
)

const (
	loggerCloseTimeout = 10 * time.Second
)

var containerState = &stateStore{s: make(map[string]*RunState)}

type stateStore struct {
	mu sync.Mutex
	s  map[string]*RunState
}

func GetRunState(c *Container) *RunState {
	containerState.mu.Lock()
	s := containerState.s[c.ID]
	containerState.mu.Unlock()
	return s
}

func RemoveRunState(c *Container) {
	containerState.mu.Lock()
	delete(containerState.s, c.ID)
	containerState.mu.Unlock()
}

func AddRunState(c *Container, s *RunState) {
	containerState.mu.Lock()
	containerState.s[c.ID] = s
	containerState.mu.Unlock()
}

type RunState struct {
	healthMonitor  chan struct{}
	logDriver      logger.Logger
	logCopier      *logger.Copier
	restartManager restartmanager.RestartManager
	streams        *stream.Config
	mu             sync.Mutex
}

func newRunState() *RunState {
	return &RunState{
		streams: stream.NewConfig(),
	}
}

func (s *RunState) OpenHealthMonitor() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.healthMonitor == nil {
		s.healthMonitor = make(chan struct{})
		return s.healthMonitor
	}
	return nil
}

func (s *RunState) CloseHealthMonitor() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.healthMonitor != nil {
		close(s.healthMonitor)
		s.healthMonitor = nil
	}
}

func (s *RunState) cancelRestartManager() {
	s.mu.Lock()
	if s.restartManager != nil {
		s.restartManager.Cancel()
	}
	s.mu.Unlock()
}

func (s *RunState) resetLogging() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logDriver != nil {
		if s.logCopier != nil {
			exit := make(chan struct{})
			go func() {
				s.logCopier.Wait()
				close(exit)
			}()
			select {
			case <-time.After(loggerCloseTimeout):
				logrus.Warn("Logger didn't exit in time: logs may be truncated")
			case <-exit:
			}
		}
		s.logDriver.Close()
	}
	s.logCopier = nil
	s.logDriver = nil
}

// Reset puts a container into a state where it can be restarted again.
func (container *Container) Reset() {
	streams := container.Streams()

	if err := streams.CloseStreams(); err != nil {
		logrus.Errorf("%s: %s", container.ID, err)
	}

	// Re-create a brand new stdin pipe once the container exited
	if container.Config.OpenStdin {
		streams.NewInputPipes()
	}

	s := GetRunState(container)
	s.resetLogging()
}
