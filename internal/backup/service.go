package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	ManifestSchemaVersion = 1
	ManifestFileName      = "manifest.json"
	ManifestChecksumFile  = "manifest.sha256"
)

var (
	ErrActiveShow       = errors.New("restore is blocked while SHOW is active")
	ErrDestructiveTarget = errors.New("restore target must be a new data root")
	ErrIntegrity        = errors.New("backup integrity verification failed")
)

type DatabaseManifest struct {
	File          string `json:"file"`
	SizeBytes     int64  `json:"size_bytes"`
	SHA256        string `json:"sha256"`
	SchemaVersion int64  `json:"schema_version"`
}

type Manifest struct {
	SchemaVersion      int              `json:"schema_version"`
	BackupID           string           `json:"backup_id"`
	BackupType         string           `json:"backup_type"`
	CreatedAt          time.Time        `json:"created_at"`
	Database           DatabaseManifest `json:"database"`
	ProjectIDs         []string         `json:"project_ids"`
	RuntimeSnapshotIDs []string         `json:"runtime_snapshot_ids"`
	SessionIDs         []string         `json:"session_ids"`
}

type Record struct {
	ID       string
	Path     string
	Manifest Manifest
	Verified bool
}

type RestoreResult struct {
	DataRoot string
	Manifest Manifest
}

type Service struct {
	handle   *db.Handle
	store    *store.Store
	dataRoot string
	bulk     *bulk.Manager
	now      func() time.Time
}

func New(handle *db.Handle, s *store.Store, dataRoot string, manager *bulk.Manager) (*Service, error) {
	if handle == nil || handle.DB == nil || s == nil {
		return nil, fmt.Errorf("backup service requires database handle and store")
	}
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return nil, fmt.Errorf("backup service data root is required")
	}
	return &Service{handle: handle, store: s, dataRoot: filepath.Clean(dataRoot), bulk: manager, now: time.Now}, nil
}

