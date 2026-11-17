package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestQualifiedOwnerAdapterCheckRejectsRevokedSession(t *testing.T) {
	h := newAuthHarness(t)
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const pattern = "POST /api/v1/stage-devices/{device_id}/commands"
	handler := New(WithProjectRouteQualification(access), func(s *Server) {
		s.mux.HandleFunc(pattern, withPermission(h.auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			if err := capability.CheckDispatch(r.Context()); err != nil {
				t.Fatalf("Owner dispatch rejected before revocation: %v", err)
			}
			if err := h.auth.Logout(r.Context(), login.Token); err != nil {
				t.Fatal(err)
			}
			if err := capability.CheckDispatch(r.Context()); !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
				t.Fatalf("revoked Owner retained adapter authority: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}).Handler()
	req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, "/api/v1/stage-devices/example/commands", []byte("{}"))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("test did not reach dispatch boundary: %d %s", res.Code, res.Body.String())
	}
}

type revokeDuringRead struct {
	*strings.Reader
	revoke func()
}

func (r revokeDuringRead) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.revoke()
	return n, err
}

func TestBrowserMediaDiscardsChunkWhenRevokedDuringRead(t *testing.T) {
	allowed := true
	stream := &authorizedMediaReader{
		ReadSeeker: revokeDuringRead{strings.NewReader("private payload"), func() { allowed = false }},
		authorize:  func() bool { return allowed },
	}
	buf := make([]byte, 64)
	n, err := stream.Read(buf)
	if n != 0 || !errors.Is(err, domain.ErrNotFound) || !stream.denied {
		t.Fatalf("read returned %d bytes, err=%v, denied=%v", n, err, stream.denied)
	}
}
