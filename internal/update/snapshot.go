package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

const (
	snapshotManifestVersion = 1
	snapshotManifestName    = "manifest.json"
)

var ErrSnapshotIntegrity = errors.New("update rollback snapshot integrity verification failed")

type SnapshotOptions struct {
	BackupRoot  string
	InstallRoot string
	ConfigRoot  string
	DataRoot    string
	SystemdUnit string
}

type Snapshot struct {
	ID       string
	Path     string
	Manifest SnapshotManifest
}

type SnapshotManifest struct {
	SchemaVersion int            `json:"schema_version"`
	SnapshotID    string         `json:"snapshot_id"`
	CreatedAt     time.Time      `json:"created_at"`
	Items         []SnapshotItem `json:"items"`
}

type SnapshotItem struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Payload      string `json:"payload"`
	TreeSHA256   string `json:"tree_sha256"`
	RegularFiles int    `json:"regular_files"`
	SizeBytes    int64  `json:"size_bytes"`
}

type Snapshotter struct {
	Now  func() time.Time
	EUID func() int
}

func NewSnapshotter() *Snapshotter {
	return &Snapshotter{Now: time.Now, EUID: os.Geteuid}
}

func (s *Snapshotter) Create(_ context.Context, opts SnapshotOptions) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, fmt.Errorf("snapshotter is required")
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.EUID == nil {
		s.EUID = os.Geteuid
	}
	backupRoot, err := cleanAbsolute("backup root", opts.BackupRoot)
	if err != nil {
		return Snapshot{}, err
	}
	installRoot, err := cleanAbsolute("install root", opts.InstallRoot)
	if err != nil {
		return Snapshot{}, err
	}
	configRoot, err := cleanAbsolute("config root", opts.ConfigRoot)
	if err != nil {
		return Snapshot{}, err
	}
	dataRoot, err := cleanAbsolute("data root", opts.DataRoot)
	if err != nil {
		return Snapshot{}, err
	}
	unit, err := cleanAbsolute("systemd unit", opts.SystemdUnit)
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create update backup root: %w", err)
	}
	if err := os.Chmod(backupRoot, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("protect update backup root: %w", err)
	}

	id, err := stageid.New()
	if err != nil {
		return Snapshot{}, err
	}
	staging := filepath.Join(backupRoot, "."+id+".staging")
	final := filepath.Join(backupRoot, id)
	_ = os.RemoveAll(staging)
	if err := os.Mkdir(staging, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create update snapshot staging: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(staging)
		}
	}()

	specs := []struct {
		name    string
		source  string
		payload string
	}{
		{name: "managed-binaries", source: filepath.Join(installRoot, "bin"), payload: "managed-binaries"},
		{name: "deployment-config", source: configRoot, payload: "deployment-config"},
		{name: "data-root", source: dataRoot, payload: "data-root"},
		{name: "systemd-unit", source: unit, payload: "systemd-unit"},
	}
	manifest := SnapshotManifest{SchemaVersion: snapshotManifestVersion, SnapshotID: id, CreatedAt: s.Now().UTC()}
	for _, spec := range specs {
		if _, err := os.Lstat(spec.source); err != nil {
			return Snapshot{}, fmt.Errorf("snapshot %s: %w", spec.name, err)
		}
		destination := filepath.Join(staging, spec.payload)
		if err := copyPath(spec.source, destination, s.EUID() == 0); err != nil {
			return Snapshot{}, fmt.Errorf("snapshot %s: %w", spec.name, err)
		}
		digest, files, bytes, err := treeDigest(destination)
		if err != nil {
			return Snapshot{}, fmt.Errorf("verify snapshot %s: %w", spec.name, err)
		}
		manifest.Items = append(manifest.Items, SnapshotItem{
			Name: spec.name, Source: spec.source, Payload: spec.payload,
			TreeSHA256: digest, RegularFiles: files, SizeBytes: bytes,
		})
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode update snapshot manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(staging, snapshotManifestName), manifestBytes, 0o600); err != nil {
		return Snapshot{}, fmt.Errorf("write update snapshot manifest: %w", err)
	}
	if err := os.Rename(staging, final); err != nil {
		return Snapshot{}, fmt.Errorf("promote update snapshot: %w", err)
	}
	succeeded = true
	return Snapshot{ID: id, Path: final, Manifest: manifest}, nil
}

