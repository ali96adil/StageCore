package vault

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestImportContentIdentityAndDedup(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("stagecore-vault\n"), 1024)
	a, err := s.Import(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Import(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 || a.Path != b.Path {
		t.Fatalf("expected dedup identity")
	}
	st, err := os.Stat(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != int64(len(payload)) {
		t.Fatalf("size %d", st.Size())
	}
}

func TestCapacityReserveBlocksBulkWrite(t *testing.T) {
	policy := CapacityPolicy{RuntimeReserveBytes: 2 << 30}
	if err := policy.AdmitBulkWrite(10<<30, 4<<30); err != nil {
		t.Fatalf("expected admission: %v", err)
	}
	if err := policy.AdmitBulkWrite(3<<30, 2<<30); err == nil {
		t.Fatal("expected runtime reserve rejection")
	}
}