func (s *Service) CreateStateBackup(ctx context.Context, destinationRoot string) (Record, error) {
	if s == nil || s.handle == nil {
		return Record{}, fmt.Errorf("backup service is unavailable")
	}
	destinationRoot = strings.TrimSpace(destinationRoot)
	if destinationRoot == "" {
		return Record{}, fmt.Errorf("backup destination is required")
	}

	jobID := ""
	if s.bulk != nil {
		total := int64(0)
		if info, err := os.Stat(s.handle.Path); err == nil {
			total = info.Size()
		}
		var err error
		jobID, err = s.bulk.Begin(ctx, bulk.KindBackup, total)
		if err != nil {
			return Record{}, err
		}
	}
	succeeded := false
	defer func() {
		if s.bulk == nil || jobID == "" {
			return
		}
		if succeeded {
			s.bulk.Complete(jobID)
		} else {
			s.bulk.Fail(jobID, "backup did not complete")
		}
	}()

	backupID, err := stageid.New()
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(destinationRoot, 0o750); err != nil {
		return Record{}, fmt.Errorf("create backup destination: %w", err)
	}
	staging := filepath.Join(destinationRoot, "."+backupID+".staging")
	final := filepath.Join(destinationRoot, backupID)
	if err := os.RemoveAll(staging); err != nil {
		return Record{}, fmt.Errorf("clear backup staging: %w", err)
	}
	if _, err := os.Stat(final); err == nil {
		return Record{}, fmt.Errorf("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("inspect backup destination: %w", err)
	}
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(staging)
		}
	}()

	databasePath := filepath.Join(staging, "db", db.FileName)
	if err := db.Backup(ctx, s.handle.DB, databasePath); err != nil {
		return Record{}, err
	}

	copied, err := db.Open(ctx, db.Config{DataRoot: staging})
	if err != nil {
		return Record{}, fmt.Errorf("open copied backup database: %w", err)
	}
	projects, err := queryIDs(ctx, copied.DB, `SELECT project_id FROM projects ORDER BY project_id`)
	if err != nil {
		_ = copied.Close()
		return Record{}, err
	}
	snapshots, err := queryIDs(ctx, copied.DB, `SELECT runtime_snapshot_id FROM runtime_snapshots ORDER BY runtime_snapshot_id`)
	if err != nil {
		_ = copied.Close()
		return Record{}, err
	}
	sessions, err := queryIDs(ctx, copied.DB, `SELECT session_id FROM sessions ORDER BY session_id`)
	if err != nil {
		_ = copied.Close()
		return Record{}, err
	}
	schemaVersion, err := db.SchemaVersion(copied.DB)
	if err != nil {
		_ = copied.Close()
		return Record{}, fmt.Errorf("read backup schema version: %w", err)
	}
	if err := copied.Close(); err != nil {
		return Record{}, fmt.Errorf("close copied backup database: %w", err)
	}

	databaseSize, databaseHash, err := fileDigest(databasePath)
	if err != nil {
		return Record{}, err
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		BackupID: backupID,
		BackupType: "STATE",
		CreatedAt: s.now().UTC(),
		Database: DatabaseManifest{
			File: filepath.ToSlash(filepath.Join("db", db.FileName)),
			SizeBytes: databaseSize,
			SHA256: databaseHash,
			SchemaVersion: schemaVersion,
		},
		ProjectIDs: projects,
		RuntimeSnapshotIDs: snapshots,
		SessionIDs: sessions,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Record{}, fmt.Errorf("encode backup manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestPath := filepath.Join(staging, ManifestFileName)
	if err := os.WriteFile(manifestPath, manifestBytes, 0o640); err != nil {
		return Record{}, fmt.Errorf("write backup manifest: %w", err)
	}
	manifestHash := sha256.Sum256(manifestBytes)
	if err := os.WriteFile(filepath.Join(staging, ManifestChecksumFile), []byte(hex.EncodeToString(manifestHash[:])+"\n"), 0o640); err != nil {
		return Record{}, fmt.Errorf("write backup manifest checksum: %w", err)
	}
	if _, err := verifyDirectory(staging); err != nil {
		return Record{}, err
	}
	if err := os.Rename(staging, final); err != nil {
		return Record{}, fmt.Errorf("promote verified backup: %w", err)
	}
	succeeded = true
	return Record{ID: backupID, Path: final, Manifest: manifest, Verified: true}, nil
}

func (s *Service) Verify(backupPath string) (Manifest, error) {
	return verifyDirectory(filepath.Clean(strings.TrimSpace(backupPath)))
}

func (s *Service) Restore(ctx context.Context, backupPath, targetDataRoot string) (RestoreResult, error) {
	if s == nil || s.store == nil {
		return RestoreResult{}, fmt.Errorf("backup service is unavailable")
	}
	manifest, err := s.Verify(backupPath)
	if err != nil {
		return RestoreResult{}, err
	}
	mode, err := s.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return RestoreResult{}, err
	}
	if mode == domain.SessionShow {
		return RestoreResult{}, ErrActiveShow
	}

	sourceRoot, err := filepath.Abs(s.dataRoot)
	if err != nil {
		return RestoreResult{}, err
	}
	targetDataRoot = strings.TrimSpace(targetDataRoot)
	if targetDataRoot == "" {
		return RestoreResult{}, fmt.Errorf("restore target data root is required")
	}
	targetRoot, err := filepath.Abs(filepath.Clean(targetDataRoot))
	if err != nil {
		return RestoreResult{}, err
	}
	if targetRoot == sourceRoot {
		return RestoreResult{}, ErrDestructiveTarget
	}
	if _, err := os.Stat(targetRoot); err == nil {
		return RestoreResult{}, ErrDestructiveTarget
	} else if !errors.Is(err, os.ErrNotExist) {
		return RestoreResult{}, fmt.Errorf("inspect restore target: %w", err)
	}

	jobID := ""
	if s.bulk != nil {
		jobID, err = s.bulk.Begin(ctx, bulk.KindRestore, manifest.Database.SizeBytes)
		if err != nil {
			return RestoreResult{}, err
		}
	}
	succeeded := false
	defer func() {
		if s.bulk == nil || jobID == "" {
			return
		}
		if succeeded {
			s.bulk.Complete(jobID)
		} else {
			s.bulk.Fail(jobID, "restore did not complete")
		}
	}()

	restoreID, err := stageid.New()
	if err != nil {
		return RestoreResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetRoot), 0o750); err != nil {
		return RestoreResult{}, fmt.Errorf("create restore parent: %w", err)
	}
	staging := targetRoot + ".stagecore-restore-" + restoreID
	_ = os.RemoveAll(staging)
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(staging)
		}
	}()

	sourceDatabase := filepath.Join(filepath.Clean(backupPath), filepath.FromSlash(manifest.Database.File))
	targetDatabase := filepath.Join(staging, "db", db.FileName)
	if err := copyVerifiedFile(sourceDatabase, targetDatabase, manifest.Database.SizeBytes, manifest.Database.SHA256, func(n int64) {
		if s.bulk != nil && jobID != "" {
			s.bulk.Advance(jobID, n)
		}
	}); err != nil {
		return RestoreResult{}, err
	}

	restored, err := db.Open(ctx, db.Config{DataRoot: staging})
	if err != nil {
		return RestoreResult{}, fmt.Errorf("open restored database in staging: %w", err)
	}
	if err := verifyManifestIdentities(ctx, restored.DB, manifest); err != nil {
		_ = restored.Close()
		return RestoreResult{}, err
	}
	if err := restored.Close(); err != nil {
		return RestoreResult{}, fmt.Errorf("close restored staging database: %w", err)
	}
	if err := os.Rename(staging, targetRoot); err != nil {
		return RestoreResult{}, fmt.Errorf("promote restored data root: %w", err)
	}
	succeeded = true
	return RestoreResult{DataRoot: targetRoot, Manifest: manifest}, nil
}

