package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestExtensionPermissionReviewRejectedDuringShowAndAllowedAfterExit(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	v, err := vault.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "review.show-plugin", Version: "1.0.0", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "review-show-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("permission review SHOW payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(s, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
		"schema_version":1,
		"extension_id":"review.show-plugin",
		"version":"1.0.0",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Review Show Plugin","ar-IQ":"إضافة اختبار مراجعة العرض"},
		"summary":{"en":"Tests permission review during SHOW.","ar-IQ":"تختبر مراجعة الصلاحيات أثناء العرض."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"]
	}`)
	if _, err := library.Register(ctx, pkg.ID, manifest, "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.InstallPlanned(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := extension.NewPermissionReviewer(installer)
	if err != nil {
		t.Fatal(err)
	}

	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "extension permission review gate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewer.Decide(ctx, installed.InstallationID, "network.udp.send", extension.PermissionDecisionApproved, "owner"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("permission review during SHOW err=%v", err)
	}
	pending, err := reviewer.Get(ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != extension.PermissionReviewPending {
		t.Fatalf("review changed during SHOW: %+v", pending)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	approved, err := reviewer.Decide(ctx, installed.InstallationID, "network.udp.send", extension.PermissionDecisionApproved, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != extension.PermissionReviewApproved {
		t.Fatalf("review after SHOW=%+v", approved)
	}
}
