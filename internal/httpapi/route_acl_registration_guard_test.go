package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The baseline is intentionally reviewed from the CI discovery output. A
// source-level route registration change MUST fail CI until the Project ACL
// and service/device trust-boundary map is re-reviewed. This gate is an
// inventory integrity guard, NOT Project ACL enforcement or a substitute for
// the HTTP integration tests.
//
// REVIEWED SNAPSHOT: edit this digest only together with an explicit
// endpoint security review and its documentation. No production cutover.
const reviewedHTTPRouteInventorySHA256 = "ffba2145d358503cc15ece65eb63f0571b5c564e622067673c5fc6cc199a0a7b"

// routeRegistration describes a registration expression, not an assumption
// about whether the handler actually enforces the desired Project scope.
type routeRegistration struct {
	Source               string
	Pattern              string
	BrowserAuthenticated bool
}

func collectHTTPRouteRegistrations(t *testing.T) []routeRegistration {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	records := make([]routeRegistration, 0, 200)
	seen := make(map[string]struct{})
	for _, entry := range entries {
		filename := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") ||
			strings.HasSuffix(filename, "_test.go") {
			continue
		}
		set := token.NewFileSet()
		parsed, err := parser.ParseFile(set, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Handle" &&
				selector.Sel.Name != "HandleFunc") {
				return true
			}
			pattern := ""
			if literal, ok := call.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("unquote handler pattern %s: %v", filename, err)
				}
				pattern = value
			} else {
				// Dynamic/indirect registration patterns are *not* ignored.
				// Their AST representation is inventoried, but must be
				// separately resolved by the human route/security reviewer.
				var output bytes.Buffer
				if err := format.Node(&output, set, call.Args[0]); err != nil {
					t.Fatalf("format dynamic registration %s: %v", filename, err)
				}
				pattern = "DYNAMIC(" + output.String() + ")"
			}
			record := routeRegistration{Source: filename, Pattern: pattern}
			if len(call.Args) > 1 {
				if wrapper, ok := call.Args[1].(*ast.CallExpr); ok {
					if name, ok := wrapper.Fun.(*ast.Ident); ok {
						record.BrowserAuthenticated = name.Name == "withPermission" || name.Name == "withAuthenticatedSession" ||
							// The local StageAudio GET wrapper is exercised through the real router.
							(filename == "operator_stageaudio.go" && name.Name == "get" && strings.HasPrefix(pattern, "GET /api/v1/system/audio/"))
					} else if selector, ok := wrapper.Fun.(*ast.SelectorExpr); ok && filename == "security_operations.go" && selector.Sel.Name == "route" {
						record.BrowserAuthenticated = true
					}
				}
			}
			key := filename + "|" + pattern
			// Distinct occurrences of the same pattern in one source also
			// matter. Duplicates can indicate conditional registration.
			if _, exists := seen[key]; !exists {
				records = append(records, record)
				seen[key] = struct{}{}
			} else {
				// Record duplicate registrations with deterministic sequence
				// information instead of allowing a silent coverage bypass.
				number := 2
				for {
					dup := fmt.Sprintf("%s#%d", pattern, number)
					if _, exists := seen[filename+"|"+dup]; !exists {
						record.Pattern = dup
						records = append(records, record)
						seen[filename+"|"+dup] = struct{}{}
						break
					}
					number++
				}
			}
			return true
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Source != records[j].Source {
			return records[i].Source < records[j].Source
		}
		return records[i].Pattern < records[j].Pattern
	})
	return records
}

func TestSC024RegisteredHTTPRoutesMustRemainReviewed(t *testing.T) {
	records := collectHTTPRouteRegistrations(t)
	if len(records) < 100 {
		t.Fatalf("HTTP route audit failed closed: only %d registrations found", len(records))
	}
	lines := make([]string, 0, len(records))
	dynamicCount := 0
	for _, record := range records {
		if strings.HasPrefix(record.Pattern, "DYNAMIC(") {
			dynamicCount++
		}
		lines = append(lines, record.Source+"|"+record.Pattern)
	}
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	actual := hex.EncodeToString(digest[:])
	if actual != reviewedHTTPRouteInventorySHA256 {
		t.Logf("ROUTE_REVIEW_DIGEST=%s COUNT=%d DYNAMIC=%d", actual, len(lines), dynamicCount)
		for _, line := range lines {
			t.Logf("ROUTE_REVIEW_REGISTRATION=%s", line)
		}
		t.Fatalf("SC-024 route inventory changed or is not reviewed: update security inventory, source-route classifications, endpoint bypass tests, and the reviewed SHA together")
	}
}

// A feature-specific scoped endpoint with green tests must not be mistaken
// for a complete ACL rollout. This source gate intentionally checks the
// production assembler, rather than only a test-local httpapi.New(...).
func TestSC024NoPartialProjectACLFeatureActivationInProduction(t *testing.T) {
	guardedOptions := []string{
		"WithOperatorProjectAccess",
		"WithScopedOperatorProjects",
		"WithProjectRouteQualification",
		"WithScopedOperatorShowCapsules",
		"WithScopedProjectMediaDownload",
		"WithScopedOperatorStageDeviceReads",
		"WithOwnerPluginAdministration",
		"WithScopedProjectFilesRead",
		"WithScopedLocalCapsuleExports",
		"WithOwnerCapsuleImportReview",
		"WithOwnerProjectCutoverPreview",
		"WithScopedOutputWatchRead",
		"WithScopedCompanionRoleWatch",
	}
	for _, pattern := range []string{
		filepath.Join("..", "..", "cmd", "stagecore-hub", "*.go"),
		filepath.Join("..", "app", "*.go"),
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) == 0 {
			t.Fatalf("missing production source glob %s", pattern)
		}
		for _, filename := range paths {
			if strings.HasSuffix(filename, "_test.go") {
				continue
			}
			source, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			for _, option := range guardedOptions {
				if bytes.Contains(source, []byte("httpapi."+option+"(")) ||
					bytes.Contains(source, []byte("httpapi.\n"+option+"(")) {
					t.Fatalf("partial scoped Project ACL option %q wired in production %s before complete cutover", option, filename)
				}
			}
		}
	}
}

func TestSC024QualifiedBrowserRoutesUseLiveAuthentication(t *testing.T) {
	for _, record := range collectHTTPRouteRegistrations(t) {
		qualified := projectURLPattern(record.Pattern) || projectRouteIsQualified(record.Pattern)
		// Indirect patterns are separately reconciled with the project
		// inventory. All reviewed indirect HTTP routes here are browser APIs.
		if (qualified || strings.HasPrefix(record.Pattern, "DYNAMIC(")) && !record.BrowserAuthenticated {
			t.Errorf("qualification bypass: %s %s lacks the browser middleware", record.Source, record.Pattern)
		}
	}
}

func TestSecurityOperationsRouteWrapperRechecksBeforeDelivery(t *testing.T) {
	source, err := os.ReadFile("security_operations.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"return withPermission(s.users, permission",
		"securityOperationResponseAuthorized(s.users, r, session, permission)",
		"writeSecurityOperationAuthorityEnded(w, isUnsafeMethod(r.Method))",
		"buffered.flush(w)",
	} {
		if !bytes.Contains(source, []byte(required)) {
			t.Errorf("security operations response fence lost %q", required)
		}
	}
}
