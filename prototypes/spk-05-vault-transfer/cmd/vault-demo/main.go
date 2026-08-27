package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ali96adil/StageCore/prototypes/spk-05-vault-transfer/internal/transfer"
	"github.com/ali96adil/StageCore/prototypes/spk-05-vault-transfer/internal/vault"
)

type cyclingReader struct {
	block []byte
	off   int
}

func (r *cyclingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.block[r.off]
		r.off = (r.off + 1) % len(r.block)
	}
	return len(p), nil
}

func main() {
	root, _ := os.MkdirTemp("", "stagecore-vault-demo-")
	defer os.RemoveAll(root)
	v, err := vault.New(filepath.Join(root, "vault"))
	must(err)

	sizeMB := int64(64)
	if raw := os.Getenv("SPK05_SIZE_MB"); raw != "" {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && parsed > 0 {
			sizeMB = parsed
		}
	}

	rr := &cyclingReader{block: bytes.Repeat([]byte("StageCore"), 8192)}
	obj, err := v.Import(context.Background(), io.LimitReader(rr, sizeMB<<20))
	must(err)
	fmt.Printf("vault object sha256:%s size=%d\n", obj.SHA256, obj.Size)

	gate := transfer.NewGate()
	srv := &transfer.Server{Vault: v, Gate: gate}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	dest := filepath.Join(root, "cache", "show-media.bin")
	must((transfer.Client{}).Resume(context.Background(), ts.URL+"/objects/"+obj.SHA256, obj.SHA256, dest))
	fmt.Printf("verified cache=%s\nSPK-05 PASS\n", dest)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
