package projectaccess_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// BenchmarkProjectRuntimeAuthorization measures the live browser-session and
// Project ACL checks used at runtime dispatch boundaries. It never invokes a
// device or external adapter.
func BenchmarkProjectRuntimeAuthorization(b *testing.B) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = handle.Close() })
	security, err := hubsecurity.Open(ctx, handle.DB, b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	setupCode, err := security.GenerateSetupCode(ctx)
	if err != nil {
		b.Fatal(err)
	}
	ownerID, err := security.ClaimFirstOwner(ctx, setupCode.Code, "Benchmark Owner", "benchmark-password")
	if err != nil {
		b.Fatal(err)
	}
	auth, err := userauth.New(handle.DB)
	if err != nil {
		b.Fatal(err)
	}
	operator, err := auth.CreateUser(ctx, "benchmark-operator", "benchmark-password", userauth.RoleOperator)
	if err != nil {
		b.Fatal(err)
	}
	credential, err := auth.Login(ctx, operator.Username, "benchmark-password", "127.0.0.1")
	if err != nil {
		b.Fatal(err)
	}
	access, err := projectaccess.New(handle.DB)
	if err != nil {
		b.Fatal(err)
	}
	projects := store.New(handle.DB, clock.Real{})
	type showScope struct {
		projectID string
		sessionID string
	}
	showScopes := make(map[string]showScope)
	for _, name := range []string{"Benchmark SHOW", "Benchmark REHEARSAL"} {
		project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: name, CreatedBy: ownerID})
		if err != nil {
			b.Fatal(err)
		}
		if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
			b.Fatal(err)
		}
		published, _, err := snapshot.NewBuilder(projects).Create(ctx, revision.ID, ownerID)
		if err != nil {
			b.Fatal(err)
		}
		sessionType := domain.SessionShow
		if name == "Benchmark REHEARSAL" {
			sessionType = domain.SessionRehearsal
		}
		active, err := projects.StartOperationalSession(ctx, published.ID, sessionType, name, "")
		if err != nil {
			b.Fatal(err)
		}
		if err := access.SetPermissions(ctx, ownerID, operator.ID, project.ID, []projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.ShowGO, projectaccess.ShowStop,
			projectaccess.ShowBlackoutEngage, projectaccess.RehearsalControl,
		}); err != nil {
			b.Fatal(err)
		}
		showScopes[name] = showScope{projectID: project.ID, sessionID: active.ID}
	}
	benchIntent := func(name string, scope showScope, intent projectaccess.RuntimeIntent) {
		b.Run(name, func(b *testing.B) {
			latencies := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				started := time.Now()
				current, err := auth.ValidateCSRF(ctx, credential.Token, credential.CSRFToken)
				if err == nil {
					err = access.CheckRuntimeIntent(ctx, current.User.ID, scope.projectID, scope.sessionID, intent)
				}
				latencies[i] = time.Since(started).Nanoseconds()
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100]), "p95-ns")
			b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100]), "p99-ns")
		})
	}
	show := showScopes["Benchmark SHOW"]
	rehearsal := showScopes["Benchmark REHEARSAL"]
	benchIntent("SHOW_GO", show, projectaccess.RuntimeGO)
	benchIntent("SHOW_JUMP", show, projectaccess.RuntimeJump)
	benchIntent("SHOW_STOP", show, projectaccess.RuntimeStopCue)
	benchIntent("REHEARSAL_GO", rehearsal, projectaccess.RuntimeGO)
	benchIntent("EMERGENCY_BLACKOUT_ENGAGE", show, projectaccess.RuntimeBlackoutEngage)
	b.Run("COMPOUND_CUE_4_ACTION_CHECKS", func(b *testing.B) {
		latencies := make([]int64, b.N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			started := time.Now()
			var err error
			for action := 0; action < 4; action++ {
				current, validateErr := auth.ValidateCSRF(ctx, credential.Token, credential.CSRFToken)
				if validateErr != nil {
					err = validateErr
					break
				}
				err = access.CheckRuntimeIntent(ctx, current.User.ID, rehearsal.projectID, rehearsal.sessionID, projectaccess.RuntimeGO)
				if err != nil {
					break
				}
			}
			latencies[i] = time.Since(started).Nanoseconds()
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100]), "p95-ns")
		b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100]), "p99-ns")
	})
}
