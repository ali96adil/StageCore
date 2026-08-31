package extension

import (
	"context"
	"fmt"
	"strings"
)

type ReadinessStatus string

type ReadinessCheckStatus string

const (
	ReadinessReadyForActivation ReadinessStatus = "READY_FOR_ACTIVATION"
	ReadinessNotReady           ReadinessStatus = "NOT_READY"

	ReadinessCheckPass          ReadinessCheckStatus = "PASS"
	ReadinessCheckBlocked       ReadinessCheckStatus = "BLOCKED"
	ReadinessCheckAdvisory      ReadinessCheckStatus = "ADVISORY"
	ReadinessCheckNotApplicable ReadinessCheckStatus = "NOT_APPLICABLE"
)

type ReadinessCheck struct {
	ID     string               `json:"id"`
	Status ReadinessCheckStatus `json:"status"`
	Code   string               `json:"code,omitempty"`
	Detail string               `json:"detail,omitempty"`
}

type ReadinessAssessment struct {
	InstallationID string               `json:"installation_id"`
	PackageID      string               `json:"package_id"`
	ExtensionID    string               `json:"extension_id"`
	Version        string               `json:"version"`
	Status         ReadinessStatus      `json:"status"`
	Checks         []ReadinessCheck     `json:"checks"`
	Advisories     []InstallPlanWarning `json:"advisories"`
}

type ReadinessAssessor struct {
	installer *Installer
	reviewer  *PermissionReviewer
}

func NewReadinessAssessor(installer *Installer, reviewer *PermissionReviewer) (*ReadinessAssessor, error) {
	if installer == nil || reviewer == nil {
		return nil, fmt.Errorf("readiness assessor requires installer and permission reviewer")
	}
	return &ReadinessAssessor{installer: installer, reviewer: reviewer}, nil
}

func (a *ReadinessAssessor) Assess(ctx context.Context, installationID string) (ReadinessAssessment, error) {
	if a == nil || a.installer == nil || a.reviewer == nil {
		return ReadinessAssessment{}, fmt.Errorf("extension readiness assessor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return ReadinessAssessment{}, fmt.Errorf("installation ID is required")
	}

	installed, err := a.installer.Get(ctx, installationID)
	if err != nil {
		return ReadinessAssessment{}, err
	}
	pkg, err := a.installer.library.Get(ctx, installed.PackageID)
	if err != nil {
		return ReadinessAssessment{}, err
	}
	assessment := ReadinessAssessment{
		InstallationID: installed.InstallationID,
		PackageID: installed.PackageID,
		ExtensionID: installed.ExtensionID,
		Version: installed.Version,
		Status: ReadinessReadyForActivation,
		Checks: make([]ReadinessCheck, 0, 5),
		Advisories: []InstallPlanWarning{},
	}
	assessment.Checks = append(assessment.Checks, ReadinessCheck{
		ID: "installed_integrity",
		Status: ReadinessCheckPass,
		Code: "INSTALLED_PAYLOAD_VERIFIED",
		Detail: "installed payload hash, size, file type and managed path were verified",
	})

	if pkg.Compatible {
		assessment.Checks = append(assessment.Checks, ReadinessCheck{
			ID: "package_compatibility",
			Status: ReadinessCheckPass,
			Code: "PACKAGE_COMPATIBLE",
			Detail: pkg.CompatibilityReason,
		})
	} else {
		assessment.block("package_compatibility", "PACKAGE_INCOMPATIBLE", pkg.CompatibilityReason)
	}
	if pkg.ProductionReady {
		assessment.Checks = append(assessment.Checks, ReadinessCheck{
			ID: "package_trust",
			Status: ReadinessCheckPass,
			Code: "PACKAGE_PRODUCTION_READY",
			Detail: "software package metadata satisfies the current production-ready policy",
		})
	} else {
		assessment.block("package_trust", "PACKAGE_NOT_PRODUCTION_READY", "software package is not production-ready under the current signing/release policy")
	}

	plan, err := a.installer.PlanInstall(ctx, installed.PackageID)
	if err != nil {
		return ReadinessAssessment{}, err
	}
	assessment.Advisories = append(assessment.Advisories, plan.Warnings...)
	if plan.RootAlreadyInstalled && plan.Status == InstallPlanReady && len(plan.Blockers) == 0 && len(plan.Steps) == 0 {
		assessment.Checks = append(assessment.Checks, ReadinessCheck{
			ID: "dependencies",
			Status: ReadinessCheckPass,
			Code: "REQUIRED_DEPENDENCIES_SATISFIED",
			Detail: "all required extension dependencies are installed at compatible versions",
		})
	} else {
		code := "REQUIRED_DEPENDENCIES_NOT_READY"
		detail := "required extension dependencies are not fully satisfied"
		if len(plan.Blockers) != 0 {
			code = plan.Blockers[0].Code
			if plan.Blockers[0].Detail != "" {
				detail = plan.Blockers[0].Detail
			}
		} else if plan.Status == InstallPlanRequiresDependencies {
			code = "REQUIRED_DEPENDENCIES_MISSING"
		}
		assessment.block("dependencies", code, detail)
	}

	review, err := a.reviewer.Get(ctx, installed.InstallationID)
	if err != nil {
		return ReadinessAssessment{}, err
	}
	switch review.Status {
	case PermissionReviewApproved:
		assessment.Checks = append(assessment.Checks, ReadinessCheck{
			ID: "permission_review",
			Status: ReadinessCheckPass,
			Code: "PERMISSIONS_APPROVED",
			Detail: "all permissions requested by this installation were explicitly approved",
		})
	case PermissionReviewNotRequired:
		assessment.Checks = append(assessment.Checks, ReadinessCheck{
			ID: "permission_review",
			Status: ReadinessCheckPass,
			Code: "PERMISSIONS_NOT_REQUIRED",
			Detail: "this installation requests no runtime plugin permissions",
		})
	case PermissionReviewDenied:
		assessment.block("permission_review", "PERMISSION_REVIEW_DENIED", "at least one requested permission was explicitly denied")
	default:
		assessment.block("permission_review", "PERMISSION_REVIEW_PENDING", "one or more requested permissions still require an explicit decision")
	}

	assessment.Checks = append(assessment.Checks, ReadinessCheck{
		ID: "runtime_health",
		Status: ReadinessCheckNotApplicable,
		Code: "ACTIVATION_NOT_IMPLEMENTED",
		Detail: "runtime process health is not evaluated before StageCore introduces an activation contract",
	})
	return assessment, nil
}

func (a *ReadinessAssessment) block(id, code, detail string) {
	a.Status = ReadinessNotReady
	a.Checks = append(a.Checks, ReadinessCheck{ID: id, Status: ReadinessCheckBlocked, Code: code, Detail: detail})
}
