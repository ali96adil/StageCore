package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOperatorClockStatusAuthenticatedReadOnlyAndNoSideEffects(t *testing.T) {
	h := newAuthHarness(t)
	const endpoint = "/api/v1/system/clock/status"
	fixed := time.Date(2026, 10, 17, 14, 7, 5, 321000000, time.UTC)
	handler := New(WithOperatorClockStatus(h.auth, func() time.Time { return fixed })).Handler()

	unauthorized := httptest.NewRequest(http.MethodGet, endpoint, nil)
	unauthorized.RemoteAddr = "127.0.0.1:17001"
	noSession := httptest.NewRecorder()
	handler.ServeHTTP(noSession, unauthorized)
	if noSession.Code != http.StatusUnauthorized {
		t.Fatalf("clock without browser session status=%d body=%s", noSession.Code, noSession.Body.String())
	}

	owner, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, endpoint, nil)
	request.RemoteAddr = "127.0.0.1:17002"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, request)
	if resp.Code != http.StatusOK {
		t.Fatalf("clock status=%d body=%s", resp.Code, resp.Body.String())
	}
	var value hubClockStatus
	if err := json.Unmarshal(resp.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.ServerUnixMS != fixed.UnixMilli() ||
		value.ServerUTC != fixed.Format(time.RFC3339Nano) ||
		value.DisplayZone != "Asia/Baghdad" ||
		!value.StatusIsSample || value.RTCBattery != "UNKNOWN" {
		t.Fatalf("unexpected Hub clock status: %+v", value)
	}
	for _, status := range []string{"UNKNOWN", "ABSENT", "PRESENT"} {
		if value.RTCStatus == status {
			goto RTCValid
		}
	}
	t.Fatalf("unrecognized RTC status: %q", value.RTCStatus)
RTCValid:
	for _, status := range []string{"UNKNOWN", "SYNCHRONIZED", "NOT_SYNCHRONIZED"} {
		if value.SyncStatus == status {
			return
		}
	}
	t.Fatalf("unrecognized sync status: %q", value.SyncStatus)
}

func TestOperatorClockStatusDoesNotExposeWriteEndpoint(t *testing.T) {
	h := newAuthHarness(t)
	handler := New(WithOperatorClockStatus(h.auth, time.Now)).Handler()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/system/clock/status", nil)
		req.RemoteAddr = "127.0.0.1:17003"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s clock status=%d, want 405", method, res.Code)
		}
	}
}
