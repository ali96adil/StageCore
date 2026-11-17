package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/pressly/goose/v3"
)

func TestCompanionUploadBrowserIssuerMigrationPreservesLegacyActiveTicket(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), FileName)))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	migrate := func(version int64) {
		t.Helper()
		migrationMu.Lock()
		defer migrationMu.Unlock()
		goose.SetBaseFS(migrations.FS)
		defer goose.SetBaseFS(nil)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatal(err)
		}
		if err := goose.UpTo(database, ".", version); err != nil {
			t.Fatal(err)
		}
	}
	migrate(53)

	stageStore := store.New(database, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Legacy capture ticket"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, legacyCaptureManifest(), "migration-test")
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "MAC-CAPTURE", DisplayName: "Capture Mac", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: "Legacy Capture Mac"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetCompanionTrustState(ctx, companion.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.AssignMachineRole(ctx, role.ID, companion.ID); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "migration-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runtimeSession, err := stageStore.CreateCompanionRuntimeSession(
		ctx, companion.ID, strings.Repeat("1", 64), now.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	credential := base64.RawURLEncoding.EncodeToString(bytesOf(0x5a, 32))
	credentialDigest := sha256.Sum256([]byte(credential))
	contentHash := strings.Repeat("b", 64)
	createdAt := now.Add(-time.Minute).UnixMicro()
	const ticketID = "00000000-0000-7000-8000-000000000054"
	if _, err := database.ExecContext(ctx, `
		INSERT INTO companion_upload_tickets (
			upload_ticket_id, credential_hash, companion_id, runtime_session_id,
			operation_id, environment_manifest_id, machine_role_id, runtime_snapshot_id,
			purpose, expected_content_hash, expected_size_bytes, status, created_at_us, expires_at_us
		) VALUES (?, ?, ?, ?, 'legacy-capture', ?, ?, ?, 'EXECUTION_ENVIRONMENT_CAPTURE', ?, 7, 'ACTIVE', ?, ?)
	`, ticketID, hex.EncodeToString(credentialDigest[:]), companion.ID, runtimeSession.ID,
		environment.ID, role.ID, runtimeSnapshot.ID, contentHash, createdAt, now.Add(10*time.Minute).UnixMicro()); err != nil {
		t.Fatal(err)
	}

	migrate(54)
	ticket, err := stageStore.GetCompanionUploadTicket(ctx, ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Status != store.CompanionUploadTicketActive || ticket.IssuerBrowserSessionID != "" || ticket.IssuerRequiresProjectGrants {
		t.Fatalf("legacy ticket was not preserved with compatibility defaults: %+v", ticket)
	}
	authorized, err := stageStore.AuthorizeCompanionUploadTicket(ctx, credential)
	if err != nil || authorized.ID != ticketID {
		t.Fatalf("legacy active ticket no longer authorizes: ticket=%+v err=%v", authorized, err)
	}
	completed, err := stageStore.CompleteCompanionUploadTicket(ctx, ticketID, contentHash, 7)
	if err != nil || completed.Status != store.CompanionUploadTicketCompleted {
		t.Fatalf("legacy active ticket no longer completes: ticket=%+v err=%v", completed, err)
	}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func legacyCaptureManifest() executionenv.Manifest {
	return executionenv.Manifest{
		SchemaVersion:  executionenv.ManifestSchemaVersion,
		EnvironmentKey: "legacy-capture", Name: "Legacy capture environment",
		AdapterKey: "stagecore.adapter.vdmx",
		Application: executionenv.ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", VersionConstraint: "8.x-tested",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
	}
}
