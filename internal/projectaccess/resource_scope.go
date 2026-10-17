package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ResourceKind identifies a stable StageCore row whose Project association
// is defined in the database, not supplied by the requesting browser.
type ResourceKind string

const (
	ResourceCue       ResourceKind = "cue"
	ResourceRevision  ResourceKind = "revision"
	ResourceSnapshot  ResourceKind = "snapshot"
	ResourceSession   ResourceKind = "session"
)

// ResolveProjectForResource is a *read-only* scope resolver for future
// per-Project route middleware. It never interprets resource IDs as a Project
// identifier and never trusts user-provided ProjectID as proof of ownership.
// Missing IDs return ErrNotFound; DB errors are not collapsed into approval.
//
// WARNING: use for authorization of read routes only until session/GO
// dispatch-time authority and revoke/SHOW race fencing are designed.
func (s *Service) ResolveProjectForResource(ctx context.Context, kind ResourceKind, resourceID string) (string,error) {
	if s==nil || s.db==nil{return "",ErrDenied}
	resourceID=strings.TrimSpace(resourceID)
	if resourceID==""{return "",ErrNotFound}
	var query string
	switch kind {
	case ResourceCue:
		query=`SELECT r.project_id
			FROM cues c JOIN project_revisions r ON r.revision_id=c.revision_id
			WHERE c.cue_id=?`
	case ResourceRevision:
		query=`SELECT project_id FROM project_revisions WHERE revision_id=?`
	case ResourceSnapshot:
		query=`SELECT project_id FROM runtime_snapshots WHERE runtime_snapshot_id=?`
	case ResourceSession:
		// Ensure a Session cannot ambiguously claim a Project that differs
		// from the immutable Snapshot it references.
		query=`SELECT s.project_id FROM sessions s
			JOIN runtime_snapshots rs ON rs.runtime_snapshot_id=s.runtime_snapshot_id
				AND rs.project_id=s.project_id
			WHERE s.session_id=?`
	default:
		return "",ErrDenied
	}
	var projectID string
	err:=s.db.QueryRowContext(ctx,query,resourceID).Scan(&projectID)
	if errors.Is(err,sql.ErrNoRows){return "",ErrNotFound}
	if err!=nil{return "",fmt.Errorf("resolve %s owning Project: %w",kind,err)}
	if strings.TrimSpace(projectID)==""{return "",ErrDenied}
	return projectID,nil
}

// CheckProjectResourceRead is not a GO/Cue-write authorization API. For read
// routes it combines a server-resolved child resource ID with the *current*
// live Project capability, preventing an attacker from using a valid Cue or
// Snapshot ID from another Project beneath their own ProjectID URL.
func (s *Service) CheckProjectResourceRead(ctx context.Context, userID, expectedProjectID string, kind ResourceKind, resourceID string) error {
	if strings.TrimSpace(expectedProjectID)=="" {return ErrDenied}
	actual,err:=s.ResolveProjectForResource(ctx,kind,resourceID)
	if err!=nil {return err}
	if actual!=strings.TrimSpace(expectedProjectID){return ErrDenied}
	return s.Check(ctx,userID,actual,ProjectView)
}
