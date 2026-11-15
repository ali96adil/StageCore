package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestPrivateCueEditorReadsSuppressStoreErrorsAfterProjectGrantRevocation(t *testing.T) {
	cases := []struct {
		name, path, table, hookName string
		needsCue                    bool
	}{
		{"recovery", "cue-editor-recovery", "cue_editor_recovery", "cue-editor-recovery-read", false},
		{"checkpoint-list", "cues/{cue_id}/editor-checkpoints", "cue_editor_checkpoints", "cue-editor-checkpoint-list", true},
		{"action-presets", "cue-action-presets", "cue_action_presets", "cue-action-presets-list", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			st := store.New(h.db.DB, clock.Real{})
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			tech, err := h.auth.CreateUser(ctx, "private-cue-read-"+tc.name, h.password, userauth.RoleTechnician)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			access, err := projectaccess.New(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Private Cue reads", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			cueID := ""
			if tc.needsCue {
				cue, err := st.CreateCueWithActions(ctx, domain.Cue{
					RevisionID: revision.ID, DisplayLabel: "1", Name: "Private checkpoint", OrderIndex: 0,
					Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
				}, nil)
				if err != nil {
					t.Fatal(err)
				}
				cueID = cue.ID
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
				[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.CueEdit}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE "+tc.table); err != nil {
				t.Fatal(err)
			}
			revoked := false
			revokeAtError := func(route string) {
				if route != tc.hookName {
					t.Errorf("error hook route=%q want=%q", route, tc.hookName)
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, nil); err != nil {
					t.Fatal(err)
				}
			}
			handler := New(
				withOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry()), cuePublishHooks{
					BeforeErrorResponse: revokeAtError,
				}),
				WithProjectRouteQualification(access),
			).Handler()
			path := "/api/v1/projects/" + project.ID + "/" + strings.ReplaceAll(tc.path, "{cue_id}", cueID)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
			if !revoked {
				t.Fatalf("storage error hook did not revoke the Project grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
				t.Fatalf("revoked private editor read returned status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "no such table") {
				t.Fatalf("storage error escaped after Project grant revocation: %s", response.Body.String())
			}
		})
	}
}
