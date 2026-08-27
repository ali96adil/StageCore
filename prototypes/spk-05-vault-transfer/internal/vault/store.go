package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Object struct {
	Algorithm string `json:"algorithm"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
}

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("vault root is required")
	}
	for _, p := range []string{filepath.Join(root, "objects", "sha256"), filepath.Join(root, "staging")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{root: root}, nil
}

func (s *Store) Import(ctx context.Context, r io.Reader) (Object, error) {
	f, err := os.CreateTemp(filepath.Join(s.root, "staging"), "import-*.part")
	if err != nil {
		return Object{}, err
	}
	temp := f.Name()
	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = os.Remove(temp)
		}
	}()

	h := sha256.New()
	buf := make([]byte, 256*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return Object{}, err
		}
		n, readErr := r.Read(buf)
		if n > 0 {
			if _, err := h.Write(buf[:n]); err != nil {
				return Object{}, err
			}
			wn, err := f.Write(buf[:n])
			if err != nil {
				return Object{}, err
			}
			if wn != n {
				return Object{}, io.ErrShortWrite
			}
			size += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Object{}, readErr
		}
	}
	if err := f.Sync(); err != nil {
		return Object{}, err
	}
	if err := f.Close(); err != nil {
		return Object{}, err
	}

	digest := hex.EncodeToString(h.Sum(nil))
	final := s.objectPath(digest)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return Object{}, err
	}
	if st, err := os.Stat(final); err == nil {
		if st.Size() != size {
			return Object{}, fmt.Errorf("existing object size mismatch for %s", digest)
		}
		_ = os.Remove(temp)
		committed = true
		return Object{Algorithm: "sha256", SHA256: digest, Size: size, Path: final}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Object{}, err
	}

	if err := os.Rename(temp, final); err != nil {
		return Object{}, err
	}
	committed = true
	if err := syncDir(filepath.Dir(final)); err != nil {
		return Object{}, err
	}
	return Object{Algorithm: "sha256", SHA256: digest, Size: size, Path: final}, nil
}

func (s *Store) Open(sha string) (*os.File, os.FileInfo, error) {
	if len(sha) != 64 {
		return nil, nil, errors.New("invalid sha256")
	}
	p := s.objectPath(sha)
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, st, nil
}

func (s *Store) objectPath(sha string) string {
	prefix := "xx"
	if len(sha) >= 2 {
		prefix = sha[:2]
	}
	return filepath.Join(s.root, "objects", "sha256", prefix, sha)
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
