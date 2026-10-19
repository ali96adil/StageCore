package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/userauth"
)

var ErrCutoverPreviewTooLarge = errors.New("projectaccess: cutover preview requires pagination")

// These limits intentionally avoid a misleading partial impact report. A
// cursor/streamed review is required for larger installations.
const (
	maxPreviewUsers = 128
	maxPreviewProjects = 128
	maxPreviewPairs = 4096
)

// CutoverProjectLoss identifies a Project visible under the legacy global
// browser role but unavailable under the prepared Project-scoped grant.
// This is Owner-only security metadata, not a public Project listing.
type CutoverProjectLoss struct {
	ProjectID string `json:"project_id"`
	ProjectName string `json:"project_name"`
}

// CutoverUserImpact compares CURRENT browser route eligibility with prospective
// Project grant eligibility. It is NOT a simulation of dispatch/publish safety.
type CutoverUserImpact struct {
	UserID string `json:"user_id"`
	Username string `json:"username"`
	Role string `json:"role"`
	Enabled bool `json:"enabled"`
	LegacyVisibleProjects int `json:"legacy_visible_projects"`
	PreparedVisibleProjects int `json:"prepared_visible_projects"`
	WillLoseProjectView []CutoverProjectLoss `json:"will_lose_project_view"`
	LegacyGOPermittedProjects int `json:"legacy_go_role_eligible_projects"`
	PreparedGOPermittedProjects int `json:"prepared_go_granted_projects"`
	PreparedStopGrantedProjects int `json:"prepared_stop_granted_projects"`
	PreparedBlackoutEngageGrantedProjects int `json:"prepared_blackout_engage_granted_projects"`
	GOWithoutIndependentSafety []CutoverProjectLoss `json:"go_without_independent_safety"`
	WillLoseGORoleEligibility []CutoverProjectLoss `json:"will_lose_go_role_eligibility"`
}

type CutoverImpactPreview struct {
	ReadOnly bool `json:"read_only"`
	Enforced bool `json:"enforced"`
	ReadyForCutover bool `json:"ready_for_cutover"`
	Status string `json:"status"`
	ActiveOperationalSession string `json:"active_operational_session,omitempty"`
	ProjectCount int `json:"project_count"`
	UserCount int `json:"user_count"`
	Users []CutoverUserImpact `json:"users"`
	Blockers []string `json:"blockers"`
	AuthorityCaveat string `json:"authority_caveat"`
}

type cutoverSubject struct {
	id, username, role string
	enabled bool
}
type cutoverProject struct{ id, name string }

