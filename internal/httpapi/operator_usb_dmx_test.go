package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/usbdmx"
	"github.com/google/uuid"
)

type usbDMXFixturePort struct{ frames [][usbdmx.FrameSize]byte }

func (p *usbDMXFixturePort) WritePacket(_ context.Context, frame [usbdmx.FrameSize]byte) error {
	p.frames = append(p.frames, frame)
	return nil
}

func (p *usbDMXFixturePort) Close() error { return nil }

func TestUSBManualRouteRequiresExplicitScopeAndDoesNotReplay(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	p, rev, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "USB fixture", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetRevisionStatus(ctx, rev.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(st).Create(ctx, rev.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	port := &usbDMXFixturePort{}
	manual := &usbdmx.HubManual{ProjectID: p.ID, Output: usbdmx.NewManualOutput(true, func(context.Context) (usbdmx.PacketPort, error) { return port, nil })}
	handler := New(WithOperatorUSBManualDMX(h.auth, st, manual)).Handler()
	data := usbDMXRequest{RequestID: uuid.NewString(), SnapshotID: published.ID, Confirm: "MANUAL_USB_DMX",
		DurationMS: 1, Channels: []usbdmx.ChannelValue{{Channel: 1, Value: 125}}}
	send := func(to http.Handler, project string, payload usbDMXRequest) int {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project+"/usb-dmx/manual", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:9126"
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		out := httptest.NewRecorder()
		to.ServeHTTP(out, req)
		return out.Code
	}
	if send(New(WithOperatorUSBManualDMX(h.auth, st, nil)).Handler(), p.ID, data) != http.StatusNotFound ||
		send(handler, "other-project", data) != http.StatusNotFound || len(port.frames) != 0 {
		t.Fatal("disabled or cross-project route produced output")
	}
	if send(handler, p.ID, data) != http.StatusOK || len(port.frames) != 3 ||
		port.frames[0] != usbdmx.ZeroFrame() || port.frames[2] != usbdmx.ZeroFrame() {
		t.Fatal("confirmed lease did not submit zero, data, zero")
	}
	if send(handler, p.ID, data) != http.StatusConflict || len(port.frames) != 3 {
		t.Fatal("duplicate lease dispatched again")
	}
	if err := manual.SetBlackout(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}
	data.RequestID = uuid.NewString()
	if send(handler, p.ID, data) != http.StatusConflict || len(port.frames) != 3 {
		t.Fatal("manual output started during EDIT blackout")
	}
	if err := manual.SetBlackout(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, published.ID, domain.SessionRehearsal, "blocks manual"); err != nil {
		t.Fatal(err)
	}
	data.RequestID = uuid.NewString()
	if send(handler, p.ID, data) != http.StatusConflict || len(port.frames) != 3 {
		t.Fatal("manual output started during operational session")
	}

}
