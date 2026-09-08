package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedShowCapsuleAssetsAreServedOnDeclaredRoutes(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:18131"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d", rootRes.Code)
	}
	for _, script := range []string{"/show-capsules.js", "/show-capsules-nav.js"} {
		if !strings.Contains(rootRes.Body.String(), `src="`+script+`"`) {
			t.Fatalf("operator root missing script %s", script)
		}
	}

	cases := []struct {
		path string
		want string
	}{
		{path: "/show-capsules.js", want: "f019Copy"},
		{path: "/show-capsules-nav.js", want: "capsulesNav"},
	}
	for i, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:1813" + string(rune('2'+i))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d", tc.path, res.Code)
		}
		if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/javascript") {
			t.Fatalf("%s content-type=%q", tc.path, got)
		}
		if !strings.Contains(res.Body.String(), tc.want) {
			t.Fatalf("%s missing %q", tc.path, tc.want)
		}
	}

	timingReq := httptest.NewRequest(http.MethodGet, "/timing-intelligence.js", nil)
	timingReq.RemoteAddr = "127.0.0.1:18134"
	timingRes := httptest.NewRecorder()
	handler.ServeHTTP(timingRes, timingReq)
	if timingRes.Code != http.StatusOK {
		t.Fatalf("timing-intelligence.js status=%d", timingRes.Code)
	}
	if strings.Contains(timingRes.Body.String(), "capsulesNav") {
		t.Fatal("timing-intelligence.js must not duplicate Show Capsule navigation client")
	}
}
