package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestFreshHubCanClaimFirstOwnerThenLoginThroughSupportedAPI(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	hub, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := hub.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := userauth.New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithFirstOwnerBootstrap(hub, audit),
		WithUserAuth(auth, hub, audit),
	).Handler()

	password := "stagecore first owner password"
	body, _ := json.Marshal(map[string]string{
		"setup_code": setup.Code,
		"username":   "owner",
		"password":   password,
	})
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body))
	claimReq.RemoteAddr = "127.0.0.1:18001"
	claimReq.Header.Set("Content-Type", "application/json")
	claimRes := httptest.NewRecorder()
	handler.ServeHTTP(claimRes, claimReq)
	if claimRes.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", claimRes.Code, claimRes.Body.String())
	}
	if strings.Contains(claimRes.Body.String(), setup.Code) || strings.Contains(claimRes.Body.String(), password) {
		t.Fatal("bootstrap response leaked setup code or password")
	}
	identity, err := hub.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.BootstrapState != hubsecurity.BootstrapClaimed {
		t.Fatalf("bootstrap state=%q, want CLAIMED", identity.BootstrapState)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body))
	replayReq.RemoteAddr = "127.0.0.1:18002"
	replayReq.Header.Set("Content-Type", "application/json")
	replayRes := httptest.NewRecorder()
	handler.ServeHTTP(replayRes, replayReq)
	if replayRes.Code != http.StatusConflict || !strings.Contains(replayRes.Body.String(), "BOOTSTRAP_ALREADY_CLAIMED") {
		t.Fatalf("bootstrap replay status=%d body=%s", replayRes.Code, replayRes.Body.String())
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "owner", "password": password})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.RemoteAddr = "127.0.0.1:18003"
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	handler.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK || !strings.Contains(loginRes.Body.String(), `"role":"OWNER"`) {
		t.Fatalf("login status=%d body=%s", loginRes.Code, loginRes.Body.String())
	}

	records, err := audit.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var sawBootstrap bool
	for _, record := range records {
		if record.EventType == "hub.bootstrap.owner" && record.Result == securityaudit.ResultSuccess && record.ActorUsername == "owner" {
			sawBootstrap = true
		}
		if strings.Contains(string(record.Metadata), setup.Code) || strings.Contains(record.Reason, setup.Code) || strings.Contains(string(record.Metadata), password) {
			t.Fatal("security audit leaked bootstrap credential material")
		}
	}
	if !sawBootstrap {
		t.Fatal("successful first OWNER bootstrap was not audited")
	}
}

func TestFirstOwnerBootstrapRejectsInvalidCodeWithoutClaiming(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	hub, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.GenerateSetupCode(ctx); err != nil {
		t.Fatal(err)
	}
	handler := New(WithFirstOwnerBootstrap(hub, nil)).Handler()
	body := []byte(`{"setup_code":"AAAA-BBBB-CCCC-DDDD","username":"owner","password":"stagecore first owner password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:18004"
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || !strings.Contains(res.Body.String(), "SETUP_CODE_INVALID") {
		t.Fatalf("invalid setup status=%d body=%s", res.Code, res.Body.String())
	}
	identity, err := hub.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.BootstrapState != hubsecurity.BootstrapUnclaimed {
		t.Fatalf("invalid setup changed bootstrap state to %q", identity.BootstrapState)
	}
}
