package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestAssistantPreviewDoesNotReturnSealedProposalAfterProjectGrantRevocation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	access, err := projectaccess.New(h.auth.db.DB)
	if err != nil { t.Fatal(err) }
	operator, err := h.auth.auth.CreateUser(ctx, "assistant-revoked-preview", h.auth.password, userauth.RoleOperator)
	if err != nil { t.Fatal(err) }
	login, err := h.auth.auth.Login(ctx, operator.Username, h.auth.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	if err := access.SetPermissions(ctx, h.owner.Session.User.ID, operator.ID, h.project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil { t.Fatal(err) }
	handler := New(WithProjectRouteQualification(access), WithOperatorConfigurationDraft(h.auth.auth, h.stageStore, h.audit)).Handler()
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalNoteDraft, "Preview private note", `{"category":"assistant","body":"Secret note"}`)
	payload, err := json.Marshal(assistantProposalRequest{Proposal: proposal})
	if err != nil { t.Fatal(err) }
	path := "/api/v1/projects/" + h.project.ID + "/assistant/proposals/preview"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.RemoteAddr = "127.0.0.1:19031"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	req.Header.Set(csrfHeader, login.CSRFToken)
	req.Header.Set("Content-Type", "application/json")
	revoked := false
	req.Body = io.NopCloser(revokeDuringRead{Reader: strings.NewReader(string(payload)), revoke: func() {
		if revoked { return }
		revoked = true
		if err := access.SetPermissions(ctx, h.owner.Session.User.ID, operator.ID, h.project.ID, nil); err != nil { t.Fatal(err) }
	}})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !revoked || res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), "apply_required") ||
		strings.Contains(res.Body.String(), "Secret note") {
		t.Fatalf("revoked preview: revoked=%v status=%d body=%s", revoked, res.Code, res.Body.String())
	}
}
