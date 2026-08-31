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

func TestExtensionInstallRejectedDuringShowAndAllowedAfterExit(t *testing.T) {
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
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("show-gated install payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(s, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
		"schema_version":1,
		"extension_id":"example.osc-plugin",
		"version":"1.2.3",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Example OSC Plugin","ar-IQ":"إضافة OSC تجريبية"},
		"summary":{"en":"Sends OSC messages.","ar-IQ":"ترسل رسائل OSC إلى أجهزة المسرح."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"],
		"capabilities":["osc.send"]
	}`)
	if _, err := library.Register(ctx, pkg.ID, manifest, "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "extension install gate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(ctx, pkg.ID, "owner"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("install during SHOW err=%v", err)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	installed, err := installer.Install(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if installed.LifecycleState != store.ExtensionInstallationInstalled {
		t.Fatalf("installed after SHOW=%+v", installed)
	}
}
