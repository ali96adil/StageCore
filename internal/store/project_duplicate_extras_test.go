package store_test

import (
    "context"
    "database/sql"
    "strings"
    "testing"

    "github.com/ali96adil/StageCore/internal/store"
    "github.com/ali96adil/StageCore/internal/domain"
)

// This exercises persistent cross-domain authoring data, not just Cue names.
func TestDuplicateProjectAuthoringProjectWideSettingsAndMedia(t *testing.T) {
    ctx:=context.Background()
    s,h:=newStore(t)
    src,rev,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"Source Production",Description:"Opening night",CreatedBy:"operator"})
    if err!=nil {t.Fatal(err)}
    cue,err:=s.CreateCueWithActions(ctx, makeTestDuplicateCue(rev.ID), nil)
    if err!=nil {t.Fatal(err)}
    exec:=func(query string,args ...any){
        t.Helper()
        if _,err:=h.DB.ExecContext(ctx,query,args...);err!=nil{t.Fatalf("SQL %s: %v",query,err)}
    }
    roleID:="00000000-0000-7000-8000-000000000301"
    liveSourceID:="00000000-0000-7000-8000-000000000306"
    assetID:="00000000-0000-7000-8000-000000000302"
    versionID:="00000000-0000-7000-8000-000000000303"
    mediaHash:=strings.Repeat("a",64)
    exec(`INSERT INTO machine_roles
        (machine_role_id,project_id,role_key,display_name,required_capabilities_json,
         required_runtime_snapshot_id,required_config_hash,required,created_at_us,updated_at_us)
         VALUES (?,?,'VIDEO-VDMX','VDMX','["osc.send"]',NULL,'',1,1,1)`,roleID,src.ID)
    exec(`INSERT INTO vault_objects (content_hash,size_bytes,relative_path,created_at_us) VALUES (?,4,'vault/a',1)`,mediaHash)
    exec(`INSERT INTO media_assets (media_asset_id,project_id,name,asset_policy,created_at_us,updated_at_us)
         VALUES (?,?,'Ambient Video','MANAGED',1,1)`,assetID,src.ID)
    exec(`INSERT INTO media_content_versions
         (content_version_id,media_asset_id,content_hash,original_filename,size_bytes,created_at_us)
         VALUES (?,?,?,'ambient.mp4',4,1)`,versionID,assetID,mediaHash)
    exec(`INSERT INTO media_locations
        (media_location_id,content_version_id,location_type,locator,status)
        VALUES ('00000000-0000-7000-8000-000000000304',?,'HUB','vault/a','AVAILABLE')`,versionID)
    actionParams := `{"media_asset_id":"`+assetID+`","nested":{"content_version_id":"`+versionID+`","live_source_id":"`+liveSourceID+`","machine_role_id":"`+roleID+`"},"filename":"ambient.mp4"}`
    exec(`INSERT INTO actions
        (action_id,cue_id,order_index,execution_mode,target_ref,capability_key,
         parameters_json,timeout_policy_json,error_policy_json,priority_class,enabled)
         VALUES ('00000000-0000-7000-8000-000000000309',?,0,'SEQUENTIAL','VIDEO-VDMX',
                 'video.source.open',?,'{}','{}','P1',1)`,cue.ID,actionParams)
    exec(`INSERT INTO machine_role_media_requirements
        (media_requirement_id,machine_role_id,content_version_id,required,created_at_us)
        VALUES ('00000000-0000-7000-8000-000000000305',?,?,1,1)`,roleID,versionID)
    exec(`INSERT INTO live_video_sources
        (source_id,project_id,name,source_class,execution_machine_role_id,endpoint_ref,created_at_us,updated_at_us)
        VALUES (? ,?,'Rear Camera','NETWORK_STREAM',?,'stagecam.local',1,1)`,liveSourceID,src.ID,roleID)
    exec(`INSERT INTO operator_notes
        (note_id,project_id,cue_id,category,body,status,created_by,created_at_us,updated_at_us)
        VALUES ('00000000-0000-7000-8000-000000000307',?,?,'DIRECTION','Hold entrance','OPEN','operator',1,1)`,src.ID,cue.ID)
    exec(`INSERT INTO visual_engine_revision_settings (revision_id,engine_mode,updated_by,updated_at_us)
        VALUES (?,'NATIVE','operator',1)`,rev.ID)
    exec(`INSERT INTO stage_devices
        (device_id,project_id,device_kind,display_name,created_at_us,updated_at_us)
        VALUES ('00000000-0000-7000-8000-000000000308',?,'GENERIC','DMX Stage Device',1,1)`,src.ID)
    exec(`INSERT INTO lighting_node_revision_bindings
        (revision_id,device_id,profile_id,configuration_json,aliases_json,updated_by,updated_at_us)
        VALUES (?,'00000000-0000-7000-8000-000000000308','lighting','{}','{}','operator',1)`,rev.ID)

    duplicate,draft,err:=s.DuplicateProjectAuthoring(ctx,src.ID,"Copy Production","operator")
    if err!=nil{t.Fatal(err)}
    if duplicate.ID==src.ID || draft.ID==rev.ID {t.Fatal("IDs reused")}

    var role string
    if err:=h.DB.QueryRowContext(ctx,`SELECT machine_role_id FROM machine_roles WHERE project_id=? AND role_key='VIDEO-VDMX'`,duplicate.ID).Scan(&role);err!=nil{t.Fatal(err)}
    if role==roleID{t.Fatal("Machine Role ID reused")}
    var newAsset,newVersion string
    if err:=h.DB.QueryRowContext(ctx,`SELECT media_asset_id FROM media_assets WHERE project_id=?`,duplicate.ID).Scan(&newAsset);err!=nil{t.Fatal(err)}
    if newAsset==assetID{t.Fatal("media asset ID reused")}
    if err:=h.DB.QueryRowContext(ctx,`SELECT content_version_id FROM media_content_versions WHERE media_asset_id=?`,newAsset).Scan(&newVersion);err!=nil{t.Fatal(err)}
    if newVersion==versionID{t.Fatal("content version ID reused")}
    copiedCues,err:=s.ListCues(ctx,draft.ID)
    if err!=nil{t.Fatal(err)}
    if len(copiedCues)!=1 || len(copiedCues[0].Actions)!=1 {
        t.Fatalf("expected cloned media Cue action, got %+v",copiedCues)
    }
    copiedParams:=string(copiedCues[0].Actions[0].Parameters)
    if !strings.Contains(copiedParams,newAsset) || !strings.Contains(copiedParams,newVersion) ||
       strings.Contains(copiedParams,assetID) || strings.Contains(copiedParams,versionID) ||
       !strings.Contains(copiedParams,"ambient.mp4") {
        t.Fatalf("media IDs in cloned Cue action were not remapped: %s",copiedParams)
    }
    var matchingRequirements int
    if err:=h.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM machine_role_media_requirements
        WHERE machine_role_id=? AND content_version_id=?`,role,newVersion).Scan(&matchingRequirements);err!=nil{t.Fatal(err)}
    if matchingRequirements!=1{t.Fatal("role media binding lost")}
    var cameraRole, cameraSourceID string
    var executionDevice sql.NullString
    if err:=h.DB.QueryRowContext(ctx,`SELECT source_id,execution_machine_role_id,execution_device_id FROM live_video_sources WHERE project_id=?`,
        duplicate.ID).Scan(&cameraSourceID,&cameraRole,&executionDevice);err!=nil{t.Fatal(err)}
    if cameraRole!=role || executionDevice.Valid || cameraSourceID==liveSourceID {
        t.Fatal("video source not remapped or hardware authority inherited")
    }
    if !strings.Contains(copiedParams,cameraSourceID) || !strings.Contains(copiedParams,role) ||
       strings.Contains(copiedParams,liveSourceID) || strings.Contains(copiedParams,roleID) {
        t.Fatalf("copied Cue action still references original live source/Machine Role IDs: %s",copiedParams)
    }
    var noteCue,body string
    if err:=h.DB.QueryRowContext(ctx,`SELECT cue_id,body FROM operator_notes WHERE project_id=?`,duplicate.ID).Scan(&noteCue,&body);err!=nil{t.Fatal(err)}
    if noteCue==cue.ID || body!="Hold entrance"{t.Fatal("operator note not cloned correctly")}
    var mode string
    if err:=h.DB.QueryRowContext(ctx,`SELECT engine_mode FROM visual_engine_revision_settings WHERE revision_id=?`,draft.ID).Scan(&mode);err!=nil{t.Fatal(err)}
    if mode!="NATIVE"{t.Fatal("visual mode lost")}
    var lightingCount int
    if err:=h.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM lighting_node_revision_bindings WHERE revision_id=?`,draft.ID).Scan(&lightingCount);err!=nil{t.Fatal(err)}
    if lightingCount!=1{t.Fatal("DMX authoring lost")}
    var assigned string
    if err:=h.DB.QueryRowContext(ctx,`SELECT project_id FROM stage_devices WHERE device_id='00000000-0000-7000-8000-000000000308'`).Scan(&assigned);err!=nil{t.Fatal(err)}
    if assigned!=src.ID{t.Fatal("stage device ownership moved")}
}

