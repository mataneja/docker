package container

import "context"

// StoreFilter defines a function to filter
// container in the store.
type StoreFilter func(*Container) bool

// StoreReducer defines a function to
// manipulate containers in the store
type StoreReducer func(*Container)

// Store defines an interface that
// any container store must implement.
type Store interface {
	// Add appends a new container to the store.
	Add(string, *Container) error
	// Get returns a container from the store by the identifier it was stored with.
	Get(string) *Container
	// Delete removes a container from the store by the identifier it was stored with.
	Delete(string) error
	// List returns a list of containers from the store.
	List() []*Container
	// Size returns the number of containers in the store.
	Size() int
	// First returns the first container found in the store by a given filter.
	First(StoreFilter) *Container
	// ApplyAll calls the reducer function with every container in the store.
	ApplyAll(StoreReducer)
	// Commit commits a change to a container to the store
	// There is no need to call commit when using ApplyAll, this function is only
	// for syncing changes to an individual container with the container store
	Commit(*Container) error
	// WaitStop waits for the passed in container to stop and returns the updated container
	// TODO(cpuguy83): this feels a bit weird here.
	WaitStop(context.Context, *Container) (*Container, error)
	// WaitAttachStop waits for the passed in container to stop and returns the updated container
	// TODO(cpuguy83): this feels a bit weird here.
	WaitAttachStop(context.Context, *Container) (*Container, error)
}
