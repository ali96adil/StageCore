package securityaudit

import (
	"context"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/secretstore"
)

func TestAuditPersistsAndRedactsCurrentSecrets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	secrets, err := secretstore.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Create(ctx, "device-token", "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	audit, err := New(h.DB, secrets)
	if err != nil {
		t.Fatal(err)
	}
	record, err := audit.Append(ctx, Event{
		EventType: "secret.test", ActorUsername: "owner", ResourceType: "secret",
		ResourceID: "secret:device-token", Result: ResultSuccess,
		Reason: "rotated super-secret-value safely",
		Metadata: map[string]any{"accidental": "super-secret-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.Reason, "super-secret-value") || strings.Contains(string(record.Metadata), "super-secret-value") {
		t.Fatalf("audit leaked secret: %#v", record)
	}

	reopened, err := New(h.DB, secrets)
	if err != nil {
		t.Fatal(err)
	}
	records, err := reopened.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].AuditID != record.AuditID || records[0].Result != ResultSuccess {
		t.Fatalf("records=%#v", records)
	}
	if strings.Contains(records[0].Reason+string(records[0].Metadata), "super-secret-value") {
		t.Fatal("persisted audit leaked secret")
	}
}
