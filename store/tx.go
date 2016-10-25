package store

import (
	"strings"

	"github.com/docker/swarmkit/manager/state"
	memdb "github.com/hashicorp/go-memdb"
)

// ReadTx is a read transaction. Note that transaction does not imply
// any internal batching. It only means that the transaction presents a
// consistent view of the data that cannot be affected by other
// transactions.
type ReadTx interface {
	lookup(table, index, id string) Object
	get(table, id string) Object
	find(table string, by By, checkType func(By) error, appendResult func(Object)) error
}

type readTx struct {
	memDBTx *memdb.Txn
}

// Tx is a read/write transaction. Note that transaction does not imply
// any internal batching. The purpose of this transaction is to give the
// user a guarantee that its changes won't be visible to other transactions
// until the transaction is over.
type Tx interface {
	ReadTx
	create(table string, o Object) error
	update(table string, o Object) error
	delete(table, id string) error
}

type tx struct {
	readTx
	//curVersion *api.Version
	changeList []Event
}

func (tx *tx) init(memDBTx *memdb.Txn) {
	tx.memDBTx = memDBTx
	tx.changeList = nil
}

// lookup is an internal typed wrapper around memdb.
func (tx readTx) lookup(table, index, id string) Object {
	j, err := tx.memDBTx.First(table, index, id)
	if err != nil {
		return nil
	}
	if j != nil {
		return j.(Object)
	}
	return nil
}

// create adds a new object to the store.
// Returns ErrExist if the ID is already taken.
func (tx *tx) create(table string, o Object) error {
	if tx.lookup(table, indexID, o.ID()) != nil {
		return ErrExist
	}

	copy := o.Copy()
	err := tx.memDBTx.Insert(table, copy)
	if err == nil {
		tx.changeList = append(tx.changeList, copy.EventCreate())
	}
	return err
}

// Update updates an existing object in the store.
// Returns ErrNotExist if the object doesn't exist.
func (tx *tx) update(table string, o Object) error {
	oldN := tx.lookup(table, indexID, o.ID())
	if oldN == nil {
		return ErrNotExist
	}

	if o.GetVersion() != oldN.GetVersion() {
		return ErrSequenceConflict
	}
	copy := o.Copy()
	copy.SetVersion(copy.GetVersion() + 1)
	err := tx.memDBTx.Insert(table, copy)
	if err == nil {
		tx.changeList = append(tx.changeList, copy.EventUpdate())
		o.SetVersion(copy.GetVersion())
	}
	return err
}

// Delete removes an object from the store.
// Returns ErrNotExist if the object doesn't exist.
func (tx *tx) delete(table, id string) error {
	n := tx.lookup(table, indexID, id)
	if n == nil {
		return ErrNotExist
	}

	err := tx.memDBTx.Delete(table, n)
	if err == nil {
		tx.changeList = append(tx.changeList, n.EventDelete())
	}
	return err
}

// Get looks up an object by ID.
// Returns nil if the object doesn't exist.
func (tx readTx) get(table, id string) Object {
	o := tx.lookup(table, indexID, id)
	if o == nil {
		return nil
	}
	return o.Copy()
}

