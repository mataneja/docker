package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/container"
)

func TestStoreContainer(t *testing.T) {
	store := NewMemoryStore()

	c := &container.Container{}
	c.ID = "1a2bc3"
	c.State = container.NewState()
	c.SetRunning(1, true)
	c.Name = "hello"

	topics := []Event{
		EventContainerCreate{Container: c, Checks: []ContainerCheckFunc{MatchContainerID}},
		EventContainerUpdate{Container: c, Checks: []ContainerCheckFunc{MatchContainerID}},
		EventContainerDelete{Container: c, Checks: []ContainerCheckFunc{MatchContainerID}},
	}
	chEvent := store.SubscribeEvents(topics...)

	// test container create
	err := store.Update(func(tx Tx) error {
		return CreateContainer(tx, c)
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-chEvent:
		ce, ok := e.(EventContainerCreate)
		if !ok {
			t.Fatalf("expected EvnetContainerCreate, got: %v", reflect.TypeOf(e))
		}
		if ce.Container == nil || ce.Container.ID != c.ID {
			t.Fatalf("expected container %v, got %v", c, ce.Container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve container create event")
	}

	var cc *container.Container
	store.View(func(tx ReadTx) {
		cc = GetContainer(tx, c.ID)
	})
	if cc == nil || cc.ID != c.ID {
		t.Fatalf("expected container %v, got: %v", c, cc)
	}
	if !cc.IsRunning() {
		t.Fatalf("expected container in running state, got: %s", cc.State.String())
	}

	// test find by ID prefix
	var ls []*container.Container
	store.View(func(tx ReadTx) {
		ls, err = FindContainers(tx, ByIDPrefix(c.ID[:2]))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) == 0 || ls[0].ID != c.ID {
		t.Fatal("expected get by ID prefix to work")
	}

	// Now test fnding by name prefix
	store.View(func(tx ReadTx) {
		ls, err = FindContainers(tx, ByNamePrefix(c.Name[:3]))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) == 0 || ls[0].Name != c.Name {
		t.Fatalf("expected get by name prefix to work: %v", ls)
	}

	// test update the container state and store it
	err = store.Update(func(tx Tx) error {
		c.SetStopped(&container.ExitStatus{ExitCode: 1})
		return UpdateContainer(tx, c)
	})
	if err != nil {
		t.Fatal(err)
	}
	store.View(func(tx ReadTx) {
		cc = GetContainer(tx, c.ID)
	})
	// container should be seen as stopped after store update
	if cc.IsRunning() {
		t.Fatal("expected container to not be running")
	}

	select {
	case e := <-chEvent:
		ce, ok := e.(EventContainerUpdate)
		if !ok {
			t.Fatalf("expected EvnetContainerUpdate, got: %v", reflect.TypeOf(e))
		}
		if ce.Container == nil || ce.Container.ID != c.ID {
			t.Fatalf("expected container %v, got %v", c, ce.Container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve container update event")
	}

	// Test out of sequence updates
	err = store.Update(func(tx Tx) error {
		c := GetContainer(tx, c.ID)
		if c == nil {
			t.Fatal("could not find container")
		}
		c.CurrentVersion = c.CurrentVersion + 1
		return UpdateContainer(tx, c)
	})
	if err != ErrSequenceConflict {
		t.Fatalf("expected ErrSequenceConflict, got: %v", err)
	}

	// Test container delete
	err = store.Update(func(tx Tx) error {
		return DeleteContainer(tx, c.ID)
	})
	if err != nil {
		t.Fatal(err)
	}

	store.View(func(tx ReadTx) {
		cc = GetContainer(tx, c.ID)
	})
	if cc != nil {
		t.Fatal("container should not exist")
	}

	select {
	case e := <-chEvent:
		ce, ok := e.(EventContainerDelete)
		if !ok {
			t.Fatalf("expected EvnetContainerDelete, got: %v", reflect.TypeOf(e))
		}
		if ce.Container == nil || ce.Container.ID != c.ID {
			t.Fatalf("expected container %v, got %v", c, ce.Container)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not recieve container delete event")
	}

}
