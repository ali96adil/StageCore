package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestClockPreviewOwnerOnlyAndNoOSClockChange(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	fixed := time.Date(2026, 10, 17, 12, 15, 0, 0, time.UTC)
	handler := New(WithOperatorClockPreview(h.auth, stageStore, func() time.Time { return fixed })).Handler()
	requestJSON, _ := json.Marshal(hubClockPreviewRequest{
		Source: "PHONE", ProposedUnixMS: fixed.Add(13*time.Minute).UnixMilli(),
	})
	issue := func(token, csrf string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/clock/preview", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:17330"
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" { req.Header.Set(csrfHeader, csrf) }
		if token != "" { req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token}) }
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp
	}
	if res := issue("", "", requestJSON); res.Code != http.StatusUnauthorized && res.Code != http.StatusForbidden {
		t.Fatalf("anonymous preview status=%d body=%s", res.Code, res.Body.String())
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	if res := issue(owner.Token, "", requestJSON); res.Code != http.StatusForbidden {
		t.Fatalf("CSRF-less preview status=%d body=%s", res.Code, res.Body.String())
	}
	if res := issue(owner.Token, owner.CSRFToken, []byte(`{"source":"UNTRUSTED","proposed_unix_ms":1792240000000}`)); res.Code != http.StatusBadRequest {
		t.Fatalf("untrusted source preview status=%d body=%s", res.Code, res.Body.String())
	}
	if res := issue(owner.Token, owner.CSRFToken, []byte(`{"source":"PHONE","proposed_unix_ms":10}`)); res.Code != http.StatusBadRequest {
		t.Fatalf("bad timestamp preview status=%d body=%s", res.Code, res.Body.String())
	}
	success := issue(owner.Token, owner.CSRFToken, requestJSON)
	if success.Code != http.StatusOK { t.Fatalf("preview status=%d body=%s", success.Code, success.Body.String()) }
	if success.Header().Get("Cache-Control") != "no-store" { t.Fatal("clock preview must disable caching") }
	var preview hubClockPreviewResponse
	if err := json.Unmarshal(success.Body.Bytes(), &preview); err != nil { t.Fatal(err) }
	if !preview.PreviewOnly || preview.ApplyAvailable || !preview.RequiresConfirmation ||
		preview.RuntimeBlocked || preview.DifferenceMS != 13*60*1000 ||
		preview.ServerUnixMS != fixed.UnixMilli() || preview.Source != "PHONE" {
		t.Fatalf("unexpected safe dry-run response: %+v", preview)
	}

	// Permission is enforced on the current User role, not only by UI.
	if _, err := h.db.DB.ExecContext(ctx, "UPDATE local_users SET role = 'OPERATOR' WHERE username = 'owner'"); err != nil {
		t.Fatal(err)
	}
	denied := issue(owner.Token, owner.CSRFToken, requestJSON)
	if denied.Code != http.StatusForbidden { t.Fatalf("OPERATOR status=%d body=%s", denied.Code, denied.Body.String()) }
}

func TestClockPreviewMarksActiveRehearsalAsBlocked(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, rev, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Clock session guard", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	if err := stageStore.SetRevisionStatus(ctx, rev.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, rev.ID, "owner")
	if err != nil { t.Fatal(err) }
	active, err := stageStore.CreateSession(ctx, published.ID, domain.SessionRehearsal, "Clock guard test")
	if err != nil { t.Fatal(err) }
	defer stageStore.EndSession(ctx, active.ID, domain.SessionCompleted)
	_ = project

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	fixed := time.Date(2026, 10, 17, 12, 15, 0, 0, time.UTC)
	handler := New(WithOperatorClockPreview(h.auth, stageStore, func() time.Time { return fixed })).Handler()
	body, _ := json.Marshal(hubClockPreviewRequest{Source: "MANUAL", ProposedUnixMS: fixed.Add(-time.Hour).UnixMilli()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/clock/preview", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:17331"
	req.Header.Set(csrfHeader, owner.CSRFToken)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK { t.Fatalf("preview status=%d body=%s", res.Code, res.Body.String()) }
	var preview hubClockPreviewResponse
	if err := json.Unmarshal(res.Body.Bytes(), &preview); err != nil { t.Fatal(err) }
	if preview.ActiveSession != string(domain.SessionRehearsal) || !preview.RuntimeBlocked ||
		!preview.PreviewOnly || preview.ApplyAvailable || preview.DifferenceMS != -3600000 {
		t.Fatalf("active REHEARSAL preview did not enforce lock: %+v", preview)
	}
}
