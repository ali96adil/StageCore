package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/timecode"
)

type timecodeReadFunc func(context.Context, string, string) (timecode.RuntimeSummary, error)

func (f timecodeReadFunc) LiveSummary(ctx context.Context, project, snapshot string) (timecode.RuntimeSummary, error) {
	return f(ctx, project, snapshot)
}

func TestTimecodeWithholdsSummaryAndErrorsAfterGrantRevocation(t *testing.T) {
	for _, outcome := range []struct {
		name string
		err  error
	}{
		{"summary", nil},
		{"diagnostic error", context.DeadlineExceeded},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			f := setupProjectAccessAPI(t)
			ctx := context.Background()
			set := func(permissions []projectaccess.Permission) {
				t.Helper()
				if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.viewerID, f.projectID, permissions); err != nil {
					t.Fatal(err)
				}
			}
			set([]projectaccess.Permission{projectaccess.ProjectView})
			called := false
			reader := timecodeReadFunc(func(_ context.Context, project, _ string) (timecode.RuntimeSummary, error) {
				called = project == f.projectID
				set(nil)
				return timecode.RuntimeSummary{}, outcome.err
			})
			handler := New(WithProjectRouteQualification(f.access), WithOperatorTimecode(f.h.auth, reader)).Handler()
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+f.projectID+"/timecode", f.viewer.Token))
			if !called || res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), "supported_rates") {
				t.Fatalf("read=%t status=%d body=%s", called, res.Code, res.Body)
			}
		})
	}
}
