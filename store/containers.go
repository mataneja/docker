package store

import (
	"strings"

	"src/github.com/pkg/errors"

	"github.com/docker/docker/container"
	memdb "github.com/hashicorp/go-memdb"
)

const tableContainer = "container"

func init() {
	register(ObjectStoreConfig{
		Name: tableContainer,
		Table: &memdb.TableSchema{
			Name: tableContainer,
			Indexes: map[string]*memdb.IndexSchema{
				indexID: {
					Name:    indexID,
					Unique:  true,
					Indexer: containerIndexerByID{},
				},
				indexName: {
					Name:    indexName,
					Unique:  true,
					Indexer: containerIndexerByName{},
				},
			},
		},
	})
}

type containerEntry struct {
	*container.Container
}

func (c containerEntry) ID() string {
	return c.Container.ID
}

func (c containerEntry) Copy() Object {
	return containerEntry{c.Container.Copy()}
}

func (c containerEntry) EventCreate() Event {
	return EventContainerCreate{
		Container: c.Container,
	}
}

func (c containerEntry) EventUpdate() Event {
	return EventContainerUpdate{
		Container: c.Container,
	}
}

func (c containerEntry) EventDelete() Event {
	return EventContainerDelete{
		Container: c.Container,
	}
}

func (c containerEntry) GetVersion() uint64 {
	return c.Container.CurrentVersion
}

func (c containerEntry) SetVersion(ver uint64) {
	c.Container.CurrentVersion = ver
}

type containerIndexerByID struct{}

func (containerIndexerByID) FromArgs(args ...interface{}) ([]byte, error) {
	return fromArgs(args...)
}

func (containerIndexerByID) FromObject(obj interface{}) (bool, []byte, error) {
	c, ok := obj.(containerEntry)
	if !ok {
		panic("unexpected type passed to FromObject")
	}

	// Add the null character as a terminator
	val := c.Container.ID + "\x00"
	return true, []byte(val), nil
}

func (containerIndexerByID) PrefixFromArgs(args ...interface{}) ([]byte, error) {
	return prefixFromArgs(args...)
}

type containerIndexerByName struct{}

func (containerIndexerByName) FromArgs(args ...interface{}) ([]byte, error) {
	return fromArgs(args...)
}

func (containerIndexerByName) FromObject(obj interface{}) (bool, []byte, error) {
	c, ok := obj.(containerEntry)
	if !ok {
		panic("unexpected type passed to FromObject")
	}

	// Add the null character as a terminator
	val := strings.ToLower(c.Container.Name) + "\x00"
	return true, []byte(val), nil
}

func (containerIndexerByName) PrefixFromArgs(args ...interface{}) ([]byte, error) {
	return prefixFromArgs(args...)
}

// CreateContainer creates a new container in the store
func CreateContainer(tx Tx, c *container.Container) error {
	if tx.lookup(tableContainer, indexName, strings.ToLower(c.Name)) != nil {
		return ErrNameConflict
	}
	return errors.Wrap(tx.create(tableContainer, containerEntry{c}), "could not create container in store")
}

// UpdateContainer updates an existing container in the store
// Returns ErrNotExists if container does not exist in the store
func UpdateContainer(tx Tx, c *container.Container) error {
	if existing := tx.lookup(tableContainer, indexName, strings.ToLower(c.Name)); existing != nil {
		if existing.ID() != c.ID {
			return ErrNameConflict
		}
	}
	update := containerEntry{c}
	err := tx.update(tableContainer, update)
	return errors.Wrap(err, "error saving container update to store")
}

// DeleteContainer removes a container from the store
// Returns ErrNotExist if the container does not exist in the store
func DeleteContainer(tx Tx, id string) error {
	return errors.Wrap(tx.delete(tableContainer, id), "error deleting container from store")
}

// GetContainer looks up a container by the passed in ID or name.
// Returns nil if the container does not exist in the store.
func GetContainer(tx ReadTx, id string) *container.Container {
	c := tx.get(tableContainer, id)
	if c != nil {
		return c.(containerEntry).Container
	}

	name := id
	// TODO(@cpuguy83): this usage of `/` in front of names should be cleaned up
	// in daemon
	if name[0] != '/' {
		name = "/" + name
	}
	c = tx.lookup(tableContainer, indexName, name)
	if c != nil {
		return c.(containerEntry).Container
	}
	c = tx.lookup(tableContainer, indexID+prefix, id)
	if c != nil {
		return c.(containerEntry).Container
	}
	c = tx.lookup(tableContainer, indexName+prefix, name)
	if c != nil {
		return c.(containerEntry).Container
	}
	return nil
}

// FindContainers finds a set of containers in the store
func FindContainers(tx ReadTx, by By) ([]*container.Container, error) {
	checkType := func(by By) error {
		switch by.(type) {
		case byName, byNamePrefix, byIDPrefix:
			return nil
		default:
			return ErrInvalidFindBy
		}
	}

	containerList := []*container.Container{}
	appendResult := func(o Object) {
		containerList = append(containerList, o.(containerEntry).Container)
	}

	err := tx.find(tableContainer, by, checkType, appendResult)
	return containerList, errors.Wrap(err, "error finding containers")
}

// ContainerCheckFunc is the type of function used to perform filtering checks
// on containers
type ContainerCheckFunc func(j1, j2 *container.Container) bool

// MatchContainerID is a ContainerCheckFunc that maches based on container ID
func MatchContainerID(i, j *container.Container) bool {
	if i == j {
		return true
	}
	if i == nil || j == nil {
		return false
	}
	return i.ID == j.ID
}

// EventContainerCreate is an `Event` that is emitted when a new container is
// created in the store.
type EventContainerCreate struct {
	Container *container.Container
	// Checks is a list of functions to call to filter events for a watch
	// stream. They are applied with AND logic.
	Checks []ContainerCheckFunc
}

func (e EventContainerCreate) matches(watchedEvent Event) bool {
	typedEvent, ok := watchedEvent.(EventContainerCreate)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Container, typedEvent.Container) {
			return false
		}
	}
	return true
}

// EventContainerUpdate is an `Event` that is emitted when a container is
// updated in the store.
type EventContainerUpdate struct {
	Container *container.Container
	// Checks is a list of functions to call to filter events for a watch
	// stream. They are applied with AND logic.
	Checks []ContainerCheckFunc
}

func (e EventContainerUpdate) matches(watchedEvent Event) bool {
	typedEvent, ok := watchedEvent.(EventContainerUpdate)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Container, typedEvent.Container) {
			return false
		}
	}
	return true
}

// EventContainerDelete is an `Event` that is emitted when a container is
// updated in the store.
type EventContainerDelete struct {
	Container *container.Container
	// Checks is a list of functions to call to filter events for a watch
	// stream. They are applied with AND logic.
	Checks []ContainerCheckFunc
}

func (e EventContainerDelete) matches(watchedEvent Event) bool {
	typedEvent, ok := watchedEvent.(EventContainerDelete)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Container, typedEvent.Container) {
			return false
		}
	}
	return true
}
