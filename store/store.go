package store

import (
	"fmt"
	"reflect"

	"github.com/docker/docker/pkg/pubsub"
	memdb "github.com/hashicorp/go-memdb"
)

const (
	indexID   = "id"
	indexName = "name"
	prefix    = "_prefix"
)

// Store is an interface for implementing a genric, transactional, object store
type Store interface {
	View(func(ReadTx))
	Update(func(Tx) error) error
	Batch(func(*Batch) error) (int, error)
	SubscribeEvents(...Event) <-chan interface{}
	Close()
}

// EventCommit is an `Event` that is emitted when a commit is made to the store
type EventCommit struct{}

func (EventCommit) matches(e Event) bool {
	_, ok := e.(EventCommit)
	return ok
}

// MemoryStore is a concurrency-safe, in-memory object store
type MemoryStore struct {
	db        *memdb.MemDB
	publisher *pubsub.Publisher
}

// NewMemoryStore returns an in-memory store
func NewMemoryStore() *MemoryStore {
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		panic(err)
	}
	return &MemoryStore{
		db:        db,
		publisher: pubsub.NewPublisher(0, 10),
	}
}

// Close closes the memory store and frees its associated resources.
func (s *MemoryStore) Close() {
	s.publisher.Close()
}

// View executes a read transaction.
func (s *MemoryStore) View(cb func(ReadTx)) {
	memDBTx := s.db.Txn(false)

	readTx := readTx{
		memDBTx: memDBTx,
	}
	cb(readTx)
	memDBTx.Commit()
}

// Update executes a read/write transaction
func (s *MemoryStore) Update(cb func(Tx) error) error {
	memDBTx := s.db.Txn(true)

	var tx tx
	tx.init(memDBTx)

	err := cb(&tx)
	if err == nil {
		memDBTx.Commit()
		for _, c := range tx.changeList {
			s.publisher.Publish(c)
		}
		if len(tx.changeList) != 0 {
			s.publisher.Publish(EventCommit{})
		}
	} else {
		memDBTx.Abort()
	}
	return err
}

// SubscribeEvents returns the publish/subscribe queue.
func (s *MemoryStore) SubscribeEvents(watchEvents ...Event) <-chan interface{} {
	topic := func(i interface{}) bool {
		observed, ok := i.(Event)
		if !ok {
			panic(fmt.Sprintf("unexpected type passed to event channel: %v", reflect.TypeOf(i)))
		}
		for _, e := range watchEvents {
			if e.matches(observed) {
				return true
			}
		}
		// If no specific events are specified always assume a matched event
		// If some events were specified and none matched above, then the event
		// doesn't match
		return watchEvents == nil
	}
	return s.publisher.SubscribeTopic(topic)
}

func prefixFromArgs(args ...interface{}) ([]byte, error) {
	val, err := fromArgs(args...)
	if err != nil {
		return nil, err
	}

	// Strip the null terminator, the rest is a prefix
	n := len(val)
	if n > 0 {
		return val[:n-1], nil
	}
	return val, nil
}

func fromArgs(args ...interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("must provide only a single argument")
	}
	arg, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("argument must be a string: %#v", args[0])
	}
	// Add the null character as a terminator
	arg += "\x00"
	return []byte(arg), nil
}
