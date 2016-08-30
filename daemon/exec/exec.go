package exec

import (
	"runtime"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container/stream"
	"github.com/docker/docker/libcontainerd"
	"github.com/docker/docker/pkg/stringid"
)

type Store interface {
	Commands() []*Config
	CommandsByContainerID(id string) []*Config
	Add(*Config) error
	Get(id string) *Config
	Commit(*Config) error
	Delete(id string) error
	List(containerID string) []string
}

// Config holds the configurations for execs. The Daemon keeps
// track of both running and finished execs so that they can be
// examined both during and after completion.
type Config struct {
	sync.Mutex
	StreamConfig *stream.Config
	ID           string
	Running      bool
	ExitCode     *int
	OpenStdin    bool
	OpenStderr   bool
	OpenStdout   bool
	CanRemove    bool
	ContainerID  string
	DetachKeys   []byte
	Entrypoint   string
	Args         []string
	Tty          bool
	Privileged   bool
	User         string
	Env          []string
	Pid          int
}

// NewConfig initializes the a new exec configuration
func NewConfig() *Config {
	return &Config{
		ID:           stringid.GenerateNonCryptoID(),
		StreamConfig: stream.NewConfig(),
	}
}

func (c *Config) Copy() *Config {
	var copy Config
	copy = *c
	if c.ExitCode != nil {
		var ec int
		ec = *c.ExitCode
		copy.ExitCode = &ec
	}
	return &copy
}

// InitializeStdio is called by libcontainerd to connect the stdio.
func (c *Config) InitializeStdio(iop libcontainerd.IOPipe) error {
	c.StreamConfig.CopyToPipe(iop)

	if c.StreamConfig.Stdin() == nil && !c.Tty && runtime.GOOS == "windows" {
		if iop.Stdin != nil {
			if err := iop.Stdin.Close(); err != nil {
				logrus.Errorf("error closing exec stdin: %+v", err)
			}
		}
	}

	return nil
}

// CloseStreams closes the stdio streams for the exec
func (c *Config) CloseStreams() error {
	return c.StreamConfig.CloseStreams()
}
