package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Client struct {
	HTTP *http.Client
}

func (c Client) Resume(ctx context.Context, url, expectedSHA, finalPath string) error {
	part := finalPath + ".part"
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	offset := st.Size()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("resume expected 206, got %d", resp.StatusCode)
	}
	if offset == 0 && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download expected 200, got %d", resp.StatusCode)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, 256*1024)
	if _, err := io.CopyBuffer(f, resp.Body, buf); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := verifySHA(part, expectedSHA); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(part, finalPath)
}

func verifySHA(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 256*1024)); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, expected)
	}
	return nil
}
