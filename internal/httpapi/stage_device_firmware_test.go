package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceupdate"
	"github.com/ali96adil/StageCore/internal/domain"
)

type fakeDeviceSessionValidator struct {
	token     string
	deviceID  string
}

func (f fakeDeviceSessionValidator) ValidateRuntimeSession(_ context.Context, token string) (domain.CompanionRuntimeSession, error) {
	if token != f.token {
		return domain.CompanionRuntimeSession{}, &sessionTestError{}
	}
	return domain.CompanionRuntimeSession{ID: "session-1", CompanionID: f.deviceID}, nil
}

type sessionTestError struct{}

func (*sessionTestError) Error() string { return "invalid test session" }

func importHTTPTestArtifact(t *testing.T, registry *deviceupdate.ArtifactRegistry, deviceID string) (deviceupdate.ArtifactMetadata, []byte) {
	t.Helper()
	payload := []byte("qualified-stagelaser-firmware")
	hash := sha256.Sum256(payload)
	metadata := deviceupdate.ArtifactMetadata{
		ArtifactID:       "018f2744-0cb0-7bf6-9637-4a3a467a7a21",
		DeviceID:         deviceID,
		ProfileID:        "stagecore.esp32-stagelaser",
		Version:          "0.1.0",
		SourceRevision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Qualification:    deviceupdate.QualificationQualified,
		SizeBytes:        int64(len(payload)),
		SHA256:           hex.EncodeToString(hash[:]),
		OriginalFilename: "stagelaser.bin",
		CreatedAt:        time.Date(2026, 10, 7, 10, 55, 0, 0, time.UTC),
	}
	if _, err := registry.ImportQualified(metadata, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	return metadata, payload
}

func TestStageDeviceFirmwareDownloadRequiresSessionAndExactDevice(t *testing.T) {
	deviceID := "23c45a07-7286-4afc-91d9-7e54df72aeee"
	registry, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, payload := importHTTPTestArtifact(t, registry, deviceID)

	server := New(WithStageDeviceFirmwareArtifacts(
		fakeDeviceSessionValidator{token: "good-token", deviceID: deviceID},
		registry,
	))
	path := "/api/v1/stage-device-firmware/artifacts/" + metadata.ArtifactID + "/firmware.bin"

	req := httptest.NewRequest(http.MethodGet, "https://hub.local"+path, nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without token status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "https://hub.local"+path, nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "StageCoreSession good-token")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), payload) {
		t.Fatalf("download body = %q", rec.Body.Bytes())
	}
	if got := rec.Header().Get("X-Content-SHA256"); got != metadata.SHA256 {
		t.Fatalf("sha header = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}

	otherServer := New(WithStageDeviceFirmwareArtifacts(
		fakeDeviceSessionValidator{
			token:    "other-token",
			deviceID: "12345678-1234-1234-1234-123456789012",
		},
		registry,
	))
	req = httptest.NewRequest(http.MethodGet, "https://hub.local"+path, nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "StageCoreSession other-token")
	rec = httptest.NewRecorder()
	otherServer.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-device status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStageDeviceFirmwareDownloadRejectsPlainNonLoopbackTransport(t *testing.T) {
	deviceID := "23c45a07-7286-4afc-91d9-7e54df72aeee"
	registry, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := importHTTPTestArtifact(t, registry, deviceID)
	server := New(WithStageDeviceFirmwareArtifacts(
		fakeDeviceSessionValidator{token: "good-token", deviceID: deviceID},
		registry,
	))

	req := httptest.NewRequest(
		http.MethodGet,
		"http://hub.local/api/v1/stage-device-firmware/artifacts/"+metadata.ArtifactID+"/firmware.bin",
		nil,
	)
	req.RemoteAddr = "192.0.2.10:40000"
	req.TLS = nil
	req.Header.Set("Authorization", "StageCoreSession good-token")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain transport status = %d", rec.Code)
	}
}
