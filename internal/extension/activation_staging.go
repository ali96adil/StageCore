package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/domain"
)

const activationStagingFileMode = 0o400

var (
	ErrActivationNotReady        = errors.New("extension is not ready for activation")
	ErrActivationStagingIntegrity = errors.New("extension activation staging failed integrity verification")
)

const ActivationExecutionIsolationRequired = "RUNTIME_EXECUTION_ISOLATION_REQUIRED"

type ActivationNotReadyError struct {
	Assessment ReadinessAssessment `json:"assessment"`
}

func (e *ActivationNotReadyError) Error() string { return ErrActivationNotReady.Error() }
func (e *ActivationNotReadyError) Unwrap() error { return ErrActivationNotReady }

type ActivationStagingResult struct {
	InstallationID     string   `json:"installation_id"`
	PackageID          string   `json:"package_id"`
	ExtensionID        string   `json:"extension_id"`
	Version            string   `json:"version"`
	Status             string   `json:"status"`
	Platform           string   `json:"platform"`
	Architecture       string   `json:"architecture"`
	ContentSHA256      string   `json:"content_sha256"`
	RuntimePermissions []string `json:"runtime_permissions"`
	ExecutionAuthorized bool    `json:"execution_authorized"`
	ExecutionBlocker   string   `json:"execution_blocker"`
}

type ActivationStager struct {
	installer *Installer
	reviewer  *PermissionReviewer
	assessor  *ReadinessAssessor
	stageRoot string
	mu        sync.Mutex
}

func NewActivationStager(installer *Installer, reviewer *PermissionReviewer, assessor *ReadinessAssessor) (*ActivationStager, error) {
	if installer == nil || reviewer == nil || assessor == nil || installer.library == nil || installer.library.store == nil {
		return nil, fmt.Errorf("activation stager requires installer, permission reviewer and readiness assessor")
	}
	runtimeRoot := filepath.Join(installer.root, "runtime")
	stageRoot := filepath.Join(runtimeRoot, "staging-check")
	if err := ensureManagedDirectory(runtimeRoot); err != nil {
		return nil, err
	}
	if err := ensureManagedDirectory(stageRoot); err != nil {
		return nil, err
	}
	stager := &ActivationStager{
		installer: installer,
		reviewer: reviewer,
		assessor: assessor,
		stageRoot: stageRoot,
	}
	if err := stager.cleanStageRoot(); err != nil {
		return nil, err
	}
	return stager, nil
}

func (s *ActivationStager) Check(ctx context.Context, installationID string) (ActivationStagingResult, error) {
	if s == nil || s.installer == nil || s.reviewer == nil || s.assessor == nil {
		return ActivationStagingResult{}, fmt.Errorf("extension activation stager is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return ActivationStagingResult{}, fmt.Errorf("installation ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rejectShow(ctx); err != nil {
		return ActivationStagingResult{}, err
	}
	assessment, err := s.assessor.Assess(ctx, installationID)
	if err != nil {
		return ActivationStagingResult{}, err
	}
	if assessment.Status != ReadinessReadyForActivation {
		return ActivationStagingResult{}, &ActivationNotReadyError{Assessment: assessment}
	}

	installed, err := s.installer.Get(ctx, installationID)
	if err != nil {
		return ActivationStagingResult{}, err
	}
	artifact, err := s.installer.InspectRuntimeArtifact(ctx, installationID)
	if err != nil {
		return ActivationStagingResult{}, err
	}
	if err := runtimeHostCompatibility(artifact.Platform, artifact.Architecture); err != nil {
		return ActivationStagingResult{}, &ActivationNotReadyError{Assessment: assessment}
	}
	permissions, err := s.approvedRuntimePermissions(ctx, installed)
	if err != nil {
		if errors.Is(err, ErrActivationNotReady) {
			return ActivationStagingResult{}, &ActivationNotReadyError{Assessment: assessment}
		}
		return ActivationStagingResult{}, err
	}

	stagedPath, err := s.stageReadOnlyCopy(ctx, installed, artifact)
	if err != nil {
		return ActivationStagingResult{}, err
	}
	cleanup := func() error { return s.removeStagedCopy(stagedPath) }

	if err := s.rejectShow(ctx); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return ActivationStagingResult{}, fmt.Errorf("activation staging cleanup failed: %v after %v", cleanupErr, err)
		}
		return ActivationStagingResult{}, err
	}
	if cleanupErr := cleanup(); cleanupErr != nil {
		return ActivationStagingResult{}, fmt.Errorf("activation staging cleanup failed: %w", cleanupErr)
	}

	return ActivationStagingResult{
		InstallationID: installed.InstallationID,
		PackageID: installed.PackageID,
		ExtensionID: installed.ExtensionID,
		Version: installed.Version,
		Status: "STAGING_VERIFIED",
		Platform: artifact.Platform,
		Architecture: artifact.Architecture,
		ContentSHA256: installed.ContentSHA256,
		RuntimePermissions: permissions,
		ExecutionAuthorized: false,
		ExecutionBlocker: ActivationExecutionIsolationRequired,
	}, nil
}

