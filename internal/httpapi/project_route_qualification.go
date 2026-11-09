package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type projectQualificationKey struct{}
type projectRouteGrantKey struct{}

type projectRouteGrant struct {
	access *projectaccess.Service
	auth   *userauth.Service
	token  string
	user   userauth.Session
	needed []projectaccess.Permission
}

// WithProjectRouteQualification applies the explicitly reviewed browser route
// policy. Keep it out of the Hub assembly until the remaining routes qualify.
func WithProjectRouteQualification(access *projectaccess.Service) Option {
	return func(s *Server) { s.projectRouteQualification = access }
}

const projectRoute = "/api/v1/projects/{project_id}"
const envRoute = projectRoute + "/revisions/{revision_id}/execution-environments"
const envItem = envRoute + "/{execution_environment_id}"

var projectRoutePermissions = map[string][]projectaccess.Permission{
	"GET " + projectRoute + "/cues":                                                {projectaccess.ProjectView},
	"POST " + projectRoute + "/cues":                                               {projectaccess.CueEdit},
	"POST " + projectRoute + "/cues/reorder":                                       {projectaccess.CueEdit},
	"PUT " + projectRoute + "/cues/{cue_id}":                                       {projectaccess.CueEdit},
	"POST " + projectRoute + "/cues/{cue_id}/duplicate":                            {projectaccess.CueEdit},
	"DELETE " + projectRoute + "/cues/{cue_id}":                                    {projectaccess.CueEdit},
	"POST " + projectRoute + "/publish":                                            {projectaccess.SnapshotPublish},
	"POST " + projectRoute + "/configuration/draft":                                {projectaccess.ProjectEdit},
	"DELETE " + projectRoute + "/configuration/draft":                              {projectaccess.ProjectEdit},
	"GET " + envRoute:                                                              {projectaccess.ProjectView},
	"POST " + envRoute:                                                             {projectaccess.ProjectEdit},
	"PUT " + envItem:                                                               {projectaccess.ProjectEdit},
	"DELETE " + envItem:                                                            {projectaccess.ProjectEdit},
	"PUT " + envItem + "/machine-role":                                             {projectaccess.ProjectEdit, projectaccess.DeviceConfigure},
	"GET " + envItem + "/rebuild-plan":                                             {projectaccess.ProjectView},
	"PUT " + envItem + "/rebuild-plan":                                             {projectaccess.ProjectEdit},
	"DELETE " + envItem + "/rebuild-plan":                                          {projectaccess.ProjectEdit},
	"POST " + envItem + "/rebuild-plan/seed":                                       {projectaccess.ProjectEdit},
	"GET " + envItem + "/snapshots":                                                {projectaccess.MediaRead},
	"GET " + envItem + "/snapshots/diff":                                           {projectaccess.MediaRead},
	"GET " + envItem + "/snapshots/{snapshot_id}":                                  {projectaccess.MediaRead},
	"GET " + envItem + "/assets/{asset_key}/vault-status":                          {projectaccess.MediaRead},
	"POST " + envItem + "/assets/{asset_key}/capture":                              {projectaccess.ProjectEdit, projectaccess.MediaUpload},
	"POST " + envItem + "/operations":                                              {projectaccess.RehearsalControl, projectaccess.MediaRead},
	"POST " + envItem + "/oscquery/discover":                                       {projectaccess.ProjectEdit, projectaccess.DeviceConfigure},
	"GET " + projectRoute + "/runtime":                                             {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/start":                                      {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/go":                                         {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/jump":                                       {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/stop":                                       {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/stop-session":                               {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/emergency-blackout":                         {projectaccess.ProjectView},
	"POST " + projectRoute + "/runtime/project-blackout":                           {projectaccess.ProjectView},
	"DELETE " + projectRoute + "/inputs/{input_id}":                                {projectaccess.ProjectEdit},
	"DELETE " + projectRoute + "/routes/{route_id}":                                {projectaccess.ProjectEdit},
	"DELETE " + projectRoute + "/targets/{alias_id}":                               {projectaccess.ProjectEdit},
	"GET " + projectRoute + "/configuration":                                       {projectaccess.ProjectView},
	"POST " + projectRoute + "/inputs":                                             {projectaccess.ProjectEdit},
	"POST " + projectRoute + "/outputs":                                            {projectaccess.ProjectEdit},
	"POST " + projectRoute + "/routes":                                             {projectaccess.ProjectEdit},
	"POST " + projectRoute + "/targets":                                            {projectaccess.ProjectEdit},
	"PUT " + projectRoute + "/inputs/{input_id}":                                   {projectaccess.ProjectEdit},
	"PUT " + projectRoute + "/outputs/{output_id}":                                 {projectaccess.ProjectEdit},
	"PUT " + projectRoute + "/routes/{route_id}":                                   {projectaccess.ProjectEdit},
	"PUT " + projectRoute + "/targets/{alias_id}/configuration":                    {projectaccess.ProjectEdit},
	"DELETE " + projectRoute + "/lighting-controller/configuration/{device_id}":    {projectaccess.DeviceConfigure},
	"GET " + projectRoute + "/lighting-controller/configuration":                   {projectaccess.ProjectView},
	"PUT " + projectRoute + "/lighting-controller/configuration/{device_id}":       {projectaccess.DeviceConfigure},
	"DELETE " + projectRoute + "/machine-roles/{machine_role_id}":                  {projectaccess.DeviceConfigure},
	"DELETE " + projectRoute + "/machine-roles/{machine_role_id}/assignment":       {projectaccess.DeviceConfigure},
	"GET " + projectRoute + "/machine-roles":                                       {projectaccess.ProjectView},
	"POST " + projectRoute + "/machine-roles":                                      {projectaccess.DeviceConfigure},
	"POST " + projectRoute + "/machine-roles/{machine_role_id}/assignment":         {projectaccess.DeviceConfigure},
	"POST " + projectRoute + "/machine-roles/{machine_role_id}/restore":            {projectaccess.DeviceConfigure},
	"POST " + projectRoute + "/machine-roles/{machine_role_id}/retire":             {projectaccess.DeviceConfigure},
	"PUT " + projectRoute + "/machine-roles/{machine_role_id}":                     {projectaccess.DeviceConfigure},
	"PUT " + projectRoute + "/machine-roles/{machine_role_id}/runtime-requirement": {projectaccess.DeviceConfigure},
	"POST " + projectRoute + "/stage-devices/{device_id}/stagelaser-visual-check":  {projectaccess.DeviceConfigure},
	"PUT " + projectRoute + "/live-video-sources/{source_id}":                      {projectaccess.DeviceConfigure},
	"PUT " + projectRoute + "/stage-devices/{device_id}/show-requirement":          {projectaccess.DeviceConfigure},
	"GET " + projectRoute + "/timing-intelligence":                                 {projectaccess.ProjectView},
	"PUT " + projectRoute + "/timing-intelligence/sessions/{session_id}":           {projectaccess.ProjectEdit},
}

// Raw device actuation, provisioning and assignment transfers are Owner-only
// in qualification. A project configuration grant alone cannot send output.
var ownerDeviceQualification = map[string]bool{
	"POST " + projectRoute + "/stage-device-commands":                           true,
	"POST " + projectRoute + "/stage-devices/sync-runtime-snapshot":             true,
	"POST " + projectRoute + "/stage-devices/{device_id}/lighting-activation":   true,
	"POST " + projectRoute + "/stage-devices/{device_id}/migrate-v2-lighting":   true,
	"POST " + projectRoute + "/stage-devices/{device_id}/stagelaser-assignment": true,
	"POST " + projectRoute + "/stage-devices/{device_id}/stagelaser-resync":     true,
	"POST /api/v1/stage-devices/{device_id}/assignment/preflight":               true,
	"POST /api/v1/stage-devices/{device_id}/assignment/software-transfer":       true,
	"POST /api/v1/stage-devices/{device_id}/commands":                           true,
	"POST /api/v1/stage-devices/{device_id}/decommission":                       true,
	"POST /api/v1/stage-devices/{device_id}/firmware-artifacts":                 true,
	"POST /api/v1/stage-devices/{device_id}/firmware-updates":                   true,
	"POST /api/v1/stage-devices/{device_id}/firmware-updates/{update_id}/send":  true,
	"POST /api/v1/stage-devices/{device_id}/setup-ap-password":                  true,
}

func projectRouteIsQualified(pattern string) bool {
	return len(projectRoutePermissions[pattern]) != 0 || ownerDeviceQualification[pattern]
}

func qualifyProjectRoute(w http.ResponseWriter, r *http.Request, auth *userauth.Service, session userauth.Session, token string) (*http.Request, bool) {
	access, _ := r.Context().Value(projectQualificationKey{}).(*projectaccess.Service)
	needed := projectRoutePermissions[r.Pattern]
	if access == nil {
		return r, true
	}
	if ownerDeviceQualification[r.Pattern] {
		return r, allowScopedOwner(w, session, access)
	}
	if len(needed) == 0 {
		return r, true
	}
	if !allowScopedProject(w, r, session, access, needed...) {
		return r, false
	}
	grant := projectRouteGrant{access, auth, token, session, needed}
	return r.WithContext(context.WithValue(r.Context(), projectRouteGrantKey{}, grant)), true
}

func currentProjectRouteGrant(r *http.Request) (projectRouteGrant, error) {
	grant, _ := r.Context().Value(projectRouteGrantKey{}).(projectRouteGrant)
	if grant.access == nil {
		return grant, nil
	}
	current, err := grant.auth.ValidatePassive(r.Context(), grant.token)
	if err != nil || current.ID != grant.user.ID || current.User.ID != grant.user.User.ID {
		return grant, projectaccess.ErrDenied
	}
	return grant, grant.access.CheckAll(r.Context(), current.User.ID, strings.TrimSpace(r.PathValue("project_id")), grant.needed...)
}

func writeProjectQualificationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
	} else {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_UNAVAILABLE"})
	}
	return false
}

func qualifyRuntimeIntent(w http.ResponseWriter, r *http.Request, sessionID string, intent projectaccess.RuntimeIntent) bool {
	grant, err := currentProjectRouteGrant(r)
	if err == nil && grant.access != nil {
		err = grant.access.CheckRuntimeIntent(r.Context(), grant.user.User.ID, r.PathValue("project_id"), sessionID, intent)
	}
	return writeProjectQualificationError(w, err)
}
