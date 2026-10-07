package deviceupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
)

func newLifecycleTestStore(t *testing.T) (*LifecycleStore, *db.Handle, string) {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	deviceID := "23c45a07-7286-4afc-91d9-7e54df72aeee"
	if _, err := handle.DB.ExecContext(ctx,
		`INSERT INTO stage_device_identity_registry (device_id, created_at_us) VALUES (?, ?)`,
		deviceID, time.Now().UTC().UnixMicro(),
	); err != nil {
		t.Fatal(err)
	}
	store, err := NewLifecycleStore(handle.DB)
	if err != nil {
		t.Fatal(err)
	}
	return store, handle, deviceID
}

func lifecycleManifest(deviceID string, now time.Time) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion1,
		UpdateID: "018f2744-0cb0-7bf6-9637-4a3a467a7a41",
		DeviceID: deviceID,
		ProfileID: "stagecore.esp32-stagelaser",
		CurrentVersion: "0.1.0",
		TargetVersion: "0.2.0",
		SourceRevision: strings.Repeat("a", 40),
		Qualification: QualificationQualified,
		ArtifactPath: ArtifactPathPrefix + "018f2744-0cb0-7bf6-9637-4a3a467a7a31/firmware.bin",
		ArtifactSize: 123456,
		ArtifactSHA256: strings.Repeat("b", 64),
		IssuedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
		RollbackRequired: true,
	}
}

func TestLifecycleRequiresExplicitSendAndBoundGeneration(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	issued, err := store.Issue(ctx, lifecycleManifest(deviceID, now), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if issued.State != UpdateIssued || issued.ConnectionGeneration != 0 {
		t.Fatalf("issued=%+v", issued)
	}

	sent, err := store.MarkSent(ctx, issued.Manifest.UpdateID, deviceID, 11)
	if err != nil {
		t.Fatal(err)
	}
	if sent.State != UpdateSent || sent.ConnectionGeneration != 11 {
		t.Fatalf("sent=%+v", sent)
	}

	if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 12, UpdateAccepted, "", ""); !errors.Is(err, ErrUpdateGeneration) {
		t.Fatalf("wrong-generation result err=%v", err)
	}
	accepted, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, UpdateAccepted, "accepted", "")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.State != UpdateAccepted {
		t.Fatalf("accepted state=%s", accepted.State)
	}
	completed, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, UpdateCompleted, "verified", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != UpdateCompleted {
		t.Fatalf("completed state=%s", completed.State)
	}
	if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, UpdateFailed, "", "late"); !errors.Is(err, ErrUpdateState) {
		t.Fatalf("terminal mutation err=%v", err)
	}
}

func TestLifecycleExpiresBeforeSend(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	issuedAt := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return issuedAt }
	record, err := store.Issue(ctx, lifecycleManifest(deviceID, issuedAt), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return issuedAt.Add(6 * time.Minute) }
	if _, err := store.MarkSent(ctx, record.Manifest.UpdateID, deviceID, 3); !errors.Is(err, ErrUpdateExpired) {
		t.Fatalf("expired send err=%v", err)
	}
	current, err := store.Get(ctx, record.Manifest.UpdateID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != UpdateExpired {
		t.Fatalf("state=%s", current.State)
	}
}

func TestLifecycleRestartInterruptsDeliveredUpdatesWithoutRetry(t *testing.T) {
	ctx := context.Background()
	store, handle, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	first, err := store.Issue(ctx, lifecycleManifest(deviceID, now), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkSent(ctx, first.Manifest.UpdateID, deviceID, 7); err != nil {
		t.Fatal(err)
	}

	secondManifest := lifecycleManifest(deviceID, now)
	secondManifest.UpdateID = "018f2744-0cb0-7bf6-9637-4a3a467a7a42"
	if _, err := store.Issue(ctx, secondManifest, "owner-1"); err != nil {
		t.Fatal(err)
	}

	count, err := store.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("interrupted count=%d", count)
	}
	delivered, _ := store.Get(ctx, first.Manifest.UpdateID)
	unsent, _ := store.Get(ctx, secondManifest.UpdateID)
	if delivered.State != UpdateInterrupted {
		t.Fatalf("delivered state=%s", delivered.State)
	}
	if unsent.State != UpdateIssued {
		t.Fatalf("unsent state=%s", unsent.State)
	}

	var active int
	if err := handle.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM stage_device_firmware_updates WHERE state IN ('SENT','ACCEPTED','DOWNLOADING','VERIFYING','WRITING','REBOOTING')`,
	).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active delivered updates=%d", active)
	}
}