func (s *ActivationStager) rejectShow(ctx context.Context) error {
	activeType, err := s.installer.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return err
	}
	if activeType == domain.SessionShow {
		return domain.ErrShowConfigurationLocked
	}
	return nil
}

func (s *ActivationStager) approvedRuntimePermissions(ctx context.Context, installed Installation) ([]string, error) {
	review, err := s.reviewer.Get(ctx, installed.InstallationID)
	if err != nil {
		return nil, err
	}
	if review.PackageID != installed.PackageID || review.ExtensionID != installed.ExtensionID || review.Version != installed.Version {
		return nil, fmt.Errorf("%w: permission review identity does not match installation", ErrPermissionReviewIntegrity)
	}
	if review.Status == PermissionReviewNotRequired {
		return []string{}, nil
	}
	if review.Status != PermissionReviewApproved {
		return nil, ErrActivationNotReady
	}
	permissions := make([]string, 0, len(review.Items))
	for _, item := range review.Items {
		if item.Decision != PermissionDecisionApproved {
			return nil, ErrActivationNotReady
		}
		permissions = append(permissions, item.Permission)
	}
	sort.Strings(permissions)
	return permissions, nil
}

func (s *ActivationStager) stageReadOnlyCopy(ctx context.Context, installed Installation, artifact RuntimeArtifact) (string, error) {
	if err := inspectManagedDirectory(s.stageRoot); err != nil {
		return "", err
	}
	source, err := os.Open(artifact.Path)
	if err != nil {
		return "", fmt.Errorf("%w: open immutable runtime artifact: %v", ErrActivationStagingIntegrity, err)
	}
	defer source.Close()

	staged, err := os.CreateTemp(s.stageRoot, "stage-*.bin")
	if err != nil {
		return "", fmt.Errorf("create activation staging file: %w", err)
	}
	stagedPath := staged.Name()
	keep := false
	defer func() {
		_ = staged.Close()
		if !keep {
			_ = os.Remove(stagedPath)
		}
	}()

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(staged, hasher), source)
	if copyErr != nil {
		return "", fmt.Errorf("%w: copy runtime artifact: %v", ErrActivationStagingIntegrity, copyErr)
	}
	if written != installed.SizeBytes {
		return "", fmt.Errorf("%w: staged runtime size mismatch", ErrActivationStagingIntegrity)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != strings.ToLower(strings.TrimSpace(installed.ContentSHA256)) {
		return "", fmt.Errorf("%w: staged runtime SHA-256 mismatch", ErrActivationStagingIntegrity)
	}
	if err := staged.Sync(); err != nil {
		return "", fmt.Errorf("sync activation staging file: %w", err)
	}
	if err := staged.Chmod(activationStagingFileMode); err != nil {
		return "", fmt.Errorf("set activation staging read-only mode: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close activation staging file: %w", err)
	}
	if _, err := s.installer.Get(ctx, installed.InstallationID); err != nil {
		return "", err
	}
	if err := verifyActivationStagingFile(stagedPath, installed.ContentSHA256, installed.SizeBytes); err != nil {
		return "", err
	}
	if err := syncActivationDirectory(s.stageRoot); err != nil {
		return "", err
	}
	keep = true
	return stagedPath, nil
}

func (s *ActivationStager) removeStagedCopy(filename string) error {
	if filename == "" {
		return nil
	}
	relative, err := filepath.Rel(s.stageRoot, filepath.Clean(filename))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != relative {
		return fmt.Errorf("activation staging cleanup path escaped managed root")
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncActivationDirectory(s.stageRoot)
}

func (s *ActivationStager) cleanStageRoot() error {
	if err := inspectManagedDirectory(s.stageRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.stageRoot)
	if err != nil {
		return fmt.Errorf("read activation staging directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "stage-") || !strings.HasSuffix(entry.Name(), ".bin") {
			return fmt.Errorf("unexpected entry in managed activation staging directory: %s", entry.Name())
		}
		fullPath := filepath.Join(s.stageRoot, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected activation staging entry: %s", entry.Name())
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("remove stale activation staging file: %w", err)
		}
	}
	return syncActivationDirectory(s.stageRoot)
}

func verifyActivationStagingFile(filename, expectedHash string, expectedSize int64) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrActivationStagingIntegrity, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != activationStagingFileMode || info.Size() != expectedSize {
		return fmt.Errorf("%w: staged runtime metadata mismatch", ErrActivationStagingIntegrity)
	}
	if info.Mode().Perm()&0o111 != 0 {
		return fmt.Errorf("%w: staged runtime must remain non-executable", ErrActivationStagingIntegrity)
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("%w: open staged runtime artifact: %v", ErrActivationStagingIntegrity, err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("%w: hash staged runtime artifact: %v", ErrActivationStagingIntegrity, err)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(strings.TrimSpace(expectedHash)) {
		return fmt.Errorf("%w: staged runtime SHA-256 mismatch", ErrActivationStagingIntegrity)
	}
	return nil
}

func syncActivationDirectory(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open activation staging directory for sync: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync activation staging directory: %w", err)
	}
	return nil
}
