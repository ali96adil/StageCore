package store_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestListProjectDeviceTransportEventsIsScopedOrderedAndBounded(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	now := time.Date(2026, 11, 14, 22, 0, 0, 0, time.UTC)
	s := store.New(h.DB, clock.Fixed{Time: now})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Transport events", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Other transport events", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	planRows, err := h.DB.QueryContext(ctx, `EXPLAIN QUERY PLAN
		SELECT event_id FROM event_records
		WHERE session_id IS NULL AND source_ref = 'hub.stage_device_runtime' AND project_id = ?
		  AND event_type IN ('stage_device.transport.connected', 'stage_device.transport.disconnected')
		  AND occurred_at_us >= ? AND occurred_at_us <= ?
		ORDER BY occurred_at_us, sequence LIMIT 501
	`, project.ID, now.UnixMicro(), now.Add(time.Second).UnixMicro())
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for planRows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := planRows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
	}
	if err := planRows.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "event_records_project_transport_timeline_idx") {
		t.Fatalf("project transport query does not use its bounded timeline index: %s", plan.String())
	}
	appendEvent := func(projectID, eventType string, at time.Time, payload string) {
		t.Helper()
		if _, err := s.AppendEvent(ctx, nil, contracts.EventEnvelope{
			EventType: eventType, SchemaVersion: contracts.SchemaVersion1,
			OccurredAt: at, Source: "hub.stage_device_runtime", ProjectID: projectID, Priority: "P2",
			Payload: json.RawMessage(payload),
		}); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(project.ID, contracts.StageDeviceTransportDisconnectedEventType, now.Add(time.Second), `{"device_id":"later"}`)
	appendEvent(project.ID, "stage_device.command.failed", now, `{"device_id":"command"}`)
	appendEvent(otherProject.ID, contracts.StageDeviceTransportConnectedEventType, now, `{"device_id":"other"}`)
	appendEvent(project.ID, contracts.StageDeviceTransportConnectedEventType, now, `{"device_id":"first"}`)
	if _, err := s.AppendEvent(ctx, nil, contracts.EventEnvelope{
		EventType: contracts.StageDeviceTransportConnectedEventType, SchemaVersion: contracts.SchemaVersion1,
		OccurredAt: now, Source: "adapter.untrusted", ProjectID: project.ID, Priority: "P2",
		Payload: json.RawMessage(`{"device_id":"forged"}`),
	}); err != nil {
		t.Fatal(err)
	}
	events, truncated, err := s.ListProjectDeviceTransportEvents(ctx, project.ID, now, now.Add(2*time.Second))
	if err != nil || truncated || len(events) != 2 {
		t.Fatalf("events=%+v truncated=%v err=%v", events, truncated, err)
	}
	if events[0].EventType != contracts.StageDeviceTransportConnectedEventType ||
		events[1].EventType != contracts.StageDeviceTransportDisconnectedEventType ||
		events[0].ProjectID != project.ID || events[1].ProjectID != project.ID {
		t.Fatalf("project transport event ordering/scope=%+v", events)
	}
	for i := 0; i < 501; i++ {
		appendEvent(project.ID, contracts.StageDeviceTransportConnectedEventType, now, `{"device_id":"bounded"}`)
	}
	events, truncated, err = s.ListProjectDeviceTransportEvents(ctx, project.ID, now, now.Add(2*time.Second))
	if err != nil || !truncated || len(events) != 500 {
		t.Fatalf("bounded events=%d truncated=%v err=%v", len(events), truncated, err)
	}
	if _, _, err := s.ListProjectDeviceTransportEvents(ctx, "", now, now); err == nil {
		t.Fatal("empty Project ID accepted")
	}
	if _, _, err := s.ListProjectDeviceTransportEvents(ctx, project.ID, now.Add(time.Second), now); err == nil {
		t.Fatal("reversed event interval accepted")
	}
}