func (s *Snapshotter) Restore(_ context.Context, snapshot Snapshot) error {
	if s == nil {
		return fmt.Errorf("snapshotter is required")
	}
	if s.EUID == nil {
		s.EUID = os.Geteuid
	}
	path := filepath.Clean(strings.TrimSpace(snapshot.Path))
	if path == "" || path == "." || !filepath.IsAbs(path) {
		return fmt.Errorf("snapshot path must be absolute")
	}
	manifest, err := readSnapshotManifest(path)
	if err != nil {
		return err
	}
	for _, item := range manifest.Items {
		payload := filepath.Join(path, filepath.FromSlash(item.Payload))
		if !pathWithin(path, payload) {
			return fmt.Errorf("%w: payload escapes snapshot root", ErrSnapshotIntegrity)
		}
		digest, files, bytes, err := treeDigest(payload)
		if err != nil {
			return err
		}
		if digest != item.TreeSHA256 || files != item.RegularFiles || bytes != item.SizeBytes {
			return fmt.Errorf("%w: %s checksum or size mismatch", ErrSnapshotIntegrity, item.Name)
		}
	}

	restoreID, err := stageid.New()
	if err != nil {
		return err
	}
	for _, item := range manifest.Items {
		source, err := cleanAbsolute("snapshot restore target", item.Source)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrSnapshotIntegrity, err)
		}
		payload := filepath.Join(path, filepath.FromSlash(item.Payload))
		staging := source + ".stagecore-update-restore-" + restoreID
		_ = os.RemoveAll(staging)
		if err := copyPath(payload, staging, s.EUID() == 0); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("stage rollback restore for %s: %w", item.Name, err)
		}
		if err := os.RemoveAll(source); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("remove failed candidate path %s: %w", source, err)
		}
		if err := os.Rename(staging, source); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("promote rollback restore for %s: %w", item.Name, err)
		}
	}
	return nil
}

func readSnapshotManifest(root string) (SnapshotManifest, error) {
	content, err := os.ReadFile(filepath.Join(root, snapshotManifestName))
	if err != nil {
		return SnapshotManifest{}, fmt.Errorf("read update snapshot manifest: %w", err)
	}
	var manifest SnapshotManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return SnapshotManifest{}, fmt.Errorf("decode update snapshot manifest: %w", err)
	}
	if manifest.SchemaVersion != snapshotManifestVersion || strings.TrimSpace(manifest.SnapshotID) == "" || len(manifest.Items) != 4 {
		return SnapshotManifest{}, fmt.Errorf("%w: unsupported update snapshot manifest", ErrSnapshotIntegrity)
	}
	seen := map[string]bool{}
	for _, item := range manifest.Items {
		if item.Name == "" || item.Source == "" || item.Payload == "" || item.TreeSHA256 == "" || seen[item.Name] {
			return SnapshotManifest{}, fmt.Errorf("%w: invalid update snapshot item", ErrSnapshotIntegrity)
		}
		seen[item.Name] = true
	}
	return manifest, nil
}

func copyPath(source, destination string, preserveOwner bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to snapshot symlink %s", source)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()), preserveOwner); err != nil {
				return err
			}
		}
		if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
			return err
		}
		return preserveOwnership(destination, info, preserveOwner)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to snapshot non-regular path %s (%s)", source, info.Mode())
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
		return err
	}
	return preserveOwnership(destination, info, preserveOwner)
}

func preserveOwnership(path string, info os.FileInfo, enabled bool) error {
	if !enabled {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read ownership for %s", path)
	}
	return os.Chown(path, int(stat.Uid), int(stat.Gid))
}

func treeDigest(root string) (string, int, int64, error) {
	type node struct {
		path string
		info os.FileInfo
	}
	nodes := make([]node, 0)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink in snapshot payload", ErrSnapshotIntegrity)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("%w: non-regular path in snapshot payload", ErrSnapshotIntegrity)
		}
		nodes = append(nodes, node{path: path, info: info})
		return nil
	}); err != nil {
		return "", 0, 0, err
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].path < nodes[j].path })
	h := sha256.New()
	files := 0
	var bytes int64
	for _, n := range nodes {
		rel, err := filepath.Rel(root, n.path)
		if err != nil {
			return "", 0, 0, err
		}
		kind := "D"
		if n.info.Mode().IsRegular() {
			kind = "F"
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00%o\x00", kind, filepath.ToSlash(rel), n.info.Mode().Perm())
		if kind == "F" {
			file, err := os.Open(n.path)
			if err != nil {
				return "", 0, 0, err
			}
			written, err := io.Copy(h, file)
			closeErr := file.Close()
			if err != nil {
				return "", 0, 0, err
			}
			if closeErr != nil {
				return "", 0, 0, closeErr
			}
			files++
			bytes += written
		}
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), files, bytes, nil
}

func cleanAbsolute(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	cleaned := filepath.Clean(value)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%s must be absolute", label)
	}
	return cleaned, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
