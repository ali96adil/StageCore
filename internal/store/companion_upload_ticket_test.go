package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCompanionUploadTicketAuthorityAndLifecycle(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Upload ticket", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	environment, err := s.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, executionEnvironmentFixture("capture-upload"), "test",
	)
	if err != nil { t.Fatal(err) }
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "MAC-UPLOAD", DisplayName: "Upload Mac", Required: true,
	})
	if err != nil { t.Fatal(err) }
	companion := registerTrustedCompanion(t, ctx, s, "Upload Mac")
	if _, err := s.AssignMachineRole(ctx, role.ID, companion.ID); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }

	expectedHash := strings.Repeat("a", 64)
	grant, err := s.CreateCompanionUploadTicket(ctx, store.CreateCompanionUploadTicketParams{
		CompanionID: companion.ID,
		OperationID: "capture-op-1",
		EnvironmentManifestID: environment.ID,
		MachineRoleID: role.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Purpose: store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash: expectedHash,
		ExpectedSizeBytes: 4096,
		ExpiresAt: fixedTime.Add(10 * time.Minute),
	})
	if err != nil { t.Fatal(err) }
	if grant.Credential == "" ||
		grant.Ticket.Status != store.CompanionUploadTicketActive ||
		grant.Ticket.CompanionID != companion.ID ||
		grant.Ticket.EnvironmentManifestID != environment.ID ||
		grant.Ticket.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("grant=%+v", grant)
	}

	var storedCredentialHash string
	if err := handle.DB.QueryRowContext(ctx,
		`SELECT credential_hash FROM companion_upload_tickets WHERE upload_ticket_id = ?`,
		grant.Ticket.ID,
	).Scan(&storedCredentialHash); err != nil {
		t.Fatal(err)
	}
	if storedCredentialHash == grant.Credential || strings.Contains(storedCredentialHash, grant.Credential) {
		t.Fatal("raw upload credential was stored")
	}

	authorized, err := s.AuthorizeCompanionUploadTicket(ctx, grant.Credential)
	if err != nil { t.Fatal(err) }
	if authorized.ID != grant.Ticket.ID {
		t.Fatalf("authorized=%+v grant=%+v", authorized, grant.Ticket)
	}
	if _, err := s.AuthorizeCompanionUploadTicket(ctx, "not-a-ticket"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid credential err=%v", err)
	}

	if _, err := s.CompleteCompanionUploadTicket(ctx, grant.Ticket.ID, strings.Repeat("b", 64), 4096); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wrong hash completion err=%v", err)
	}
	active, err := s.GetCompanionUploadTicket(ctx, grant.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if active.Status != store.CompanionUploadTicketActive {
		t.Fatalf("mismatched completion mutated ticket=%+v", active)
	}

	completed, err := s.CompleteCompanionUploadTicket(ctx, grant.Ticket.ID, expectedHash, 4096)
	if err != nil { t.Fatal(err) }
	if completed.Status != store.CompanionUploadTicketCompleted || completed.TerminalAt == nil {
		t.Fatalf("completed=%+v", completed)
	}
	if _, err := s.AuthorizeCompanionUploadTicket(ctx, grant.Credential); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("completed ticket authorize err=%v", err)
	}
}

