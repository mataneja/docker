package store

import (
	"github.com/docker/docker/daemon/exec"
	memdb "github.com/hashicorp/go-memdb"
)

const (
	tableExec        = "exec"
	indexContainerID = "container_id"
)

func init() {
	register(ObjectStoreConfig{
		Name: tableExec,
		Table: &memdb.TableSchema{
			Name: tableExec,
			Indexes: map[string]*memdb.IndexSchema{
				indexID: {
					Name:    indexID,
					Unique:  true,
					Indexer: execIndexerByID{},
				},
				indexContainerID: {
					Name:    indexContainerID,
					Indexer: execIndexerByContainerID{},
				},
			},
		},
	})
}

type execEntry struct {
	exec *exec.Config
}

func (e execEntry) ID() string {
	return e.exec.ID
}

func (e execEntry) Copy() Object {
	return execEntry{e.exec.Copy()}
}

func (e execEntry) EventCreate() Event {
	return EventExecCreate{
		Config: e.exec,
	}
}

func (e execEntry) EventUpdate() Event {
	return EventExecUpdate{
		Config: e.exec,
	}
}

func (e execEntry) EventDelete() Event {
	return EventExecDelete{
		Config: e.exec,
	}
}

func (e execEntry) GetVersion() uint64 {
	return uint64(0)
}

func (e execEntry) SetVersion(ver uint64) {

}

type execIndexerByID struct{}

func (execIndexerByID) FromArgs(args ...interface{}) ([]byte, error) {
	return fromArgs(args...)
}

func (execIndexerByID) FromObject(obj interface{}) (bool, []byte, error) {
	e, ok := obj.(execEntry)
	if !ok {
		panic("unexpected type passed to FromObject")
	}

	// Add the null character as a terminator
	val := e.exec.ID + "\x00"
	return true, []byte(val), nil
}

type execIndexerByContainerID struct{}

func (execIndexerByContainerID) FromArgs(args ...interface{}) ([]byte, error) {
	return fromArgs(args...)
}

func (execIndexerByContainerID) FromObject(obj interface{}) (bool, []byte, error) {
	e, ok := obj.(execEntry)
	if !ok {
		panic("unexpected type passed to FromObject")
	}

	// Add the null character as a terminator
	val := e.exec.ContainerID + "\x00"
	return true, []byte(val), nil
}

func CreateExec(tx Tx, e *exec.Config) error {
	return tx.create(tableExec, execEntry{e})
}

func UpdateExec(tx Tx, e *exec.Config) error {
	return tx.update(tableExec, execEntry{e})
}

func DeleteExec(tx Tx, id string) error {
	return tx.delete(tableExec, id)
}

func GetExec(tx ReadTx, id string) *exec.Config {
	e := tx.get(tableExec, id)
	if e != nil {
		return e.(execEntry).exec
	}
	return nil
}

func FindExecs(tx ReadTx, by By) ([]*exec.Config, error) {
	checkType := func(by By) error {
		switch by.(type) {
		case byContainerID, byAll:
			return nil
		default:
			return ErrInvalidFindBy
		}
	}

	execList := []*exec.Config{}
	appendResult := func(o Object) {
		execList = append(execList, o.(execEntry).exec)
	}
	err := tx.find(tableExec, by, checkType, appendResult)
	return execList, err
}

type ExecCheckFunc func(i, j *exec.Config) bool

func MatchExecID(i, j *exec.Config) bool {
	if i == j {
		return true
	}
	if i == nil || j == nil {
		return false
	}
	return i.ID == j.ID
}

type EventExecCreate struct {
	Config *exec.Config
	Checks []ExecCheckFunc
}

func (e EventExecCreate) matches(watchedEvent Event) (ret bool) {
	typedEvent, ok := watchedEvent.(EventExecCreate)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Config, typedEvent.Config) {
			return false
		}
	}
	return true
}

type EventExecUpdate struct {
	Config *exec.Config
	Checks []ExecCheckFunc
}

func (e EventExecUpdate) matches(watchedEvent Event) bool {
	typedEvent, ok := watchedEvent.(EventExecUpdate)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Config, typedEvent.Config) {
			return false
		}
	}
	return true
}

type EventExecDelete struct {
	Config *exec.Config
	Checks []ExecCheckFunc
}

func (e EventExecDelete) matches(watchedEvent Event) bool {
	typedEvent, ok := watchedEvent.(EventExecDelete)
	if !ok {
		return false
	}
	for _, check := range e.Checks {
		if !check(e.Config, typedEvent.Config) {
			return false
		}
	}
	return true
}
