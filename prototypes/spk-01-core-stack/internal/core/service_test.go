package core_test

import (
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/core"
	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/store"
)

func TestProjectCuePublishGoPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	bus := core.NewBus()
	svc, err := core.NewService(store.File{Path: path}, bus)
	if err != nil { t.Fatal(err) }
	p, err := svc.CreateProject("Demo Show")
	if err != nil { t.Fatal(err) }
	if _, err := svc.AddCue(p.ID, "1", "Intro", "hello"); err != nil { t.Fatal(err) }
	snap, err := svc.Publish(p.ID)
	if err != nil { t.Fatal(err) }
	exec, err := svc.Go(p.ID)
	if err != nil { t.Fatal(err) }
	if exec.Status != "COMPLETED" || exec.SnapshotID != snap.ID { t.Fatalf("unexpected execution: %+v", exec) }

	restarted, err := core.NewService(store.File{Path: path}, core.NewBus())
	if err != nil { t.Fatal(err) }
	st := restarted.State()
	rp := st.Projects[p.ID]
	if rp == nil { t.Fatal("project missing after restart") }
	if rp.Published == nil || rp.Published.ID != snap.ID { t.Fatal("snapshot missing after restart") }
	if len(rp.Executions) != 1 || rp.Executions[0].ID != exec.ID { t.Fatalf("history mismatch: %+v", rp.Executions) }
}

func TestGoPublishesTraceEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	bus := core.NewBus()
	ch, cancel := bus.Subscribe(8)
	defer cancel()
	svc, _ := core.NewService(store.File{Path: path}, bus)
	p, _ := svc.CreateProject("Demo")
	_, _ = svc.AddCue(p.ID, "1", "Intro", "hello")
	_, _ = svc.Publish(p.ID)
	<-ch
	if _, err := svc.Go(p.ID); err != nil { t.Fatal(err) }
	want := []string{"cue.started", "action.completed", "cue.completed"}
	for _, typ := range want {
		evt := <-ch
		if evt.Type != typ { t.Fatalf("want %s, got %s", typ, evt.Type) }
	}
}
