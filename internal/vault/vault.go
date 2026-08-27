package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/store"
)

const directoryMode = 0o750

type Vault struct {
	root        string
	stagingRoot string
	objectsRoot string
	store       *store.Store
}

type ImportParams struct {
	ProjectID        string
	Name             string
	AssetPolicy      string
	OriginalFilename string
}

func Open(root string, s *store.Store) (*Vault, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("Vault root is required")
	}
	if s == nil {
		return nil, fmt.Errorf("Vault store is required")
	}
	root = filepath.Clean(root)
	staging := filepath.Join(root, "staging")
	objects := filepath.Join(root, "objects", "sha256")
	for _, dir := range []string{staging, objects} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			return nil, fmt.Errorf("create Vault directory %q: %w", dir, err)
		}
	}
	probe, err := os.CreateTemp(staging, ".write-probe-*")
	if err != nil {
		return nil, fmt.Errorf("Vault staging is not writable: %w", err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return nil, fmt.Errorf("close Vault write probe: %w", err)
	}
	if err := os.Remove(probeName); err != nil {
		return nil, fmt.Errorf("remove Vault write probe: %w", err)
	}
	return &Vault{root: root, stagingRoot: staging, objectsRoot: objects, store: s}, nil
}

func (v *Vault) Root() string {
	if v == nil {
		return ""
	}
	return v.root
}

func (v *Vault) ImportManaged(ctx context.Context, p ImportParams, r io.Reader) (store.ManagedMedia, error) {
	if v == nil || v.store == nil {
		return store.ManagedMedia{}, fmt.Errorf("Vault is unavailable")
	}
	if r == nil {
		return store.ManagedMedia{}, fmt.Errorf("import reader is required")
	}
	if _, err := v.store.GetProject(ctx, strings.TrimSpace(p.ProjectID)); err != nil {
		return store.ManagedMedia{}, fmt.Errorf("validate media project: %w", err)
	}

	staged, err := os.CreateTemp(v.stagingRoot, "import-*.part")
	if err != nil {
		return store.ManagedMedia{}, fmt.Errorf("create Vault staging file: %w", err)
	}
	stagedPath := staged.Name()
	promoted := false
	defer func() {
		_ = staged.Close()
		if !promoted {
			_ = os.Remove(stagedPath)
		}
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(staged, hasher), r)
	if err != nil {
		return store.ManagedMedia{}, fmt.Errorf("stream managed media into staging: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return store.ManagedMedia{}, fmt.Errorf("sync staged managed media: %w", err)
	}
	if err := staged.Close(); err != nil {
		return store.ManagedMedia{}, fmt.Errorf("close staged managed media: %w", err)
	}

	contentHash := hex.EncodeToString(hasher.Sum(nil))
	relativePath := objectRelativePath(contentHash)
	objectPath := filepath.Join(v.root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(objectPath), directoryMode); err != nil {
		return store.ManagedMedia{}, fmt.Errorf("create Vault object directory: %w", err)
	}

	if info, statErr := os.Stat(objectPath); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() != size {
			return store.ManagedMedia{}, fmt.Errorf("existing Vault object conflicts with verified content identity")
		}
	} else if !os.IsNotExist(statErr) {
		return store.ManagedMedia{}, fmt.Errorf("inspect Vault object: %w", statErr)
	} else {
		// Link is an atomic no-overwrite promotion while staging and objects are on
		// the same Vault filesystem. A concurrent identical import may win first;
		// in that case the existing immutable object is reused.
		if err := os.Link(stagedPath, objectPath); err != nil {
			if info, retryErr := os.Stat(objectPath); retryErr != nil || !info.Mode().IsRegular() || info.Size() != size {
				return store.ManagedMedia{}, fmt.Errorf("atomically promote Vault object: %w", err)
			}
		}
	}
	promoted = true
	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return store.ManagedMedia{}, fmt.Errorf("remove promoted staging link: %w", err)
	}

	managed, err := v.store.RegisterManagedMedia(ctx, store.RegisterManagedMediaParams{
		ProjectID:        p.ProjectID,
		Name:             p.Name,
		AssetPolicy:      p.AssetPolicy,
		ContentHash:      contentHash,
		SizeBytes:        size,
		RelativePath:     filepath.ToSlash(relativePath),
		OriginalFilename: filepath.Base(strings.TrimSpace(p.OriginalFilename)),
	})
	if err != nil {
		return store.ManagedMedia{}, fmt.Errorf("register imported managed media: %w", err)
	}
	return managed, nil
}

func (v *Vault) ObjectPath(contentHash string) (string, error) {
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	if len(contentHash) != 64 {
		return "", fmt.Errorf("invalid SHA-256 content identity")
	}
	if _, err := hex.DecodeString(contentHash); err != nil {
		return "", fmt.Errorf("invalid SHA-256 content identity: %w", err)
	}
	return filepath.Join(v.root, filepath.FromSlash(objectRelativePath(contentHash))), nil
}

func objectRelativePath(contentHash string) string {
	return filepath.ToSlash(filepath.Join("objects", "sha256", contentHash[:2], contentHash[2:4], contentHash))
}
