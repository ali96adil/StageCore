package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/stageaudio"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

type StageAudioProjectScope struct {
	ProjectID string
	Access    *projectaccess.Service
	Vault     *vault.Vault
}

type audioMediaChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *StageAudioProjectScope) authorize(r *http.Request, userID string, mutate bool) bool {
	if s == nil {
		return true
	}
	if s.Access == nil || s.Vault == nil || s.ProjectID == "" || r.URL.Query().Get("project_id") != s.ProjectID {
		return false
	}
	if s.Access.Check(r.Context(), userID, s.ProjectID, projectaccess.MediaRead) != nil {
		return false
	}
	return !mutate || s.Access.Check(r.Context(), userID, s.ProjectID, projectaccess.AudioControl) == nil
}

func (s *StageAudioProjectScope) choices(ctx context.Context, userID string) ([]audioMediaChoice, error) {
	workspace, err := s.Access.ListScopedProjectFiles(ctx, userID, s.ProjectID)
	if err != nil {
		return nil, err
	}
	out := make([]audioMediaChoice, 0)
	for _, file := range workspace.Files {
		if file.Category == "AUDIO" && file.ContentVersionID != "" {
			name := file.OriginalFilename
			if name == "" {
				name = file.MediaAssetName
			}
			out = append(out, audioMediaChoice{ID: file.ContentVersionID, Name: name})
		}
	}
	return out, nil
}

func (s *StageAudioProjectScope) resolveLoad(ctx context.Context, st *store.Store, userID string, cmd stageaudio.Request) (stageaudio.Request, error) {
	choices, err := s.choices(ctx, userID)
	if err != nil {
		return cmd, err
	}
	for _, choice := range choices {
		if choice.ID != cmd.File {
			continue
		}
		version, err := st.GetMediaContentVersion(ctx, choice.ID)
		if err != nil {
			return cmd, err
		}
		asset, err := st.GetMediaAsset(ctx, version.MediaAssetID)
		if err != nil || asset.ProjectID != s.ProjectID {
			break
		}
		file, object, err := s.Vault.OpenObject(ctx, version.ContentHash)
		if err != nil {
			return cmd, err
		}
		_ = file.Close()
		if object.SizeBytes != version.SizeBytes {
			break
		}
		cmd.Operation, cmd.File, cmd.VaultVersionID = "LOAD_VAULT", version.ContentHash, version.ID
		return cmd, nil
	}
	return cmd, errors.New("audio content version unavailable in this project")
}
