package extension

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type PermissionReviewStatus string

type PermissionDecision string

const (
	PermissionReviewNotRequired PermissionReviewStatus = "NOT_REQUIRED"
	PermissionReviewPending     PermissionReviewStatus = "PENDING"
	PermissionReviewApproved    PermissionReviewStatus = "APPROVED"
	PermissionReviewDenied      PermissionReviewStatus = "DENIED"

	PermissionDecisionPending  PermissionDecision = "PENDING"
	PermissionDecisionApproved PermissionDecision = "APPROVED"
	PermissionDecisionDenied   PermissionDecision = "DENIED"
)

var (
	ErrPermissionNotRequested   = errors.New("permission was not requested by the installed extension")
	ErrPermissionReviewIntegrity = errors.New("extension permission review failed integrity validation")
)

type PermissionReviewItem struct {
	Permission string             `json:"permission"`
	Decision   PermissionDecision `json:"decision"`
	ReviewedBy string             `json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time         `json:"reviewed_at,omitempty"`
}

type PermissionReview struct {
	InstallationID string                 `json:"installation_id"`
	PackageID      string                 `json:"package_id"`
	ExtensionID    string                 `json:"extension_id"`
	Version        string                 `json:"version"`
	Status         PermissionReviewStatus `json:"status"`
	Items          []PermissionReviewItem `json:"items"`
}

type PermissionReviewer struct {
	installer *Installer
}

func NewPermissionReviewer(installer *Installer) (*PermissionReviewer, error) {
	if installer == nil || installer.library == nil || installer.library.store == nil {
		return nil, fmt.Errorf("permission reviewer requires an extension installer")
	}
	return &PermissionReviewer{installer: installer}, nil
}

func (r *PermissionReviewer) Get(ctx context.Context, installationID string) (PermissionReview, error) {
	if r == nil || r.installer == nil || r.installer.library == nil {
		return PermissionReview{}, fmt.Errorf("extension permission reviewer is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return PermissionReview{}, fmt.Errorf("installation ID is required")
	}
	installed, err := r.installer.Get(ctx, installationID)
	if err != nil {
		return PermissionReview{}, err
	}
	pkg, err := r.installer.library.Get(ctx, installed.PackageID)
	if err != nil {
		return PermissionReview{}, err
	}
	requested := append([]string(nil), pkg.Manifest.Permissions...)
	sort.Strings(requested)
	decisions, err := r.installer.library.store.ListExtensionPermissionDecisions(ctx, installationID)
	if err != nil {
		return PermissionReview{}, err
	}
	requestedSet := make(map[string]struct{}, len(requested))
	for _, permission := range requested {
		requestedSet[permission] = struct{}{}
	}
	decisionByPermission := make(map[string]store.ExtensionPermissionDecision, len(decisions))
	for _, decision := range decisions {
		if _, ok := requestedSet[decision.Permission]; !ok {
			return PermissionReview{}, fmt.Errorf("%w: stored decision exists for unrequested permission %q", ErrPermissionReviewIntegrity, decision.Permission)
		}
		decisionByPermission[decision.Permission] = decision
	}

	review := PermissionReview{
		InstallationID: installed.InstallationID,
		PackageID: installed.PackageID,
		ExtensionID: installed.ExtensionID,
		Version: installed.Version,
		Items: make([]PermissionReviewItem, 0, len(requested)),
	}
	if len(requested) == 0 {
		review.Status = PermissionReviewNotRequired
		return review, nil
	}

	anyPending := false
	anyDenied := false
	for _, permission := range requested {
		item := PermissionReviewItem{Permission: permission, Decision: PermissionDecisionPending}
		if decision, ok := decisionByPermission[permission]; ok {
			switch decision.Decision {
			case store.ExtensionPermissionApproved:
				item.Decision = PermissionDecisionApproved
			case store.ExtensionPermissionDenied:
				item.Decision = PermissionDecisionDenied
				anyDenied = true
			default:
				return PermissionReview{}, fmt.Errorf("%w: stored permission decision %q is unsupported", ErrPermissionReviewIntegrity, decision.Decision)
			}
			item.ReviewedBy = decision.ReviewedBy
			reviewedAt := decision.ReviewedAt
			item.ReviewedAt = &reviewedAt
		} else {
			anyPending = true
		}
		review.Items = append(review.Items, item)
	}
	switch {
	case anyDenied:
		review.Status = PermissionReviewDenied
	case anyPending:
		review.Status = PermissionReviewPending
	default:
		review.Status = PermissionReviewApproved
	}
	return review, nil
}

func (r *PermissionReviewer) Decide(ctx context.Context, installationID, permission string, decision PermissionDecision, actor string) (PermissionReview, error) {
	if r == nil || r.installer == nil || r.installer.library == nil || r.installer.library.store == nil {
		return PermissionReview{}, fmt.Errorf("extension permission reviewer is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	permission = strings.TrimSpace(permission)
	actor = strings.TrimSpace(actor)
	decision = PermissionDecision(strings.ToUpper(strings.TrimSpace(string(decision))))
	if installationID == "" || permission == "" || actor == "" {
		return PermissionReview{}, fmt.Errorf("installation ID, permission and actor are required")
	}
	if decision != PermissionDecisionApproved && decision != PermissionDecisionDenied {
		return PermissionReview{}, fmt.Errorf("permission decision must be APPROVED or DENIED")
	}
	activeType, err := r.installer.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return PermissionReview{}, err
	}
	if activeType == domain.SessionShow {
		return PermissionReview{}, domain.ErrShowConfigurationLocked
	}

	current, err := r.Get(ctx, installationID)
	if err != nil {
		return PermissionReview{}, err
	}
	requested := false
	for _, item := range current.Items {
		if item.Permission == permission {
			requested = true
			break
		}
	}
	if !requested {
		return PermissionReview{}, ErrPermissionNotRequested
	}
	storeDecision := store.ExtensionPermissionDenied
	if decision == PermissionDecisionApproved {
		storeDecision = store.ExtensionPermissionApproved
	}
	if _, err := r.installer.library.store.SetExtensionPermissionDecision(ctx, installationID, permission, storeDecision, actor); err != nil {
		return PermissionReview{}, err
	}
	return r.Get(ctx, installationID)
}
