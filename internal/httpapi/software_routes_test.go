package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestSoftwareBootstrapListsStreamsAndBlocksIncompatiblePackage(t *testing.T) {
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
	payload := []byte("stagecore-offline-macos-package")
	compatible, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "stagecore-companion", Version: "0.1.0", Platform: "macos", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "StageCoreCompanion.pkg",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotarized,
		ReleaseChannel: store.SoftwareChannelRelease, ReleaseNotes: "offline bootstrap",
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	incompatible, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "stagecore-companion", Version: "0.2.0", Platform: "macos", Architecture: "arm64",
		MinAPIVersion: 2, MaxAPIVersion: 2, OriginalFilename: "StageCoreCompanion-future.pkg",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotarized,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader([]byte("future-package")))
	if err != nil {
		t.Fatal(err)
	}

	api := httpapi.New(httpapi.WithSoftwareRepository(repository))

	metadataRequest := httptest.NewRequest(http.MethodGet, "/api/v1/software/packages?product_id=stagecore-companion&platform=macos&architecture=arm64", nil)
	metadataResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(metadataResponse, metadataRequest)
	if metadataResponse.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", metadataResponse.Code, metadataResponse.Body.String())
	}
	var metadata struct {
		HubAPIVersion int `json:"hub_api_version"`
		Packages []struct {
			PackageID string `json:"package_id"`
			Compatible bool `json:"compatible"`
			Preferred bool `json:"preferred"`
			ProductionReady bool `json:"production_ready"`
			SHA256 string `json:"sha256"`
			DownloadPath string `json:"download_path"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(metadataResponse.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.HubAPIVersion != 1 || len(metadata.Packages) != 2 {
		t.Fatalf("metadata=%#v", metadata)
	}
	if metadata.Packages[0].PackageID != compatible.ID || !metadata.Packages[0].Compatible || !metadata.Packages[0].Preferred || !metadata.Packages[0].ProductionReady || metadata.Packages[0].SHA256 != compatible.ContentHash {
		t.Fatalf("preferred metadata=%#v", metadata.Packages[0])
	}
	if metadata.Packages[1].PackageID != incompatible.ID || metadata.Packages[1].Compatible || metadata.Packages[1].Preferred || metadata.Packages[1].DownloadPath != "" {
		t.Fatalf("incompatible metadata=%#v", metadata.Packages[1])
	}

	setupRequest := httptest.NewRequest(http.MethodGet, "/downloads/setup?platform=macos&architecture=arm64", nil)
	setupResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(setupResponse, setupRequest)
	if setupResponse.Code != http.StatusOK || !strings.Contains(setupResponse.Body.String(), "PREFERRED") || !strings.Contains(setupResponse.Body.String(), "download blocked") {
		t.Fatalf("setup status=%d body=%s", setupResponse.Code, setupResponse.Body.String())
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/downloads/software/"+compatible.ID, nil)
	downloadRequest.Header.Set("Range", "bytes=5-14")
	downloadResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(downloadResponse, downloadRequest)
	if downloadResponse.Code != http.StatusPartialContent {
		t.Fatalf("download status=%d body=%s", downloadResponse.Code, downloadResponse.Body.String())
	}
	if !bytes.Equal(downloadResponse.Body.Bytes(), payload[5:15]) {
		t.Fatalf("range bytes=%q want=%q", downloadResponse.Body.Bytes(), payload[5:15])
	}
	if downloadResponse.Header().Get("X-Content-SHA256") != compatible.ContentHash {
		t.Fatalf("download checksum header=%q", downloadResponse.Header().Get("X-Content-SHA256"))
	}

	blockedRequest := httptest.NewRequest(http.MethodGet, "/downloads/software/"+incompatible.ID, nil)
	blockedResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(blockedResponse, blockedRequest)
	if blockedResponse.Code != http.StatusConflict || !strings.Contains(blockedResponse.Body.String(), "SOFTWARE_PACKAGE_INCOMPATIBLE") {
		t.Fatalf("incompatible download status=%d body=%s", blockedResponse.Code, blockedResponse.Body.String())
	}
}
