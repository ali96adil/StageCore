package httpapi

import (
    "fmt"
    "os"
    "strings"
    "testing"
)

// This map is a source-reviewed record of *dynamic HandleFunc registrations*
// from the canonical SC-024 stack. None of these entries is evidence of
// enforced per-Project membership: all still go through global userauth
// roles until a separate, atomic ACL cutover.
//
// The static literal registrations are independently covered by the SHA
// baseline in route_acl_registration_guard_test.go.
// Do not add wildcard allowances here: newly constructed routes must be
// individually classified and tested for cross-Project direct-ID probes.
func TestSC024AllDynamicRegistrationExpressionsAreReviewed(t *testing.T) {
    expected := map[string]string{
        // Project + revision + environment + asset. Media/version/snapshot
        // identity must be resolved from the DB, not a client path.
        `operator_execution_environment_capture.go|DYNAMIC("GET " + base + "/vault-status")`: "PROJECT_MEDIA_READ",
        `operator_execution_environment_capture.go|DYNAMIC("POST " + base + "/capture")`: "PROJECT_MEDIA_CAPTURE",
        // Runtime operation may change external software environment state.
        `operator_execution_environment_operations.go|DYNAMIC("POST " + path)`: "PROJECT_EXECUTION_OPERATION",
        // Seed, modify and remove must validate revision/project ownership.
        `operator_execution_environment_rebuild_plan.go|DYNAMIC("GET " + path)`: "PROJECT_REBUILD_PLAN_READ",
        `operator_execution_environment_rebuild_plan.go|DYNAMIC("POST " + path + "/seed")`: "PROJECT_REBUILD_PLAN_EDIT",
        `operator_execution_environment_rebuild_plan.go|DYNAMIC("PUT " + path)`: "PROJECT_REBUILD_PLAN_EDIT",
        `operator_execution_environment_rebuild_plan.go|DYNAMIC("DELETE " + path)`: "PROJECT_REBUILD_PLAN_DELETE",
        // Snapshot IDs and diff sources must match Project + revision.
        `operator_execution_environment_snapshot_diff.go|DYNAMIC("GET " + collection)`: "PROJECT_ENV_SNAPSHOT_LIST",
        `operator_execution_environment_snapshot_diff.go|DYNAMIC("GET " + detail)`: "PROJECT_ENV_SNAPSHOT_DETAIL",
        `operator_execution_environment_snapshot_diff.go|DYNAMIC("GET " + path)`: "PROJECT_ENV_SNAPSHOT_DIFF",
        `operator_execution_environments.go|DYNAMIC("GET " + collection)`: "PROJECT_ENV_LIST",
        `operator_execution_environments.go|DYNAMIC("POST " + collection)`: "PROJECT_ENV_CREATE",
        `operator_execution_environments.go|DYNAMIC("PUT " + item)`: "PROJECT_ENV_EDIT",
        `operator_execution_environments.go|DYNAMIC("PUT " + item + "/machine-role")`: "PROJECT_ENV_ASSIGN_ROLE",
        `operator_execution_environments.go|DYNAMIC("DELETE " + item)`: "PROJECT_ENV_DELETE",
        // Owner's global user.manage plus live DB permission admin;
        // separate from the Operator's project.view rights.
        `operator_project_access.go|DYNAMIC("GET " + pattern)`: "OWNER_GRANTS_READ",
        `operator_project_access.go|DYNAMIC("PUT " + pattern)`: "OWNER_GRANTS_REPLACE",
    }
    records := collectHTTPRouteRegistrations(t)
    found := make(map[string]bool)
    for _, registration := range records {
        if !strings.HasPrefix(registration.Pattern, "DYNAMIC(") {
            continue
        }
        key := fmt.Sprintf("%s|%s", registration.Source, registration.Pattern)
        if _, exists := expected[key]; !exists {
            t.Errorf("unreviewed dynamic HTTP registration: %s", key)
            continue
        }
        found[key] = true
    }
    for key := range expected {
        if !found[key] {
            t.Errorf("reviewed dynamic HTTP registration changed/missing: %s", key)
        }
    }
    if len(found) != len(expected) {
        t.Fatalf("dynamic route audit incomplete: reviewed %d of %d", len(found), len(expected))
    }
}

// Additional source-owner guard: a registration does not imply Project ACL.
// These legacy dynamic families must explicitly stay classified as using
// a global browser role until the production server-wide cutover exists.
func TestSC024DynamicProjectRouteFamiliesHaveExplicitLegacyAuthBoundary(t *testing.T) {
    families := map[string][]string{
        "operator_execution_environment_capture.go":       {"withPermission(auth, userauth.PermissionProjectRead", "withPermission(auth, userauth.PermissionProjectEdit"},
        "operator_execution_environment_operations.go":    {"withPermission(auth, userauth.PermissionRuntimeControl"},
        "operator_execution_environment_rebuild_plan.go":  {"withPermission(auth, userauth.PermissionProjectRead", "withPermission(auth, userauth.PermissionProjectEdit"},
        "operator_execution_environment_snapshot_diff.go": {"withPermission(auth, userauth.PermissionProjectRead"},
        "operator_execution_environments.go":              {"withPermission(auth, userauth.PermissionProjectRead", "withPermission(auth, userauth.PermissionProjectEdit"},
        "operator_project_access.go":                       {"withPermission(auth, userauth.PermissionUserManage"},
    }
    for name, guardMarkers := range families {
        t.Run(name, func(t *testing.T) {
            raw, err := os.ReadFile(name)
            contents := string(raw)
            if err != nil { t.Fatal(err) }
            for _, guard := range guardMarkers {
                if !strings.Contains(contents, guard) {
                    t.Errorf("legacy credential boundary changed in %s; review atomic Project ACL matrix: %s", name, guard)
                }
            }
        })
    }
}
