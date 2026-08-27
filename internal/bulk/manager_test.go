package bulk_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/bulk"
)

func TestShowPausesRunningBulkAndBlocksNewSoftware(t *testing.T) {
	var current atomic.Int32
	manager := bulk.New(func(context.Context) (bulk.Mode, error) {
		switch current.Load() {
		case 1:
			return bulk.ModeShow, nil
		case 2:
			return bulk.ModeRehearsal, nil
		default:
			return bulk.ModeEdit, nil
		}
	})
	jobID, err := manager.Begin(context.Background(), bulk.KindMediaSync, 100)
	if err != nil {
		t.Fatal(err)
	}
	current.Store(1)

	done := make(chan error, 1)
	go func() { done <- manager.WaitAllowed(context.Background(), jobID) }()
	deadline := time.Now().Add(time.Second)
	for {
		job, _ := manager.Job(jobID)
		if job.State == bulk.Paused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not pause: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("paused job returned early: %v", err)
	default:
	}
	if _, err := manager.Begin(context.Background(), bulk.KindSoftwareDownload, 50); !errors.Is(err, bulk.ErrShowBlocked) {
		t.Fatalf("new software download in SHOW error=%v", err)
	}

	current.Store(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("job did not resume after leaving SHOW")
	}
	job, _ := manager.Job(jobID)
	if job.State != bulk.Running || job.PauseReason != "" {
		t.Fatalf("resumed job=%#v", job)
	}
}

func TestRehearsalAllowsRequiredMediaButDefersBackup(t *testing.T) {
	manager := bulk.New(func(context.Context) (bulk.Mode, error) { return bulk.ModeRehearsal, nil })
	if _, err := manager.Begin(context.Background(), bulk.KindMediaSync, 1); err != nil {
		t.Fatalf("required media in rehearsal: %v", err)
	}
	if _, err := manager.Begin(context.Background(), bulk.KindBackup, 1); !errors.Is(err, bulk.ErrShowBlocked) {
		t.Fatalf("backup in rehearsal error=%v", err)
	}
}
