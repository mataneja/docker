package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/daemon/exec"
)

func TestStoreExec(t *testing.T) {
	store := NewMemoryStore()

	e := &exec.Config{ID: "1234", ContainerID: "5678"}
	topics := []Event{
		EventExecCreate{Config: e, Checks: []ExecCheckFunc{MatchExecID}},
		EventExecUpdate{Config: e, Checks: []ExecCheckFunc{MatchExecID}},
		EventExecDelete{Config: e, Checks: []ExecCheckFunc{MatchExecID}},
	}
	chEvent := store.SubscribeEvents(topics...)

	err := store.Update(func(tx Tx) error {
		return CreateExec(tx, e)
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-chEvent:
		execEvent, ok := event.(EventExecCreate)
		if !ok {
			t.Fatalf("expected EvnetEventCreate, got: %v", reflect.TypeOf(event))
		}
		if execEvent.Config == nil || execEvent.Config.ID != e.ID {
			t.Fatalf("expected exec %v, got %v", e, execEvent.Config)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve exec create event")
	}

	var ee *exec.Config
	store.View(func(tx ReadTx) {
		ee = GetExec(tx, e.ID)
	})
	if ee == nil || ee.ID != e.ID || ee.ContainerID != e.ContainerID {
		t.Fatalf("expected exec %v, got: %v", e, ee)
	}

	var ls []*exec.Config
	store.View(func(tx ReadTx) {
		ls, err = FindExecs(tx, ByContainerID(e.ContainerID))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].ContainerID != e.ContainerID {
		t.Fatal("expected find by containerID to work: %v", ls)
	}

	err = store.Update(func(tx Tx) error {
		e.Running = true
		return UpdateExec(tx, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-chEvent:
		execEvent, ok := event.(EventExecUpdate)
		if !ok {
			t.Fatalf("expected EvnetEventUpdate, got: %v", reflect.TypeOf(event))
		}
		if execEvent.Config == nil || execEvent.Config.ID != e.ID {
			t.Fatalf("expected exec %v, got %v", e, execEvent.Config)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve exec create event")
	}

	store.View(func(tx ReadTx) {
		ee = GetExec(tx, e.ID)
	})
	if !ee.Running {
		t.Fatal("expected exec to be in running state")
	}

	err = store.Update(func(tx Tx) error {
		return DeleteExec(tx, e.ID)
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-chEvent:
		execEvent, ok := event.(EventExecDelete)
		if !ok {
			t.Fatalf("expected EvnetEventDelete, got: %v", reflect.TypeOf(event))
		}
		if execEvent.Config == nil || execEvent.Config.ID != e.ID {
			t.Fatalf("expected exec %v, got %v", e, execEvent.Config)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve exec create event")
	}

	store.View(func(tx ReadTx) {
		ee = GetExec(tx, e.ID)
	})
	if ee != nil {
		t.Fatal("expected exec to be deleted")
	}
}
