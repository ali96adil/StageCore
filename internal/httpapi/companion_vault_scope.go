package httpapi

import (
	"context"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

type companionVaultReadScope struct {
	ctx          context.Context
	server       *Server
	token        string
	sessionID    string
	companionID  string
	assignmentID string
	roleID       string
	snapshotID   string
	projectID    string
	contentHash  string
	legacy       bool
}

func (s *Server) authorizeCompanionVaultRead(ctx context.Context, session domain.CompanionRuntimeSession, token, hash string) *companionVaultReadScope {
	assignment, err := s.companionVaultScope.GetActiveRoleAssignmentForCompanion(ctx, session.CompanionID)
	if err != nil {
		return nil
	}
	role, err := s.companionVaultScope.GetMachineRole(ctx, assignment.MachineRoleID)
	if err != nil || role.Retired {
		return nil
	}
	if role.RequiredRuntimeSnapshotID == nil {
		// Roles created before Runtime Snapshot binding remain compatible, but
		// their hash access is limited to media registered in the assigned
		// project's catalog.
		found, err := s.companionVaultScope.ProjectHasMediaContentHash(ctx, role.ProjectID, hash)
		if err != nil || !found {
			return nil
		}
		return &companionVaultReadScope{
			ctx: ctx, server: s, token: token, sessionID: session.ID, companionID: session.CompanionID,
			assignmentID: assignment.ID, roleID: role.ID, projectID: role.ProjectID,
			contentHash: hash, legacy: true,
		}
	}
	required, err := s.companionVaultScope.GetRuntimeSnapshot(ctx, *role.RequiredRuntimeSnapshotID)
	if err != nil || required.ProjectID != role.ProjectID || required.Status != domain.SnapshotPublished {
		return nil
	}
	manifest, err := snapshot.Decode(required.Manifest)
	if err != nil || manifest.ProjectID != required.ProjectID || manifest.RevisionID != required.RevisionID {
		return nil
	}
	for _, media := range manifest.RequiredMediaForRole(role.ID) {
		if media.ContentHash != hash {
			continue
		}
		asset, err := s.companionVaultScope.GetMediaAsset(ctx, media.MediaAssetID)
		if err != nil || asset.ProjectID != role.ProjectID {
			continue
		}
		version, err := s.companionVaultScope.GetMediaContentVersion(ctx, media.ContentVersionID)
		if err != nil || version.MediaAssetID != asset.ID || version.ContentHash != hash || version.SizeBytes != media.SizeBytes {
			continue
		}
		return &companionVaultReadScope{
			ctx: ctx, server: s, token: token, sessionID: session.ID, companionID: session.CompanionID,
			assignmentID: assignment.ID, roleID: role.ID, snapshotID: required.ID,
			projectID: role.ProjectID, contentHash: hash,
		}
	}
	return nil
}

func (scope *companionVaultReadScope) active() bool {
	session, err := scope.server.companionAuth.ValidateRuntimeSession(scope.ctx, scope.token)
	if err != nil || session.ID != scope.sessionID || session.CompanionID != scope.companionID {
		return false
	}
	current := scope.server.authorizeCompanionVaultRead(scope.ctx, session, scope.token, scope.contentHash)
	return current != nil && current.assignmentID == scope.assignmentID && current.roleID == scope.roleID &&
		current.projectID == scope.projectID && current.snapshotID == scope.snapshotID && current.legacy == scope.legacy
}