// findIterators returns a slice of iterators. The union of items from these
// iterators provides the result of the query.
func (tx readTx) findIterators(table string, by By, checkType func(By) error) ([]memdb.ResultIterator, error) {
	switch by.(type) {
	case byAll, orCombinator: // generic types
	default: // all other types
		if err := checkType(by); err != nil {
			return nil, err
		}
	}

	switch v := by.(type) {
	case byAll:
		it, err := tx.memDBTx.Get(table, indexID)
		if err != nil {
			return nil, err
		}
		return []memdb.ResultIterator{it}, nil
	case orCombinator:
		var iters []memdb.ResultIterator
		for _, subBy := range v.bys {
			it, err := tx.findIterators(table, subBy, checkType)
			if err != nil {
				return nil, err
			}
			iters = append(iters, it...)
		}
		return iters, nil
	case byName:
		it, err := tx.memDBTx.Get(table, indexName, strings.ToLower(string(v)))
		if err != nil {
			return nil, err
		}
		return []memdb.ResultIterator{it}, nil
	case byIDPrefix:
		it, err := tx.memDBTx.Get(table, indexID+prefix, string(v))
		if err != nil {
			return nil, err
		}
		return []memdb.ResultIterator{it}, nil
	case byNamePrefix:
		it, err := tx.memDBTx.Get(table, indexName+prefix, strings.ToLower(string(v)))
		if err != nil {
			return nil, err
		}
		return []memdb.ResultIterator{it}, nil
	case byContainerID:
		it, err := tx.memDBTx.Get(table, indexContainerID, strings.ToLower(string(v)))
		if err != nil {
			return nil, err
		}
		return []memdb.ResultIterator{it}, nil
	default:
		return nil, ErrInvalidFindBy
	}
}

// find selects a set of objects calls a callback for each matching object.
func (tx readTx) find(table string, by By, checkType func(By) error, appendResult func(Object)) error {
	fromResultIterators := func(its ...memdb.ResultIterator) {
		ids := make(map[string]struct{})
		for _, it := range its {
			for {
				obj := it.Next()
				if obj == nil {
					break
				}
				o := obj.(Object)
				id := o.ID()
				if _, exists := ids[id]; !exists {
					appendResult(o.Copy())
					ids[id] = struct{}{}
				}
			}
		}
	}

	iters, err := tx.findIterators(table, by, checkType)
	if err != nil {
		return err
	}

	fromResultIterators(iters...)
	return nil
}

// Batch provides a mechanism to batch updates to a store.
type Batch struct {
	tx    tx
	store *MemoryStore
	// applied counts the times Update has run successfully
	applied int
	// committed is the number of times Update had run successfully as of
	// the time pending changes were committed.
	committed int
	err       error
}

func (batch *Batch) newTx() {
	batch.tx.init(batch.store.db.Txn(true))
}

// Update adds a single change to a batch. Each call to Update is atomic, but
// different calls to Update may be spread across multiple transactions to
// circumvent transaction size limits.
func (batch *Batch) Update(cb func(Tx) error) error {
	if batch.err != nil {
		return batch.err
	}

	if err := cb(&batch.tx); err != nil {
		return err
	}

	batch.applied++
	return nil
}

func (batch *Batch) commit() error {
	batch.tx.memDBTx.Commit()

	if batch.err != nil {
		batch.tx.memDBTx.Abort()
		return batch.err
	}

	batch.committed = batch.applied

	for _, c := range batch.tx.changeList {
		batch.store.publisher.Publish(c)
	}
	if len(batch.tx.changeList) != 0 {
		batch.store.publisher.Publish(state.EventCommit{})
	}
	return nil
}

// Batch performs one or more transactions that allow reads and writes
// It invokes a callback that is passed a Batch object. The callback may
// call batch.Update for each change it wants to make as part of the
// batch. The changes in the batch may be split over multiple
// transactions if necessary to keep transactions below the size limit.
// Batch holds a lock over the state, but will yield this lock every
// it creates a new transaction to allow other writers to proceed.
// Thus, unrelated changes to the state may occur between calls to
// batch.Update.
//
// This method allows the caller to iterate over a data set and apply
// changes in sequence without holding the store write lock for an
// excessive time
//
// Batch returns the number of calls to batch.Update whose changes were
// successfully committed to the store.
func (s *MemoryStore) Batch(cb func(*Batch) error) (int, error) {
	batch := Batch{
		store: s,
	}
	batch.newTx()

	if err := cb(&batch); err != nil {
		batch.tx.memDBTx.Abort()
		return batch.committed, err
	}

	err := batch.commit()
	return batch.committed, err
}
