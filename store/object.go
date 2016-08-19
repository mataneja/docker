package store

import memdb "github.com/hashicorp/go-memdb"

var (
	objectStorers []ObjectStoreConfig
	schema        = &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{},
	}
)

// Event is an interface to that gets passed around by the store when certain
// events occur, such as create,update,destroy.
type Event interface {
	matches(Event) bool
}

func register(os ObjectStoreConfig) {
	objectStorers = append(objectStorers, os)
	schema.Tables[os.Name] = os.Table
}

// Object is a generic object that can be handled by the store.
type Object interface {
	ID() string         // Get ID
	Copy() Object       // Return a copy of this object
	EventCreate() Event // Return a creation event
	EventUpdate() Event // Return an update event
	EventDelete() Event // Return a deletion event

	// TODO(cpuguy83): versions should be strongly typed, and probably more closely align to
	// swarm's "Meta" object
	GetVersion() uint64
	SetVersion(ver uint64)
}

// ObjectStoreConfig provides the necessary methods to store a particular object
// type inside MemoryStore.
type ObjectStoreConfig struct {
	Name  string
	Table *memdb.TableSchema
}
