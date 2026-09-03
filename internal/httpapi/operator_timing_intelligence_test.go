package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
)

type fakeTimingIntelligenceService struct {
	reportCalls       int
	lastProjectID     string
	lastOptions       timingintelligence.ReportOptions
	selectionCalls    int
	lastSessionID     string
	lastSelectionMode store.TimingSelectionMode
	lastUpdatedBy     string
}

func (f *fakeTimingIntelligenceService) Report(_ context.Context, projectID string, options timingintelligence.ReportOptions) (timingintelligence.Report, error) {
	f.reportCalls++
	f.lastProjectID = projectID
	f.lastOptions = options
	return timingintelligence.Report{ProjectID: projectID, AdvisoryOnly: true}, nil
}

func (f *fakeTimingIntelligenceService) SetSessionSelection(_ context.Context, projectID, sessionID string, mode store.TimingSelectionMode, updatedBy string) (store.TimingSessionSelection, error) {
	f.selectionCalls++
	f.lastProjectID = projectID
	f.lastSessionID = sessionID
	f.lastSelectionMode = mode
	f.lastUpdatedBy = updatedBy
	return store.TimingSessionSelection{SessionID: sessionID, Mode: mode, UpdatedBy: updatedBy}, nil
}

func TestTimingIntelligenceAPIAuthenticationRBACAndBounds(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	service := &fakeTimingIntelligenceService{}
	handler := New(WithOperatorTimingIntelligence(h.auth, service)).Handler()
	const projectID = "00000000-0000-7000-8000-000000000401"
	const sessionID = "00000000-0000-7000-8000-000000000402"

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/timing-intelligence", nil)
	unauthenticated.RemoteAddr = "127.0.0.1:19001"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d, want 401", unauthenticatedRes.Code)
	}
	if service.reportCalls != 0 {
		t.Fatalf("unauthenticated request reached timing service: calls=%d", service.reportCalls)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/api/v1/projects/" + projectID + "/timing-intelligence?lead_time_seconds=-1",
		"/api/v1/projects/" + projectID + "/timing-intelligence?lead_time_seconds=3601",
		"/api/v1/projects/" + projectID + "/timing-intelligence?lead_time_seconds=abc",
		"/api/v1/projects/" + projectID + "/timing-intelligence?section_from_cue_id=cue-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "127.0.0.1:19002"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("bounded request %q status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
	if service.reportCalls != 0 {
		t.Fatalf("invalid query reached timing service: calls=%d", service.reportCalls)
	}

	validPath := "/api/v1/projects/" + projectID + "/timing-intelligence?session_id=" + sessionID + "&lead_time_seconds=45&section_from_cue_id=cue-1&section_to_cue_id=cue-3"
	validReq := httptest.NewRequest(http.MethodGet, validPath, nil)
	validReq.RemoteAddr = "127.0.0.1:19003"
	validReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	validRes := httptest.NewRecorder()
	handler.ServeHTTP(validRes, validReq)
	if validRes.Code != http.StatusOK {
		t.Fatalf("valid report status=%d body=%s", validRes.Code, validRes.Body.String())
	}
	if service.reportCalls != 1 || service.lastProjectID != projectID {
		t.Fatalf("report calls=%d project=%q", service.reportCalls, service.lastProjectID)
	}
	if service.lastOptions.SessionID != sessionID || service.lastOptions.LeadTime != 45*time.Second || service.lastOptions.SectionFromCueID != "cue-1" || service.lastOptions.SectionToCueID != "cue-3" {
		t.Fatalf("unexpected report options: %+v", service.lastOptions)
	}
	if !strings.Contains(validRes.Body.String(), `"advisory_only":true`) {
		t.Fatalf("report did not preserve advisory-only contract: %s", validRes.Body.String())
	}

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000403', 'timing-viewer', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "timing-viewer", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	viewerRead := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/timing-intelligence", nil)
	viewerRead.RemoteAddr = "127.0.0.1:19004"
	viewerRead.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerReadRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerReadRes, viewerRead)
	if viewerReadRes.Code != http.StatusOK {
		t.Fatalf("VIEWER report status=%d body=%s", viewerReadRes.Code, viewerReadRes.Body.String())
	}

	viewerWrite := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/timing-intelligence/sessions/"+sessionID, strings.NewReader(`{"mode":"EXCLUDE"}`))
	viewerWrite.RemoteAddr = "127.0.0.1:19005"
	viewerWrite.Header.Set("Content-Type", "application/json")
	viewerWrite.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerWrite.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerWriteRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerWriteRes, viewerWrite)
	if viewerWriteRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER selection status=%d, want 403", viewerWriteRes.Code)
	}
	if service.selectionCalls != 0 {
		t.Fatalf("forbidden VIEWER mutation reached timing service: calls=%d", service.selectionCalls)
	}

	missingCSRF := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/timing-intelligence/sessions/"+sessionID, strings.NewReader(`{"mode":"EXCLUDE"}`))
	missingCSRF.RemoteAddr = "127.0.0.1:19006"
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF selection status=%d, want 403", missingCSRFRes.Code)
	}
	if service.selectionCalls != 0 {
		t.Fatalf("CSRF-denied mutation reached timing service: calls=%d", service.selectionCalls)
	}

	ownerWrite := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/timing-intelligence/sessions/"+sessionID, strings.NewReader(`{"mode":"EXCLUDE"}`))
	ownerWrite.RemoteAddr = "127.0.0.1:19007"
	ownerWrite.Header.Set("Content-Type", "application/json")
	ownerWrite.Header.Set(csrfHeader, owner.CSRFToken)
	ownerWrite.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerWriteRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerWriteRes, ownerWrite)
	if ownerWriteRes.Code != http.StatusOK {
		t.Fatalf("OWNER selection status=%d body=%s", ownerWriteRes.Code, ownerWriteRes.Body.String())
	}
	if service.selectionCalls != 1 || service.lastSessionID != sessionID || service.lastSelectionMode != store.TimingSelectionExclude || service.lastUpdatedBy != "owner" {
		t.Fatalf("unexpected selection call: calls=%d session=%q mode=%q updated_by=%q", service.selectionCalls, service.lastSessionID, service.lastSelectionMode, service.lastUpdatedBy)
	}

	goReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/timing-intelligence/go", nil)
	goReq.RemoteAddr = "127.0.0.1:19008"
	goReq.Header.Set(csrfHeader, owner.CSRFToken)
	goReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	goRes := httptest.NewRecorder()
	handler.ServeHTTP(goRes, goReq)
	if goRes.Code != http.StatusNotFound {
		t.Fatalf("unexpected F-028 command surface status=%d, want 404", goRes.Code)
	}
	if service.selectionCalls != 1 {
		t.Fatalf("nonexistent GO route reached timing service: selection calls=%d", service.selectionCalls)
	}
}
