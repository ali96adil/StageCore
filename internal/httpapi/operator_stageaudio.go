package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/stageaudio"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// StageAudioLocal is the typed, same-host private Unix client. The web caller
// can never choose a socket path or submit raw mpv / BlueZ commands.
type StageAudioLocal interface {
	Call(context.Context, stageaudio.Request) (stageaudio.LocalReply, error)
}

// WithOperatorStageAudio exposes an optional, OFF-by-default rehearsal audio
// endpoint. A missing sidecar must not affect StageCore Cues or GO.
func WithOperatorStageAudio(auth *userauth.Service, stageStore *store.Store, client StageAudioLocal, audit *securityaudit.Service, fences ...*stageaudio.ShowFence) Option {
	var fence *stageaudio.ShowFence
	if len(fences) > 0 { fence = fences[0] }
	return WithScopedOperatorStageAudio(auth, stageStore, client, audit, fence, nil)
}

func WithScopedOperatorStageAudio(auth *userauth.Service, stageStore *store.Store, client StageAudioLocal, audit *securityaudit.Service, fence *stageaudio.ShowFence, scope *StageAudioProjectScope) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil { return }
		checkRuntime := func(ctx context.Context) (domain.SessionType, error) {
			return stageStore.ActiveOperationalSessionType(ctx)
		}
		get := func(operation string) http.HandlerFunc {
			return withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, sessionUser userauth.Session) {
				w.Header().Set("Cache-Control", "no-store")
				if !scope.authorize(r, sessionUser.User.ID, false) {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "STAGEAUDIO_PROJECT_UNAVAILABLE"})
					return
				}
				if scope != nil && operation == "FILES" {
					choices, err := scope.choices(r.Context(), sessionUser.User.ID)
					if err != nil {
						writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "STAGEAUDIO_MEDIA_UNAVAILABLE"})
						return
					}
					writeJSON(w, http.StatusOK, map[string]any{"media": choices})
					return
				}
				session, err := checkRuntime(r.Context())
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code":"STAGEAUDIO_SESSION_STATUS_UNAVAILABLE"})
					return
				}
				if client == nil {
					writeJSON(w, http.StatusOK, map[string]any{
						"available": false, "active_session": session, "error_code": "STAGEAUDIO_NOT_CONFIGURED",
					})
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), stageAudioTimeout(operation))
				defer cancel()
				result, err := client.Call(ctx, stageaudio.Request{Operation: operation})
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"available": false, "active_session": session, "error_code": "STAGEAUDIO_OFFLINE",
					})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"available": true, "active_session": session, "status": result.Status,
					"outputs": result.Outputs, "bluetooth_devices": result.BluetoothDevices,
					"media": result.Media,
				})
			})
		}
		s.mux.HandleFunc("GET /api/v1/system/audio/status", get("STATUS"))
		s.mux.HandleFunc("GET /api/v1/system/audio/outputs", get("OUTPUTS"))
		s.mux.HandleFunc("GET /api/v1/system/audio/bluetooth", get("BT_DEVICES"))
		s.mux.HandleFunc("GET /api/v1/system/audio/media", get("FILES"))
		s.mux.HandleFunc("POST /api/v1/system/audio/command", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			w.Header().Set("Cache-Control", "no-store")
			if client == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "STAGEAUDIO_NOT_CONFIGURED"})
				return
			}
			var command stageaudio.Request
			if !decodeBoundedJSON(w, r, &command) { return }
			command.Operation = strings.ToUpper(strings.TrimSpace(command.Operation))
			command.Deck = strings.ToUpper(strings.TrimSpace(command.Deck))
			if !allowStageAudioCommand(command) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "STAGEAUDIO_COMMAND_INVALID"})
				return
			}
			// Fail closed whenever show state cannot be read or SHOW is active.
			// A sidecar is not a qualified SHOW output and must never receive
			// operator audio commands during an active SHOW session.
			ctx, cancel := context.WithTimeout(r.Context(), stageAudioTimeout(command.Operation))
			defer cancel()
			release, err := fence.Acquire(ctx)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "STAGEAUDIO_FENCE_BUSY"})
				return
			}
			defer release()
			if !scope.authorize(r, session.User.ID, true) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "STAGEAUDIO_PROJECT_UNAVAILABLE"})
				return
			}
			active, err := checkRuntime(ctx)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "STAGEAUDIO_SESSION_STATUS_UNAVAILABLE"})
				return
			}
			if active == domain.SessionShow {
				appendAudit(r, audit, securityaudit.Event{
					EventType: "stageaudio.command", ActorUserID: session.User.ID,
					ActorUsername: session.User.Username, Source: "operator-web",
					ResourceType: "stageaudio", ResourceID: "local-hub",
					Result: securityaudit.ResultRejected, Reason: "SHOW_ACTIVE",
					Metadata: map[string]any{"operation": command.Operation, "deck": command.Deck},
				})
				writeJSON(w, http.StatusLocked, map[string]any{"error_code": "STAGEAUDIO_SHOW_LOCKED"})
				return
			}
			if scope != nil && (command.Operation == "LOAD" || command.Operation == "PLAY") {
				rehearsal, err := stageStore.ActiveSessionForProject(ctx, scope.ProjectID)
				if err != nil || rehearsal == nil || rehearsal.Type != domain.SessionRehearsal {
					writeJSON(w, http.StatusConflict, map[string]any{"error_code": "STAGEAUDIO_REHEARSAL_REQUIRED"})
					return
				}
				if command.Operation == "LOAD" {
					command, err = scope.resolveLoad(ctx, stageStore, session.User.ID, command)
					if err != nil || !scope.authorize(r, session.User.ID, true) {
						writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "STAGEAUDIO_MEDIA_UNAVAILABLE"})
						return
					}
				} else {
					status, err := client.Call(ctx, stageaudio.Request{Operation: "STATUS"})
					deck := status.Status.Decks[command.Deck]
					resolved, scopeErr := scope.resolveLoad(ctx, stageStore, session.User.ID, stageaudio.Request{File: deck.VaultVersionID})
					if err != nil || scopeErr != nil || deck.File == "" || deck.File != resolved.File || !scope.authorize(r, session.User.ID, true) {
						writeJSON(w, http.StatusConflict, map[string]any{"error_code": "STAGEAUDIO_SCOPED_LOAD_REQUIRED"})
						return
					}
				}
			}
			result, err := client.Call(ctx, command)
			if err != nil { fence.MarkUncertain() }
			auditResult := securityaudit.ResultSuccess
			reason := "LOCAL_COMMAND_ACK"
			if err != nil { auditResult, reason = securityaudit.ResultFailed, "SIDECAR_COMMAND_FAILED" }
			appendAudit(r, audit, securityaudit.Event{
				EventType: "stageaudio.command", ActorUserID: session.User.ID,
				ActorUsername: session.User.Username, Source: "operator-web",
				ResourceType: "stageaudio", ResourceID: "local-hub",
				Result: auditResult, Reason: reason,
				Metadata: map[string]any{"operation": command.Operation, "deck": command.Deck},
			})
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "STAGEAUDIO_COMMAND_FAILED"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"available": true, "status": result.Status, "bluetooth_devices": result.BluetoothDevices,
			})
		}))
	}
}

func stageAudioTimeout(operation string) time.Duration {
	if operation == "BT_CONNECT" || operation == "BT_DEVICES" { return 25 * time.Second }
	return 5 * time.Second
}

// Permit only narrow, typed operations; raw mpv commands cannot be injected.
func allowStageAudioCommand(command stageaudio.Request) bool {
	if command.VaultVersionID != "" { return false }
	switch command.Operation {
	case "LOAD":
		return (command.Deck == "A" || command.Deck == "B") && len(command.File) > 0 && len(command.File) <= 256
	case "PLAY", "PAUSE", "STOP", "VOLUME", "LOOP":
		return command.Deck == "A" || command.Deck == "B"
	case "MASTER_VOLUME":
		return command.Deck == "" && command.Volume != nil
	case "SELECT_OUTPUT":
		return command.Deck == "" && len(command.OutputID) > 0 && len(command.OutputID) <= 256
	case "BT_CONNECT":
		return command.Deck == "" && len(command.Address) == 17
	default:
		return false
	}
}
