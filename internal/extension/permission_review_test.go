package extension

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

func registerPermissionReviewPackage(t *testing.T, h *dependencyTestHarness, permissions []string) Package {
	t.Helper()
	const extensionID = "review.example"
	const version = "1.0.0"
	softwarePackage, err := h.repository.ImportPackage(h.ctx, software.ImportParams{
		ProductID: extensionID,
		Version: version,
		Platform: "linux",
		Architecture: "arm64",
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: "review-example",
		SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("permission review payload"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: extensionID,
		Version: version,
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Review Example", ArIQ: "مثال مراجعة الصلاحيات"},
		Summary: LocalizedText{EN: "Exercises permission review.", ArIQ: "يختبر مراجعة صلاحيات الإضافة."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Permissions: permissions,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := h.library.Register(h.ctx, softwarePackage.ID, raw, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestPermissionReviewerRequiresExplicitReviewWithoutRuntimeGrant(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerPermissionReviewPackage(t, h, []string{"network.udp.send", "network.udp.listen"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}

	pending, err := reviewer.Get(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != PermissionReviewPending || len(pending.Items) != 2 {
		t.Fatalf("pending review=%+v", pending)
	}
	for _, item := range pending.Items {
		if item.Decision != PermissionDecisionPending || item.ReviewedBy != "" || item.ReviewedAt != nil {
			t.Fatalf("unexpected pending item=%+v", item)
		}
	}

	partiallyApproved, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionApproved, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if partiallyApproved.Status != PermissionReviewPending {
		t.Fatalf("partial status=%s want PENDING", partiallyApproved.Status)
	}
	approved, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.listen", PermissionDecisionApproved, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != PermissionReviewApproved {
		t.Fatalf("approved status=%s want APPROVED", approved.Status)
	}

	permissionService, err := pluginpermissions.New(h.handle.DB)
	if err != nil {
		t.Fatal(err)
	}
	granted, err := permissionService.Granted(h.ctx, pkg.Manifest.ExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 0 {
		t.Fatalf("permission review leaked runtime grants: %v", granted)
	}

	reloadedReviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedReviewer.Get(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != PermissionReviewApproved || len(reloaded.Items) != 2 {
		t.Fatalf("reloaded review=%+v", reloaded)
	}
}

func TestPermissionReviewerDenialAndUnrequestedPermission(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerPermissionReviewPackage(t, h, []string{"network.udp.send"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	denied, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionDenied, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if denied.Status != PermissionReviewDenied {
		t.Fatalf("denied status=%s", denied.Status)
	}
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "filesystem.write", PermissionDecisionApproved, "owner"); err != ErrPermissionNotRequested {
		t.Fatalf("unrequested permission err=%v", err)
	}
}

func TestPermissionReviewerNotRequiredWithoutRequestedPermissions(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerPermissionReviewPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	review, err := reviewer.Get(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != PermissionReviewNotRequired || len(review.Items) != 0 {
		t.Fatalf("no-permission review=%+v", review)
	}
}