func verifyDirectory(root string) (Manifest, error) {
	if root == "" || root == "." {
		return Manifest{}, fmt.Errorf("backup path is required")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, ManifestFileName))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	expectedManifestHashBytes, err := os.ReadFile(filepath.Join(root, ManifestChecksumFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest checksum: %w", err)
	}
	expectedManifestHash := strings.TrimSpace(string(expectedManifestHashBytes))
	actualManifestHash := sha256.Sum256(manifestBytes)
	if !strings.EqualFold(expectedManifestHash, hex.EncodeToString(actualManifestHash[:])) {
		return Manifest{}, fmt.Errorf("%w: manifest checksum mismatch", ErrIntegrity)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	if manifest.SchemaVersion != ManifestSchemaVersion || manifest.BackupType != "STATE" || manifest.Database.File != filepath.ToSlash(filepath.Join("db", db.FileName)) {
		return Manifest{}, fmt.Errorf("%w: unsupported backup manifest", ErrIntegrity)
	}
	databasePath := filepath.Join(root, filepath.FromSlash(manifest.Database.File))
	size, hash, err := fileDigest(databasePath)
	if err != nil {
		return Manifest{}, err
	}
	if size != manifest.Database.SizeBytes || !strings.EqualFold(hash, manifest.Database.SHA256) {
		return Manifest{}, fmt.Errorf("%w: database checksum or size mismatch", ErrIntegrity)
	}
	return manifest, nil
}

func queryIDs(ctx context.Context, database *sql.DB, query string) ([]string, error) {
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query backup identities: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan backup identity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

func verifyManifestIdentities(ctx context.Context, database *sql.DB, manifest Manifest) error {
	checks := []struct {
		query string
		ids   []string
	}{
		{`SELECT project_id FROM projects ORDER BY project_id`, manifest.ProjectIDs},
		{`SELECT runtime_snapshot_id FROM runtime_snapshots ORDER BY runtime_snapshot_id`, manifest.RuntimeSnapshotIDs},
		{`SELECT session_id FROM sessions ORDER BY session_id`, manifest.SessionIDs},
	}
	for _, check := range checks {
		got, err := queryIDs(ctx, database, check.query)
		if err != nil {
			return err
		}
		if len(got) != len(check.ids) {
			return fmt.Errorf("%w: restored identity count mismatch", ErrIntegrity)
		}
		for i := range got {
			if got[i] != check.ids[i] {
				return fmt.Errorf("%w: restored identities do not match manifest", ErrIntegrity)
			}
		}
	}
	return nil
}

func fileDigest(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("open file for checksum: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return 0, "", fmt.Errorf("checksum file: %w", err)
	}
	return size, hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyVerifiedFile(source, destination string, expectedSize int64, expectedHash string, progress func(int64)) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open backup database: %w", err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return fmt.Errorf("create restore staging directory: %w", err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("create restored database: %w", err)
	}
	remove := true
	defer func() {
		_ = output.Close()
		if remove {
			_ = os.Remove(destination)
		}
	}()
	hasher := sha256.New()
	buffer := make([]byte, 4<<20)
	var total int64
	for {
		n, readErr := input.Read(buffer)
		if n > 0 {
			written, writeErr := io.MultiWriter(output, hasher).Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("write restored database: %w", writeErr)
			}
			if written != n {
				return io.ErrShortWrite
			}
			total += int64(written)
			if progress != nil {
				progress(int64(written))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read backup database: %w", readErr)
		}
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync restored database: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close restored database: %w", err)
	}
	if total != expectedSize || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expectedHash) {
		return fmt.Errorf("%w: restored database checksum or size mismatch", ErrIntegrity)
	}
	remove = false
	return nil
}
