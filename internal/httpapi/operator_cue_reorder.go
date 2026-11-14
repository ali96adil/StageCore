package httpapi

import (
	"net/http"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type cueReorderRequest struct {
	CueIDs []string `json:"cue_ids"`
}

func WithOperatorCueReorder(auth *userauth.Service, projectStore *store.Store) Option {
	return withOperatorCueReorder(auth, projectStore, cueReorderHooks{})
}

type cueReorderHooks struct {
	AfterDraft     func()
	BeforeResponse func()
}

func withOperatorCueReorder(auth *userauth.Service, projectStore *store.Store, hooks cueReorderHooks) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil {
			return
		}
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/cues/reorder", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body cueReorderRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			project, err := projectStore.GetProject(r.Context(), r.PathValue("project_id"))
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			sourceRevisionID := project.CurrentRevisionID
			sourceCues, err := projectStore.ListCues(r.Context(), sourceRevisionID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CUES_UNAVAILABLE"})
				return
			}
			if len(sourceCues) == 0 {
				writeCueMutationError(w, domain.ErrInvalidInput)
				return
			}
			sourceOrder := make(map[string]int, len(sourceCues))
			for _, cue := range sourceCues {
				sourceOrder[cue.ID] = cue.OrderIndex
			}
			if len(body.CueIDs) != len(sourceCues) {
				writeCueMutationError(w, domain.ErrInvalidInput)
				return
			}
			for _, cueID := range body.CueIDs {
				if _, ok := sourceOrder[cueID]; !ok {
					writeCueMutationError(w, domain.ErrInvalidInput)
					return
				}
			}

			writeCtx := store.WithCueDraftHistoryActor(
				projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit),
				session.User.ID,
			)
			revision, draftForked, err := projectStore.EnsureProjectDraftWithStatus(writeCtx, project.ID, session.User.ID, "Operator Web Cue reorder after Publish")
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if hooks.AfterDraft != nil {
				hooks.AfterDraft()
			}
			orderedIDs := append([]string(nil), body.CueIDs...)
			if revision.ID != sourceRevisionID {
				forkedCues, err := projectStore.ListCues(r.Context(), revision.ID)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CUES_UNAVAILABLE"})
					return
				}
				bySourceOrder := make(map[int]string, len(forkedCues))
				for _, cue := range forkedCues {
					bySourceOrder[cue.OrderIndex] = cue.ID
				}
				for i, sourceID := range body.CueIDs {
					mappedID := bySourceOrder[sourceOrder[sourceID]]
					if mappedID == "" {
						writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CUE_FORK_MAPPING_FAILED"})
						return
					}
					orderedIDs[i] = mappedID
				}
			}

			cues, err := projectStore.ReorderDraftCues(writeCtx, revision.ID, orderedIDs)
			if err != nil {
				if writeProjectDraftMutationDenied(w, err, draftForked) {
					return
				}
				writeCueMutationError(w, err)
				return
			}
			items := make([]cueView, 0, len(cues))
			for _, cue := range cues {
				items = append(items, makeCueView(cue))
			}
			if hooks.BeforeResponse != nil {
				hooks.BeforeResponse()
			}
			if !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"revision": makeRevisionView(revision), "cues": items})
		}))
	}
}
