package store

import "errors"

// Errors which the store can return
var (
	ErrExist         = errors.New("object exists")
	ErrNotExist      = errors.New("object does not exist")
	ErrNameConflict  = errors.New("name is in use")
	ErrInvalidFindBy = errors.New("invalid find by")
	// ErrSequenceConflict is returned when during update the version set on the
	// the object does not match the version in the store.
	// This can happen if the object was changed in between the caller getting the
	// object and then updating that object.
	ErrSequenceConflict = errors.New("update out of sequence")
)
