package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/usbdmx"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/google/uuid"
)

type usbDMXRequest struct {
	RequestID  string                `json:"request_id"`
	SnapshotID string                `json:"runtime_snapshot_id"`
	Confirm    string                `json:"confirm"`
	DurationMS int64                 `json:"duration_ms"`
	Channels   []usbdmx.ChannelValue `json:"channels"`
}

func WithOperatorUSBManualDMX(auth *userauth.Service, st *store.Store, manual *usbdmx.HubManual) Option {
	return func(s *Server) {
		if manual == nil {
			return
		}
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/usb-dmx/manual", withAuthenticatedSession(auth, true,
			func(w http.ResponseWriter, r *http.Request, session userauth.Session, token string) {
				projectID := r.PathValue("project_id")
				if projectID != manual.ProjectID || session.User.Role != userauth.RoleOwner {
					writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "USB_DMX_UNAVAILABLE"})
					return
				}
				var req usbDMXRequest
				if !decodeBoundedJSON(w, r, &req) {
					return
				}
				if req.Confirm == "STOP_USB_DMX" {
					if err := manual.Stop(r.Context(), projectID); err != nil {
						writeJSON(w, http.StatusConflict, map[string]string{"error_code": "USB_DMX_STOP_UNCONFIRMED"})
						return
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
				parsedID, idErr := uuid.Parse(req.RequestID)
				continuous := req.Confirm == "CONTINUOUS_USB_DMX"
				maxDuration := int64(10000)
				if continuous {
					if !manual.EnableContinuous {
						writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "USB_DMX_UNAVAILABLE"})
						return
					}
					maxDuration = usbdmx.MaxContinuousLease.Milliseconds()
				}
				if (req.Confirm != "MANUAL_USB_DMX" && !continuous) || req.DurationMS < 1 ||
					req.DurationMS > maxDuration || idErr != nil || parsedID == uuid.Nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "USB_DMX_INVALID_REQUEST"})
					return
				}
				authorize := func(ctx context.Context) error {
					current, err := auth.ValidatePassive(ctx, token)
					if err != nil || current.ID != session.ID || current.User.ID != session.User.ID || current.User.Role != userauth.RoleOwner {
						return errors.New("owner authorization ended")
					}
					active, err := st.ActiveOperationalSessionType(ctx)
					if err != nil || active != "" {
						return errors.New("operational session is active or unknown")
					}
					snap, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, projectID)
					if err != nil || snap == nil || snap.ID != req.SnapshotID {
						return errors.New("published snapshot changed")
					}
					return nil
				}
				if err := authorize(r.Context()); err != nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error_code": "USB_DMX_AUTHORITY_UNAVAILABLE"})
					return
				}
				commandID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("stagecore.usb-dmx:"+projectID+":"+parsedID.String())).String()
				command := contracts.CommandEnvelope{CommandID: commandID, CommandType: "usb-dmx.manual",
					SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(), ProjectID: projectID,
					RuntimeSnapshotID: req.SnapshotID, Issuer: session.User.ID, IdempotencyKey: commandID, Priority: "P1",
					Payload: json.RawMessage(`{}`)}
				if _, reserved, err := st.ReserveCommand(r.Context(), command); err != nil || !reserved {
					writeJSON(w, http.StatusConflict, map[string]string{"error_code": "USB_DMX_REQUEST_NOT_RESERVED"})
					return
				}
				duration := time.Duration(req.DurationMS) * time.Millisecond
				var err error
				if continuous {
					err = manual.RunContinuous(r.Context(), req.Channels, duration, authorize)
				} else {
					err = manual.Run(r.Context(), usbdmx.ManualRequest{Channels: req.Channels, Duration: duration}, authorize)
				}
				status := contracts.CommandCompleted
				if err != nil {
					status = contracts.CommandFailed
				}
				ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
				defer cancel()
				if finishErr := st.FinishCommand(ctx, commandID, contracts.CommandResult{CommandID: commandID, Status: status}); finishErr != nil || err != nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error_code": "USB_DMX_MANUAL_UNCONFIRMED"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"zero_frame_submitted": true, "physical_output_verified": false})
			}))
	}
}