// PreviewProjectCutover produces an Owner-only, read-only deterministic report
// from one SQLite read transaction. NO permission, session or device is
// modified. It deliberately never returns ready_for_cutover=true: the route
// matrix, physical emergency STOP policy, legacy backup/import and companion
// boundaries must be cut over atomically elsewhere first.
//
// Project grants do NOT entitle a non-Owner to see this impact report.
func (s *Service) PreviewProjectCutover(ctx context.Context, actingOwnerID string) (CutoverImpactPreview, error) {
	if s == nil || s.db == nil || strings.TrimSpace(actingOwnerID) == "" {
		return CutoverImpactPreview{}, ErrDenied
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil { return CutoverImpactPreview{}, fmt.Errorf("open read-only cutover preview: %w", err) }
	defer tx.Rollback()

	var role string
	var enabled int
	err = tx.QueryRowContext(ctx,
		`SELECT role, enabled FROM local_users WHERE user_id=?`, actingOwnerID).Scan(&role,&enabled)
	if errors.Is(err,sql.ErrNoRows) { return CutoverImpactPreview{}, ErrDenied }
	if err != nil { return CutoverImpactPreview{}, fmt.Errorf("read cutover reviewer: %w",err) }
	if role!="OWNER" || enabled!=1 { return CutoverImpactPreview{}, ErrDenied }

	projects := make([]cutoverProject,0)
	rows, err := tx.QueryContext(ctx, `SELECT project_id, name FROM projects ORDER BY name COLLATE NOCASE, project_id LIMIT ?`, maxPreviewProjects+1)
	if err!=nil {return CutoverImpactPreview{},fmt.Errorf("read cutover projects: %w",err)}
	for rows.Next() {
		var p cutoverProject
		if err:=rows.Scan(&p.id,&p.name);err!=nil {rows.Close();return CutoverImpactPreview{},err}
		projects=append(projects,p)
	}
	if err:=rows.Err();err!=nil {rows.Close();return CutoverImpactPreview{},err}
	if err:=rows.Close();err!=nil {return CutoverImpactPreview{},err}
	if len(projects)>maxPreviewProjects {return CutoverImpactPreview{},ErrCutoverPreviewTooLarge}

	users := make([]cutoverSubject,0)
	rows, err = tx.QueryContext(ctx,
		`SELECT user_id, username, role, enabled FROM local_users ORDER BY username COLLATE NOCASE, user_id LIMIT ?`,
		maxPreviewUsers+1)
	if err!=nil{return CutoverImpactPreview{},fmt.Errorf("read cutover users: %w",err)}
	for rows.Next() {
		var u cutoverSubject
		var on int
		if err:=rows.Scan(&u.id,&u.username,&u.role,&on);err!=nil {rows.Close();return CutoverImpactPreview{},err}
		u.enabled=on==1
		users=append(users,u)
	}
	if err:=rows.Err();err!=nil{rows.Close();return CutoverImpactPreview{},err}
	if err:=rows.Close();err!=nil{return CutoverImpactPreview{},err}
	if len(users)>maxPreviewUsers || len(users)*len(projects)>maxPreviewPairs {
		return CutoverImpactPreview{},ErrCutoverPreviewTooLarge
	}

	// Build grant index within the SAME database read transaction so one
	// report cannot combine old Projects with newer user/grant sets.
	grants:=make(map[string]map[string]map[Permission]bool)
	rows,err=tx.QueryContext(ctx,
		`SELECT project_id, user_id, permission FROM project_permission_grants ORDER BY project_id,user_id,permission`)
	if err!=nil{return CutoverImpactPreview{},fmt.Errorf("read prepared Project grants: %w",err)}
	for rows.Next(){
		var p,u string
		var cap Permission
		if err:=rows.Scan(&p,&u,&cap);err!=nil {rows.Close();return CutoverImpactPreview{},err}
		if !permissionSet[cap] {rows.Close();return CutoverImpactPreview{},fmt.Errorf("stored Project permission is unknown")}
		if grants[u]==nil {grants[u]=make(map[string]map[Permission]bool)}
		if grants[u][p]==nil {grants[u][p]=make(map[Permission]bool)}
		grants[u][p][cap]=true
	}
	if err:=rows.Err();err!=nil {rows.Close();return CutoverImpactPreview{},err}
	if err:=rows.Close();err!=nil{return CutoverImpactPreview{},err}

	var activeType string
	err=tx.QueryRowContext(ctx,
		`SELECT session_type FROM sessions WHERE status='ACTIVE'
		 ORDER BY CASE session_type WHEN 'SHOW' THEN 0 WHEN 'REHEARSAL' THEN 1 ELSE 2 END,
		  started_at_us DESC LIMIT 1`).Scan(&activeType)
	if errors.Is(err,sql.ErrNoRows) {activeType=""} else if err!=nil{
		return CutoverImpactPreview{},fmt.Errorf("read operational state: %w",err)
	}

	result:=CutoverImpactPreview{
		ReadOnly:true, Enforced:false, ReadyForCutover:false,
		Status:"BLOCKED_SECURITY_CUTOVER_INCOMPLETE",
		ProjectCount:len(projects), UserCount:len(users),
		Users:make([]CutoverUserImpact,0,len(users)),
		Blockers:[]string{
			"PROJECT_DIRECT_ID_AND_DEVICE_ROUTE_CUTOVER_REQUIRED",
			"EMERGENCY_STOP_AND_BLACKOUT_AUTHORITY_UNRESOLVED",
			"INDEPENDENT_OFFLINE_STOP_PHYSICAL_QUALIFICATION_REQUIRED",
			"CAPSULE_IMPORT_VAULT_DOWNLOAD_AND_EVENT_ISOLATION_REQUIRED",
			"OWNER_APPROVAL_BACKUP_ROLLBACK_AND_NON_SHOW_VALIDATION_REQUIRED",
		},
		AuthorityCaveat:"Legacy GO counts mean only global runtime.control role eligibility; prepared GO counts mean only project.view + show.go grants. Neither asserts a safe physical GO/STOP authorization.",
	}
	if activeType!="" {
		result.ActiveOperationalSession=activeType
		result.Blockers=append(result.Blockers,"ACTIVE_OPERATIONAL_SESSION_BLOCKS_CUTOVER")
	}

	for _,u:=range users {
		impact:=CutoverUserImpact{
			UserID:u.id,Username:u.username,Role:u.role,Enabled:u.enabled,
			WillLoseProjectView:make([]CutoverProjectLoss,0),
			WillLoseGORoleEligibility:make([]CutoverProjectLoss,0),
			GOWithoutIndependentSafety:make([]CutoverProjectLoss,0),
		}
		legacyView:=u.enabled && userauth.Authorize(u.role,userauth.PermissionProjectRead)==nil
		legacyGO:=u.enabled && userauth.Authorize(u.role,userauth.PermissionRuntimeControl)==nil
		for _,p:=range projects {
			caps:=grants[u.id][p.id]
			isOwner:=u.enabled && u.role==userauth.RoleOwner
			roleAllows:=u.enabled && (u.role==userauth.RoleOperator||u.role==userauth.RoleTechnician||u.role==userauth.RoleViewer)
			preparedView:=isOwner || (roleAllows && caps[ProjectView])
			preparedGO:=isOwner || (roleAllows && u.role!=userauth.RoleViewer && caps[ProjectView] && caps[ShowGO])
			preparedStop:=isOwner || (roleAllows && u.role!=userauth.RoleViewer && caps[ProjectView] && caps[ShowStop])
			preparedBlackout:=isOwner || (roleAllows && u.role!=userauth.RoleViewer && caps[ProjectView] && caps[ShowBlackoutEngage])
			if legacyView {impact.LegacyVisibleProjects++}
			if preparedView {impact.PreparedVisibleProjects++}
			if legacyGO {impact.LegacyGOPermittedProjects++}
			if preparedGO {impact.PreparedGOPermittedProjects++}
			if preparedStop {impact.PreparedStopGrantedProjects++}
			if preparedBlackout {impact.PreparedBlackoutEngageGrantedProjects++}
			if preparedGO && (!preparedStop || !preparedBlackout) {
				impact.GOWithoutIndependentSafety=append(impact.GOWithoutIndependentSafety,CutoverProjectLoss{ProjectID:p.id,ProjectName:p.name})
			}
			if legacyView && !preparedView {
				impact.WillLoseProjectView=append(impact.WillLoseProjectView,CutoverProjectLoss{ProjectID:p.id,ProjectName:p.name})
			}
			if legacyGO && !preparedGO {
				impact.WillLoseGORoleEligibility=append(impact.WillLoseGORoleEligibility,CutoverProjectLoss{ProjectID:p.id,ProjectName:p.name})
			}
		}
		if len(impact.GOWithoutIndependentSafety)>0 {
			result.Blockers=append(result.Blockers,"GO_WITHOUT_INDEPENDENT_STOP_AND_BLACKOUT_GRANTS")
		}
		result.Users=append(result.Users,impact)
	}
	// Report ordering is tied to the query's stable username ordering, and
	// the nested Project losses are sorted by Project name/id.
	sort.Strings(result.Blockers)
	uniqueBlockers:=result.Blockers[:0]
	for _,blocker:=range result.Blockers {
		if len(uniqueBlockers)==0 || uniqueBlockers[len(uniqueBlockers)-1]!=blocker {uniqueBlockers=append(uniqueBlockers,blocker)}
	}
	result.Blockers=uniqueBlockers
	if err:=tx.Commit();err!=nil{return CutoverImpactPreview{},fmt.Errorf("close cutover preview transaction: %w",err)}
	return result,nil
}
