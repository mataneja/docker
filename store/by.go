package store

// By is an interface type passed to Find methods. Implementations must be
// defined in this package.
type By interface {
	// isBy allows this interface to only be satisfied by certain internal
	// types.
	isBy()
}

type byAll struct{}

func (a byAll) isBy() {
}

// All is an argument that can be passed to find to list all items in the
// set.
var All byAll

type byNamePrefix string

func (b byNamePrefix) isBy() {
}

// ByNamePrefix creates an object to pass to Find to select by query.
func ByNamePrefix(namePrefix string) By {
	return byNamePrefix(namePrefix)
}

type byIDPrefix string

func (b byIDPrefix) isBy() {
}

// ByIDPrefix creates an object to pass to Find to select by query.
func ByIDPrefix(idPrefix string) By {
	return byIDPrefix(idPrefix)
}

type byName string

func (b byName) isBy() {
}

// ByName creates an object to pass to Find to select by name.
func ByName(name string) By {
	return byName(name)
}

type byContainerID string

func (b byContainerID) isBy() {
}

func ByContainerID(containerID string) By {
	return byContainerID(containerID)
}
