package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/haauthority"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type fakeHAAuthorityControl struct {
	status        haauthority.SupervisorStatus
	statusErr     error
	activateErr   error
	releaseErr    error
	statusCalls   int
	activateCalls int
	releaseCalls  int
	demoteCalls   int
}

func (f *fakeHAAuthorityControl) Status(context.Context) (haauthority.SupervisorStatus, error) {
	f.statusCalls++
	return f.status, f.statusErr
}

func (f *fakeHAAuthorityControl) Activate(context.Context) error {
	f.activateCalls++
	if f.activateErr != nil {
		return f.activateErr
	}
	f.status = haauthority.SupervisorStatus{
		Mode: dispatchauthority.ModeLeader, HolderID: "hub-a", Epoch: 7,
		RemainingMS: 4000, RenewalActive: true,
	}
	return nil
}

func (f *fakeHAAuthorityControl) Release(context.Context) error {
	f.releaseCalls++
	if f.releaseErr != nil {
		return f.releaseErr
	}
	f.status = haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby}
	return nil
}

func (f *fakeHAAuthorityControl) Demote() {
	f.demoteCalls++
	f.status = haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby}
}

func TestOperatorHAAuthorityOwnerControlsAndAudit(t *testing.T) {
	h := newAuthHarness(t)
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := &fakeHAAuthorityControl{status: haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby}}
	handler := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorHAAuthority(h.auth, authority, audit),
	).Handler()
	credential, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	statusRes := serveHARequest(handler, credential, http.MethodGet, "/api/v1/ha/authority", false)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	if authority.statusCalls != 1 {
		t.Fatalf("status calls=%d, want 1", authority.statusCalls)
	}

	activateRes := serveHARequest(handler, credential, http.MethodPost, "/api/v1/ha/authority/activate", true)
	if activateRes.Code != http.StatusOK {
		t.Fatalf("activate code=%d body=%s", activateRes.Code, activateRes.Body.String())
	}
	if authority.activateCalls != 1 || authority.status.Mode != dispatchauthority.ModeLeader {
		t.Fatalf("activate calls=%d status=%+v", authority.activateCalls, authority.status)
	}

	demoteRes := serveHARequest(handler, credential, http.MethodPost, "/api/v1/ha/authority/demote", true)
	if demoteRes.Code != http.StatusOK {
		t.Fatalf("demote code=%d body=%s", demoteRes.Code, demoteRes.Body.String())
	}
	if authority.demoteCalls != 1 || authority.status.Mode != dispatchauthority.ModeStandby {
		t.Fatalf("demote calls=%d status=%+v", authority.demoteCalls, authority.status)
	}

	if err := authority.Activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	authority.activateCalls-- // direct setup is not an HTTP action under test.
	releaseRes := serveHARequest(handler, credential, http.MethodPost, "/api/v1/ha/authority/release", true)
	if releaseRes.Code != http.StatusOK {
		t.Fatalf("release code=%d body=%s", releaseRes.Code, releaseRes.Body.String())
	}
	if authority.releaseCalls != 1 || authority.status.Mode != dispatchauthority.ModeStandby {
		t.Fatalf("release calls=%d status=%+v", authority.releaseCalls, authority.status)
	}

	records, err := audit.List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, record := range records {
		seen[record.EventType] = record.Result
	}
	for _, eventType := range []string{"ha.authority.activate", "ha.authority.demote", "ha.authority.release"} {
		if seen[eventType] != securityaudit.ResultSuccess {
			t.Fatalf("audit %s result=%q, want SUCCESS; records=%+v", eventType, seen[eventType], records)
		}
	}
}

func TestOperatorHAAuthorityActivationConflictIsAudited(t *testing.T) {
	h := newAuthHarness(t)
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := &fakeHAAuthorityControl{
		status:      haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby},
		activateErr: haauthority.ErrOperationalSessionActive,
	}
	handler := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorHAAuthority(h.auth, authority, audit),
	).Handler()
	credential, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	res := serveHARequest(handler, credential, http.MethodPost, "/api/v1/ha/authority/activate", true)
	if res.Code != http.StatusConflict {
		t.Fatalf("activate conflict code=%d body=%s", res.Code, res.Body.String())
	}
	if authority.activateCalls != 1 {
		t.Fatalf("activate calls=%d, want 1", authority.activateCalls)
	}
	records, err := audit.List(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[0].EventType != "ha.authority.activate" || records[0].Result != securityaudit.ResultRejected || records[0].Reason != "HA_AUTHORITY_ACTIVATION_BLOCKED_ACTIVE_SESSION" {
		t.Fatalf("unexpected rejected activation audit: %+v", records)
	}
}

func TestOperatorHAAuthorityPermissionAndStandaloneRegistration(t *testing.T) {
	h := newAuthHarness(t)
	var passwordHash string
	if err := h.db.DB.QueryRowContext(context.Background(), `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(context.Background(), `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000099', 'viewer-ha', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(context.Background(), "viewer-ha", h.password, "127.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	authority := &fakeHAAuthorityControl{status: haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby}}
	handler := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorHAAuthority(h.auth, authority, nil),
	).Handler()

	viewerRes := serveHARequest(handler, viewer, http.MethodGet, "/api/v1/ha/authority", false)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER status code=%d, want 403", viewerRes.Code)
	}
	if authority.statusCalls != 0 {
		t.Fatalf("VIEWER reached HA authority handler; status calls=%d", authority.statusCalls)
	}

	standalone := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorHAAuthority(h.auth, nil, nil),
	).Handler()
	owner, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	standaloneRes := serveHARequest(standalone, owner, http.MethodGet, "/api/v1/ha/authority", false)
	if standaloneRes.Code != http.StatusNotFound {
		t.Fatalf("STANDALONE HA route code=%d, want 404", standaloneRes.Code)
	}
}

func TestOperatorHAAuthorityMutationRequiresCSRF(t *testing.T) {
	h := newAuthHarness(t)
	authority := &fakeHAAuthorityControl{status: haauthority.SupervisorStatus{Mode: dispatchauthority.ModeStandby}}
	handler := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorHAAuthority(h.auth, authority, nil),
	).Handler()
	credential, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	res := serveHARequest(handler, credential, http.MethodPost, "/api/v1/ha/authority/activate", false)
	if res.Code != http.StatusForbidden {
		t.Fatalf("activate without CSRF code=%d, want 403", res.Code)
	}
	if authority.activateCalls != 0 {
		t.Fatalf("activate without CSRF reached authority; calls=%d", authority.activateCalls)
	}
}

func serveHARequest(handler http.Handler, credential userauth.Credential, method, path string, includeCSRF bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:43210"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	if includeCSRF {
		req.Header.Set(csrfHeader, credential.CSRFToken)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

var _ haAuthorityControl = (*fakeHAAuthorityControl)(nil)
