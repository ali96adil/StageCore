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

func advanceToRebooting(
	t *testing.T,
	store *LifecycleStore,
	deviceID string,
	now time.Time,
	generation int64,
) UpdateRecord {
	t.Helper()
	ctx := context.Background()
	record, err := store.Issue(ctx, lifecycleManifest(deviceID, now), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkSent(ctx, record.Manifest.UpdateID, deviceID, generation); err != nil {
		t.Fatal(err)
	}
	for _, state := range []UpdateState{
		UpdateAccepted,
		UpdateDownloading,
		UpdateVerifying,
		UpdateWriting,
		UpdateRebooting,
	} {
		record, err = store.RecordDeviceState(
			ctx, record.Manifest.UpdateID, deviceID, generation, state, string(state), "",
		)
		if err != nil {
			t.Fatalf("advance to %s: %v", state, err)
		}
	}
	return record
}

func TestLifecycleRequiresExactOrderAndPostRebootTargetVersion(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	issued, err := store.Issue(ctx, lifecycleManifest(deviceID, now), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	sent, err := store.MarkSent(ctx, issued.Manifest.UpdateID, deviceID, 11)
	if err != nil {
		t.Fatal(err)
	}
	if sent.State != UpdateSent || sent.ConnectionGeneration != 11 {
		t.Fatalf("sent=%+v", sent)
	}

	if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, UpdateDownloading, "", ""); !errors.Is(err, ErrUpdateState) {
		t.Fatalf("skip SENT->DOWNLOADING err=%v", err)
	}
	if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 12, UpdateAccepted, "", ""); !errors.Is(err, ErrUpdateGeneration) {
		t.Fatalf("wrong generation err=%v", err)
	}

	for _, state := range []UpdateState{
		UpdateAccepted,
		UpdateDownloading,
		UpdateVerifying,
		UpdateWriting,
		UpdateRebooting,
	} {
		if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, state, string(state), ""); err != nil {
			t.Fatalf("advance to %s: %v", state, err)
		}
	}
	if _, err := store.RecordDeviceState(ctx, issued.Manifest.UpdateID, deviceID, 11, UpdateCompleted, "", ""); !errors.Is(err, ErrUpdateState) {
		t.Fatalf("device-reported completion err=%v", err)
	}
	if _, handled, err := store.ConfirmPostReboot(ctx, deviceID, issued.Manifest.ProfileID, issued.Manifest.TargetVersion, 11); !handled || !errors.Is(err, ErrUpdateGeneration) {
		t.Fatalf("same-generation completion handled=%v err=%v", handled, err)
	}

	completed, handled, err := store.ConfirmPostReboot(
		ctx, deviceID, issued.Manifest.ProfileID, issued.Manifest.TargetVersion, 12,
	)
	if err != nil || !handled {
		t.Fatalf("post-reboot completion handled=%v err=%v", handled, err)
	}
	if completed.State != UpdateCompleted || completed.CompletionGeneration != 12 {
		t.Fatalf("completed=%+v", completed)
	}
}

func TestLifecycleExpiresBeforeAcceptance(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	issuedAt := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return issuedAt }
	record, err := store.Issue(ctx, lifecycleManifest(deviceID, issuedAt), "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkSent(ctx, record.Manifest.UpdateID, deviceID, 3); err != nil {
		t.Fatal(err)
	}

	store.now = func() time.Time { return issuedAt.Add(6 * time.Minute) }
	if _, err := store.RecordDeviceState(
		ctx, record.Manifest.UpdateID, deviceID, 3, UpdateAccepted, "late", "",
	); !errors.Is(err, ErrUpdateExpired) {
		t.Fatalf("expired acceptance err=%v", err)
	}
	current, err := store.Get(ctx, record.Manifest.UpdateID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != UpdateExpired || current.LastErrorCode != "MANIFEST_EXPIRED" {
		t.Fatalf("current=%+v", current)
	}
}

func TestLifecycleRecordsRollbackAfterReboot(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	record := advanceToRebooting(t, store, deviceID, now, 20)

	result, handled, err := store.ConfirmPostReboot(
		ctx,
		deviceID,
		record.Manifest.ProfileID,
		record.Manifest.CurrentVersion,
		21,
	)
	if err != nil || !handled {
		t.Fatalf("rollback handled=%v err=%v", handled, err)
	}
	if result.State != UpdateFailed ||
		result.LastErrorCode != "ROLLBACK_OBSERVED" ||
		result.CompletionGeneration != 21 {
		t.Fatalf("rollback result=%+v", result)
	}
}

func TestLifecycleDisconnectInterruptsBeforeRebootButPreservesRebootHandoff(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	first := lifecycleManifest(deviceID, now)
	record, err := store.Issue(ctx, first, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkSent(ctx, record.Manifest.UpdateID, deviceID, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordDeviceState(ctx, record.Manifest.UpdateID, deviceID, 30, UpdateAccepted, "", ""); err != nil {
		t.Fatal(err)
	}
	count, err := store.InterruptConnection(ctx, deviceID, 30, "socket closed")
	if err != nil || count != 1 {
		t.Fatalf("interrupt count=%d err=%v", count, err)
	}
	current, _ := store.Get(ctx, record.Manifest.UpdateID)
	if current.State != UpdateInterrupted {
		t.Fatalf("interrupted state=%s", current.State)
	}

	second := lifecycleManifest(deviceID, now)
	second.UpdateID = "018f2744-0cb0-7bf6-9637-4a3a467a7a42"
	record, err = store.Issue(ctx, second, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkSent(ctx, record.Manifest.UpdateID, deviceID, 31); err != nil {
		t.Fatal(err)
	}
	for _, state := range []UpdateState{
		UpdateAccepted, UpdateDownloading, UpdateVerifying, UpdateWriting, UpdateRebooting,
	} {
		if _, err := store.RecordDeviceState(ctx, record.Manifest.UpdateID, deviceID, 31, state, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	count, err = store.InterruptConnection(ctx, deviceID, 31, "expected reboot disconnect")
	if err != nil || count != 0 {
		t.Fatalf("reboot interrupt count=%d err=%v", count, err)
	}
	current, _ = store.Get(ctx, record.Manifest.UpdateID)
	if current.State != UpdateRebooting {
		t.Fatalf("reboot handoff state=%s", current.State)
	}
}

func TestLifecycleHubRestartPreservesRebootHandoff(t *testing.T) {
	ctx := context.Background()
	store, _, deviceID := newLifecycleTestStore(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	record := advanceToRebooting(t, store, deviceID, now, 40)

	count, err := store.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unexpected interrupted count=%d", count)
	}
	current, err := store.Get(ctx, record.Manifest.UpdateID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != UpdateRebooting {
		t.Fatalf("state after Hub restart=%s", current.State)
	}
}