func TestCompanionUploadTicketExpiresAndRejectsCrossProjectAuthority(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Upload ticket A", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	environment, err := s.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, executionEnvironmentFixture("capture-upload-a"), "test",
	)
	if err != nil { t.Fatal(err) }
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "MAC-A", Required: true})
	if err != nil { t.Fatal(err) }
	companion := registerTrustedCompanion(t, ctx, s, "Upload Mac A")
	if _, err := s.AssignMachineRole(ctx, role.ID, companion.ID); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }

	expiring, err := s.CreateCompanionUploadTicket(ctx, store.CreateCompanionUploadTicketParams{
		CompanionID: companion.ID,
		OperationID: "capture-expiring",
		EnvironmentManifestID: environment.ID,
		MachineRoleID: role.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Purpose: store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash: strings.Repeat("c", 64),
		ExpectedSizeBytes: 1,
		ExpiresAt: fixedTime.Add(time.Minute),
	})
	if err != nil { t.Fatal(err) }
	late := store.New(handle.DB, clock.Fixed{Time: fixedTime.Add(2 * time.Minute)})
	if _, err := late.AuthorizeCompanionUploadTicket(ctx, expiring.Credential); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expired authorize err=%v", err)
	}
	expired, err := late.GetCompanionUploadTicket(ctx, expiring.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if expired.Status != store.CompanionUploadTicketExpired || expired.TerminalAt == nil {
		t.Fatalf("expired=%+v", expired)
	}

	projectB, revisionB, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Upload ticket B", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	roleB, err := s.CreateMachineRole(ctx, projectB.ID, store.CreateMachineRoleParams{RoleKey: "MAC-B", Required: true})
	if err != nil { t.Fatal(err) }
	if _, err := s.AssignMachineRole(ctx, roleB.ID, companion.ID); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revisionB.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeB, _, err := snapshot.NewBuilder(s).Create(ctx, revisionB.ID, "test")
	if err != nil { t.Fatal(err) }

	_, err = s.CreateCompanionUploadTicket(ctx, store.CreateCompanionUploadTicketParams{
		CompanionID: companion.ID,
		OperationID: "capture-cross-project",
		EnvironmentManifestID: environment.ID,
		MachineRoleID: roleB.ID,
		RuntimeSnapshotID: runtimeB.ID,
		Purpose: store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash: strings.Repeat("d", 64),
		ExpectedSizeBytes: 128,
		ExpiresAt: fixedTime.Add(10 * time.Minute),
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-project ticket err=%v", err)
	}
}

func TestCompanionUploadTicketCancelAndBulkExpiry(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Upload cancel", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	environment, err := s.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, executionEnvironmentFixture("capture-upload-cancel"), "test",
	)
	if err != nil { t.Fatal(err) }
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "MAC-C", Required: true})
	if err != nil { t.Fatal(err) }
	companion := registerTrustedCompanion(t, ctx, s, "Upload Mac C")
	if _, err := s.AssignMachineRole(ctx, role.ID, companion.ID); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }

	makeTicket := func(operation string, expiresAt time.Time) store.CompanionUploadTicketGrant {
		t.Helper()
		grant, err := s.CreateCompanionUploadTicket(ctx, store.CreateCompanionUploadTicketParams{
			CompanionID: companion.ID,
			OperationID: operation,
			EnvironmentManifestID: environment.ID,
			MachineRoleID: role.ID,
			RuntimeSnapshotID: runtimeSnapshot.ID,
			Purpose: store.CompanionUploadExecutionEnvironmentCapture,
			ExpectedContentHash: strings.Repeat("e", 64),
			ExpectedSizeBytes: 64,
			ExpiresAt: expiresAt,
		})
		if err != nil { t.Fatal(err) }
		return grant
	}

	cancelledGrant := makeTicket("capture-cancel", fixedTime.Add(10*time.Minute))
	if err := s.CancelCompanionUploadTicket(ctx, cancelledGrant.Ticket.ID); err != nil { t.Fatal(err) }
	cancelled, err := s.GetCompanionUploadTicket(ctx, cancelledGrant.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if cancelled.Status != store.CompanionUploadTicketCancelled || cancelled.TerminalAt == nil {
		t.Fatalf("cancelled=%+v", cancelled)
	}
	if err := s.CancelCompanionUploadTicket(ctx, cancelledGrant.Ticket.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("repeat cancel err=%v", err)
	}

	expiringGrant := makeTicket("capture-bulk-expire", fixedTime.Add(time.Minute))
	late := store.New(handle.DB, clock.Fixed{Time: fixedTime.Add(2*time.Minute)})
	count, err := late.ExpireCompanionUploadTickets(ctx)
	if err != nil { t.Fatal(err) }
	if count != 1 {
		t.Fatalf("expired count=%d want 1", count)
	}
	expired, err := late.GetCompanionUploadTicket(ctx, expiringGrant.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if expired.Status != store.CompanionUploadTicketExpired {
		t.Fatalf("expired=%+v", expired)
	}
}
