package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestProjectNoteMutationsSuppressCommittedResultsAfterGrantRevocation(t *testing.T) {
	cases := []struct {
		name, method, suffix, body, expectedBody string
		status                                   domain.NoteStatus
	}{
		{"create", http.MethodPost, "", `{"body":"new note","category":"safety"}`, "new note", domain.NoteOpen},
		{"update", http.MethodPatch, "NOTE_ID", `{"body":"updated note","category":"timing"}`, "updated note", domain.NoteOpen},
		{"resolve", http.MethodPost, "NOTE_ID/resolve", "", "original note", domain.NoteResolved},
		{"reopen", http.MethodPost, "NOTE_ID/reopen", "", "original note", domain.NoteOpen},
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
			tech, err := h.auth.CreateUser(ctx, "note-mutation-"+tc.name, h.password, userauth.RoleTechnician)
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
			project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Note mutation response fence", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
				[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
				t.Fatal(err)
			}
			noteID := ""
			if tc.name != "create" {
				note, err := st.CreateNote(ctx, project.ID, store.CreateNoteParams{Body: "original note", CreatedBy: owner.Session.User.Username})
				if err != nil {
					t.Fatal(err)
				}
				noteID = note.ID
			}
			suffix := strings.ReplaceAll(tc.suffix, "NOTE_ID", noteID)
			path := "/api/v1/projects/" + project.ID + "/notes"
			if suffix != "" {
				path += "/" + suffix
			}
			revoked := false
			handler := New(
				withOperatorMemoryHooks(h.auth, st, sessionmemory.New(st), operatorMemoryHooks{BeforeResponse: func() {
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, nil); err != nil {
						t.Fatal(err)
					}
				}}),
				WithProjectRouteQualification(access),
			).Handler()
			request := authenticatedMutationRequest(t, tc.method, path, []byte(tc.body), login.Token, login.CSRFToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("post-commit hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
				t.Fatalf("revoked note mutation returned status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), `"note"`) {
				t.Fatalf("revoked note mutation exposed committed note details: %s", response.Body.String())
			}
			var note domain.Note
			if tc.name == "create" {
				rows, err := st.ListNotes(ctx, project.ID, store.NoteFilter{})
				if err != nil || len(rows) != 1 {
					t.Fatalf("committed created note missing: rows=%+v err=%v", rows, err)
				}
				note = rows[0]
			} else {
				note, err = st.GetNote(ctx, project.ID, noteID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if note.Body != tc.expectedBody || note.Status != tc.status {
				t.Fatalf("committed note state=%+v, want body=%q status=%q", note, tc.expectedBody, tc.status)
			}
		})
	}
}
