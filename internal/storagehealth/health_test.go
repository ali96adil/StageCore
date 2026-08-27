package storagehealth_test

import (
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/storagehealth"
)

func TestHealthThresholdsAndReserveAdmission(t *testing.T) {
	const gib = uint64(1 << 30)
	policy := storagehealth.NewPolicyWithProbe(2<<30, 15, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 100 * gib, FreeBytes: 10 * gib}, nil
	})
	warning := policy.EvaluateForTest(storagehealth.Filesystem{TotalBytes: 100 * gib, FreeBytes: 10 * gib}, true)
	if warning.State != storagehealth.Warning {
		t.Fatalf("warning status=%#v", warning)
	}
	critical := policy.EvaluateForTest(storagehealth.Filesystem{TotalBytes: 100 * gib, FreeBytes: gib}, true)
	if critical.State != storagehealth.Critical {
		t.Fatalf("critical status=%#v", critical)
	}
	unwritable := policy.EvaluateForTest(storagehealth.Filesystem{TotalBytes: 100 * gib, FreeBytes: 50 * gib}, false)
	if unwritable.State != storagehealth.Critical {
		t.Fatalf("unwritable status=%#v", unwritable)
	}
	if err := policy.Admit("ignored", 7*gib); err != nil {
		t.Fatalf("admit safe write: %v", err)
	}
	if err := policy.Admit("ignored", 9*gib); !errors.Is(err, storagehealth.ErrRuntimeReserve) {
		t.Fatalf("reserve admission error=%v", err)
	}
}
