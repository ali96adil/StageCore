package software_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestRepositoryImportsPrefersCompatibleAndRejectsIncompatible(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}

	compatiblePayload := []byte("stagecore-companion-compatible")
	compatible, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "stagecore-companion", Version: "0.1.0", Platform: "macos", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "StageCoreCompanion-0.1.0.pkg",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotarized,
		ReleaseChannel: store.SoftwareChannelRelease, ReleaseNotes: "M5 local release",
	}, bytes.NewReader(compatiblePayload))
	if err != nil {
		t.Fatal(err)
	}
	if compatible.ContentHash == "" || compatible.SizeBytes != int64(len(compatiblePayload)) {
		t.Fatalf("compatible package metadata=%#v", compatible)
	}

	incompatible, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "stagecore-companion", Version: "0.2.0", Platform: "macos", Architecture: "arm64",
		MinAPIVersion: 2, MaxAPIVersion: 3, OriginalFilename: "StageCoreCompanion-0.2.0.pkg",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotarized,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader([]byte("future-api-build")))
	if err != nil {
		t.Fatal(err)
	}

	packages, err := repository.List(ctx, "stagecore-companion", "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 {
		t.Fatalf("packages=%#v", packages)
	}
	if packages[0].Package.ID != compatible.ID || !packages[0].Compatible || !packages[0].Preferred || !packages[0].ProductionReady {
		t.Fatalf("preferred package=%#v", packages[0])
	}
	if packages[1].Package.ID != incompatible.ID || packages[1].Compatible || packages[1].Preferred {
		t.Fatalf("incompatible package=%#v", packages[1])
	}

	file, status, err := repository.OpenPackage(ctx, compatible.ID)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, compatiblePayload) || !status.Compatible {
		t.Fatalf("download bytes/status mismatch: %q %#v", read, status)
	}

	file, status, err = repository.OpenPackage(ctx, incompatible.ID)
	if file != nil {
		_ = file.Close()
	}
	if err == nil || status.Compatible {
		t.Fatalf("incompatible package was opened: status=%#v err=%v", status, err)
	}
}
