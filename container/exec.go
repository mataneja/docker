package container

import (
	"sync"

	"github.com/docker/docker/pkg/stringid"
)

// Config holds the configurations for execs. The Daemon keeps
// track of both running and finished execs so that they can be
// examined both during and after completion.
type ExecConfig struct {
	sync.Mutex
	*StreamConfig
	ID          string
	Running     bool
	ExitCode    *int
	OpenStdin   bool
	OpenStderr  bool
	OpenStdout  bool
	CanRemove   bool
	ContainerID string
	DetachKeys  []byte
	Entrypoint  string
	Args        []string
	Tty         bool
	Privileged  bool
	User        string
	Env         []string
}

// NewExecConfig initializes the a new exec configuration
func NewExecConfig() *ExecConfig {
	return &ExecConfig{
		ID:           stringid.GenerateNonCryptoID(),
		StreamConfig: NewStreamConfig(),
	}
}

// Store keeps track of the exec configurations.
type ExecStore struct {
	commands map[string]*ExecConfig
	sync.RWMutex
}

// NewStore initializes a new exec store.
func NewExecStore() *ExecStore {
	return &ExecStore{commands: make(map[string]*ExecConfig, 0)}
}

// Commands returns the exec configurations in the store.
func (e *ExecStore) Commands() map[string]*ExecConfig {
	e.RLock()
	commands := make(map[string]*ExecConfig, len(e.commands))
	for id, config := range e.commands {
		commands[id] = config
	}
	e.RUnlock()
	return commands
}

// Add adds a new exec configuration to the store.
func (e *ExecStore) Add(id string, Config *ExecConfig) {
	e.Lock()
	e.commands[id] = Config
	e.Unlock()
}

// Get returns an exec configuration by its id.
func (e *ExecStore) Get(id string) *ExecConfig {
	e.RLock()
	res := e.commands[id]
	e.RUnlock()
	return res
}

// Delete removes an exec configuration from the store.
func (e *ExecStore) Delete(id string) {
	e.Lock()
	delete(e.commands, id)
	e.Unlock()
}

// List returns the list of exec ids in the store.
func (e *ExecStore) List() []string {
	var IDs []string
	e.RLock()
	for id := range e.commands {
		IDs = append(IDs, id)
	}
	e.RUnlock()
	return IDs
}