func makeTestDuplicateCue(revisionID string) domain.Cue {
    return domain.Cue{RevisionID:revisionID,Name:"Scene A",DisplayLabel:"A",OrderIndex:1,Enabled:true,NotesSummary:"Wait for lights"}
}

func TestDuplicateProjectPreservesSharedExternalMediaRequirement(t *testing.T) {
    ctx:=context.Background()
    s,h:=newStore(t)
    library,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"Shared Media Library",CreatedBy:"operator"})
    if err!=nil{t.Fatal(err)}
    source,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"Source Show",CreatedBy:"operator"})
    if err!=nil{t.Fatal(err)}
    exec:=func(q string,args ...any){t.Helper();if _,err:=h.DB.ExecContext(ctx,q,args...);err!=nil{t.Fatal(err)}}
    hash:=strings.Repeat("b",64)
    asset:="00000000-0000-7000-8000-000000000411"
    version:="00000000-0000-7000-8000-000000000412"
    role:="00000000-0000-7000-8000-000000000413"
    exec(`INSERT INTO vault_objects(content_hash,size_bytes,relative_path,created_at_us)
        VALUES (?,3,'vault/shared',1)`,hash)
    exec(`INSERT INTO media_assets(media_asset_id,project_id,name,asset_policy,created_at_us,updated_at_us)
        VALUES (?,?,'Shared','MANAGED',1,1)`,asset,library.ID)
    exec(`INSERT INTO media_content_versions(content_version_id,media_asset_id,content_hash,original_filename,size_bytes,created_at_us)
        VALUES (?,?,?,'shared.mp4',3,1)`,version,asset,hash)
    exec(`INSERT INTO machine_roles(machine_role_id,project_id,role_key,display_name,required_capabilities_json,
         required_runtime_snapshot_id,required_config_hash,required,created_at_us,updated_at_us)
         VALUES (?,?,'VIDEO-SHARED','Shared video','[]',NULL,'',1,1,1)`,role,source.ID)
    exec(`INSERT INTO machine_role_media_requirements(media_requirement_id,machine_role_id,content_version_id,required,created_at_us)
         VALUES ('00000000-0000-7000-8000-000000000414',?,?,1,1)`,role,version)
    clone,_,err:=s.DuplicateProjectAuthoring(ctx,source.ID,"Copy of shared show","operator")
    if err!=nil{t.Fatal(err)}
    var newRole,newVersion string
    err=h.DB.QueryRowContext(ctx,`SELECT machine_role_id FROM machine_roles WHERE project_id=?`,clone.ID).Scan(&newRole)
    if err!=nil{t.Fatal(err)}
    err=h.DB.QueryRowContext(ctx,`SELECT content_version_id FROM machine_role_media_requirements
        WHERE machine_role_id=?`,newRole).Scan(&newVersion)
    if err!=nil{t.Fatal(err)}
    if newRole==role || newVersion!=version{t.Fatalf("external media reference corrupted: role=%s version=%s",newRole,newVersion)}
    var copies int
    err=h.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM media_assets WHERE project_id=?`,clone.ID).Scan(&copies)
    if err!=nil{t.Fatal(err)}
    if copies!=0{t.Fatalf("unexpected imported external media assets: %d",copies)}
}
