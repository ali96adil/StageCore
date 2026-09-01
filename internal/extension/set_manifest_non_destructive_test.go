package extension

import (
	"context"
	"testing"
)

func TestExtensionSetRestoreDoesNotPruneUnrelatedInstallations(t *testing.T) {
	ctx := context.Background()
	source := newExtensionSetTestRig(t, ctx)
	defer source.close()
	target := newExtensionSetTestRig(t, ctx)
	defer target.close()

	wantedManifest := extensionSetAddonManifest(t, "example.wanted-addon", nil)
	extraManifest := extensionSetAddonManifest(t, "example.extra-addon", nil)

	sourceWanted := extensionSetRegisterPackage(t, ctx, source, "example.wanted-addon", []byte("wanted payload"), wantedManifest)
	if _, err := source.installer.InstallPlanned(ctx, sourceWanted.PackageID, "source-owner"); err != nil {
		t.Fatal(err)
	}
	_, raw, err := source.service.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}

	targetWanted := extensionSetRegisterPackage(t, ctx, target, "example.wanted-addon", []byte("wanted payload"), wantedManifest)
	targetExtra := extensionSetRegisterPackage(t, ctx, target, "example.extra-addon", []byte("extra payload"), extraManifest)
	extraInstallation, err := target.installer.InstallPlanned(ctx, targetExtra.PackageID, "target-owner")
	if err != nil {
		t.Fatal(err)
	}

	plan, err := target.service.PlanRestore(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != ExtensionSetRestoreReady || len(plan.Steps) != 1 || plan.Steps[0].PackageID != targetWanted.PackageID {
		t.Fatalf("restore plan=%+v", plan)
	}

	result, err := target.service.Restore(ctx, raw, "target-owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Installed) != 1 || result.Installed[0].ExtensionID != "example.wanted-addon" {
		t.Fatalf("restore result=%+v", result)
	}

	installations, err := target.installer.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 2 {
		t.Fatalf("restore pruned or duplicated installations: %+v", installations)
	}
	foundExtra := false
	for _, installation := range installations {
		if installation.InstallationID == extraInstallation.InstallationID && installation.ExtensionID == "example.extra-addon" {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Fatalf("unrelated installation was removed: %+v", installations)
	}

	noop, err := target.service.PlanRestore(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if noop.Status != ExtensionSetRestoreNoop {
		t.Fatalf("satisfied manifest with unrelated installation should be NOOP: %+v", noop)
	}
}
