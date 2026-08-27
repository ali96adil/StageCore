package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestSoftwareDownloadBlockedInShowAndAvailableAfterExit(t *testing.T) {
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
	payload := []byte("show-policy-software-package")
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "stagecore-companion", Version: "0.1.0", Platform: "macos", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "StageCoreCompanion.pkg",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotarized,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	var show atomic.Bool
	manager := bulk.New(func(context.Context) (bulk.Mode, error) {
		if show.Load() {
			return bulk.ModeShow, nil
		}
		return bulk.ModeEdit, nil
	})
	api := httpapi.New(httpapi.WithSoftwareRepository(repository), httpapi.WithBulkManager(manager))

	show.Store(true)
	blocked := httptest.NewRecorder()
	api.Handler().ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/downloads/software/"+pkg.ID, nil))
	if blocked.Code != http.StatusLocked || !strings.Contains(blocked.Body.String(), "BULK_TRANSFER_BLOCKED_SHOW") {
		t.Fatalf("SHOW software download status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	show.Store(false)
	allowed := httptest.NewRecorder()
	api.Handler().ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/downloads/software/"+pkg.ID, nil))
	if allowed.Code != http.StatusOK || !bytes.Equal(allowed.Body.Bytes(), payload) {
		t.Fatalf("post-SHOW software download status=%d body=%q", allowed.Code, allowed.Body.Bytes())
	}
}
