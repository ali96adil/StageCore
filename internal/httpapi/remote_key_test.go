package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginRemoteKeyUsesPeerIPNotSourcePort(t *testing.T) {
	first := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	first.RemoteAddr = "10.20.30.40:41001"
	second := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	second.RemoteAddr = "10.20.30.40:51999"
	if loginRemoteKey(first) != "10.20.30.40" || loginRemoteKey(second) != "10.20.30.40" {
		t.Fatalf("remote keys differ: %q %q", loginRemoteKey(first), loginRemoteKey(second))
	}
}
