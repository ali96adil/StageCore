package transfer

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-05-vault-transfer/internal/vault"
)

type cyclingReader struct {
	block []byte
	off   int
}

func (r *cyclingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.block[r.off]
		r.off++
		if r.off == len(r.block) {
			r.off = 0
		}
	}
	return len(p), nil
}

func TestResumeAndChecksum(t *testing.T) {
	v, obj := makeObject(t, 16<<20)
	gate := NewGate()
	s := &Server{Vault: v, Gate: gate, ChunkSize: 64 << 10}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "media.bin")
	part := dest + ".part"
	resp, err := http.Get(ts.URL + "/objects/" + obj.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(part)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(f, resp.Body, 2<<20); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	_ = resp.Body.Close()

	if err := (Client{}).Resume(context.Background(), ts.URL+"/objects/"+obj.SHA256, obj.SHA256, dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != obj.Size {
		t.Fatalf("size %d want %d", st.Size(), obj.Size)
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists")
	}
}

func TestShowPausesBulkButRuntimePingWorks(t *testing.T) {
	v, obj := makeObject(t, 32<<20)
	gate := NewGate()
	s := &Server{Vault: v, Gate: gate, ChunkSize: 64 << 10, ChunkDelay: time.Millisecond}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	done := make(chan error, 1)
	dest := filepath.Join(t.TempDir(), "media.bin")
	go func() {
		done <- (Client{}).Resume(context.Background(), ts.URL+"/objects/"+obj.SHA256, obj.SHA256, dest)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for s.BytesSent() < 512<<10 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	gate.Set(ModeShow)
	before := s.BytesSent()
	time.Sleep(75 * time.Millisecond)
	after := s.BytesSent()
	if after > before+int64(s.ChunkSize) {
		t.Fatalf("bulk kept advancing in SHOW: before=%d after=%d", before, after)
	}

	start := time.Now()
	resp, err := http.Get(ts.URL + "/runtime/ping")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "PONG" {
		t.Fatalf("runtime ping %q", body)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("runtime ping too slow: %v", elapsed)
	}

	gate.Set(ModeEdit)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transfer did not resume")
	}
}

func TestCorruptPartialNeverPromotes(t *testing.T) {
	v, obj := makeObject(t, 4<<20)
	gate := NewGate()
	s := &Server{Vault: v, Gate: gate}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "media.bin")
	if err := os.WriteFile(dest+".part", []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := (Client{}).Resume(context.Background(), ts.URL+"/objects/"+obj.SHA256, obj.SHA256, dest)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("corrupt file was promoted")
	}
}

func makeObject(t *testing.T, size int64) (*vault.Store, vault.Object) {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	block := bytes.Repeat([]byte("StageCore-0123456789abcdef"), 4096)
	rr := &cyclingReader{block: block}
	obj, err := v.Import(context.Background(), io.LimitReader(rr, size))
	if err != nil {
		t.Fatal(err)
	}
	return v, obj
}
