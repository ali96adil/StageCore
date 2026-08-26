package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestEventJournalSequencePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil { t.Fatal(err) }
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Journal Show"})
	if err != nil { t.Fatal(err) }
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, Name: "Cue", OrderIndex: 0, Enabled: true}, nil); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "journal")
	if err != nil { t.Fatal(err) }
	appendEvent := func(target *store.Store, eventType string) contracts.EventEnvelope {
		event, err := target.AppendEvent(ctx, &session.ID, contracts.EventEnvelope{EventType: eventType, SchemaVersion: 1, Source: "test", ProjectID: runtimeSnapshot.ProjectID, RuntimeSnapshotID: runtimeSnapshot.ID, Priority: "P1", TraceContext: json.RawMessage(`{"trace_id":"abc"}`), Payload: json.RawMessage(`{}`)})
		if err != nil { t.Fatal(err) }
		return event
	}
	first := appendEvent(s, "test.first")
	second := appendEvent(s, "test.second")
	if first.Sequence != 1 || second.Sequence != 2 { t.Fatalf("initial sequences %d %d", first.Sequence, second.Sequence) }
	if err := h.Close(); err != nil { t.Fatal(err) }
	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	s = store.New(h.DB, clock.Fixed{Time: fixedTime})
	third := appendEvent(s, "test.third")
	if third.Sequence != 3 { t.Fatalf("sequence after reopen=%d want 3", third.Sequence) }
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if len(events) != 3 || string(events[2].TraceContext) != `{"trace_id":"abc"}` { t.Fatalf("events=%#v", events) }
}
