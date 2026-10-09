package store

import (
    "context"
    "database/sql"
    "fmt"

    stageid "github.com/ali96adil/StageCore/internal/id"
    "github.com/ali96adil/StageCore/internal/domain"
)

// duplicateMachineRolesTx copies authoring role definitions, not Companion
// assignments, runtime leases or a previously published Snapshot requirement.
func duplicateMachineRolesTx(ctx context.Context, tx *sql.Tx, sourceProjectID, newProjectID string, nowUS int64) (map[string]string, error) {
    rows, err := tx.QueryContext(ctx, `SELECT machine_role_id, role_key, display_name, required_capabilities_json,
        required, retired FROM machine_roles WHERE project_id = ? ORDER BY role_key`, sourceProjectID)
    if err != nil { return nil, err }
    type role struct { old, key, name, caps string; required, retired int }
    var records []role
    for rows.Next() {
        var item role
        if err := rows.Scan(&item.old, &item.key, &item.name, &item.caps, &item.required, &item.retired); err != nil {
            rows.Close(); return nil, err
        }
        records = append(records, item)
    }
    if err := rows.Err(); err != nil { rows.Close(); return nil, err }
    rows.Close()
    mapped := make(map[string]string, len(records))
    for _, item := range records {
        id, err := stageid.New()
        if err != nil { return nil, err }
        _, err = tx.ExecContext(ctx, `INSERT INTO machine_roles
            (machine_role_id,project_id,role_key,display_name,required_capabilities_json,
             required_runtime_snapshot_id,required_config_hash,required,retired,created_at_us,updated_at_us)
            VALUES (?,?,?,?,?,NULL,'',?,?,?,?)`, id, newProjectID, item.key, item.name,
            item.caps, item.required, item.retired, nowUS, nowUS)
        if err != nil { return nil, fmt.Errorf("copy Machine Role %s: %w", item.key, err) }
        mapped[item.old] = id
    }
    return mapped, nil
}

// duplicateProjectExtrasTx copies Project-level *authoring* configuration.
// All source reads are inside the same transaction as the destination inserts.
// Never copy Session identities, device authority, commands or observations.
func duplicateProjectExtrasTx(ctx context.Context, tx *sql.Tx,
    sourceProjectID, newProjectID, sourceRevisionID, newRevisionID, actor string,
    nowUS int64, cueIDs, roleIDs map[string]string) error {

    if _, err := tx.ExecContext(ctx, `INSERT INTO visual_engine_revision_settings
        (revision_id,engine_mode,updated_by,updated_at_us)
        SELECT ?,engine_mode,?,? FROM visual_engine_revision_settings WHERE revision_id = ?`,
        newRevisionID, actor, nowUS, sourceRevisionID); err != nil {
        return fmt.Errorf("duplicate Visual Engine settings: %w", err)
    }
    if _, err := tx.ExecContext(ctx, `INSERT INTO lighting_node_revision_bindings
        (revision_id,device_id,profile_id,configuration_json,aliases_json,updated_by,updated_at_us)
        SELECT ?,device_id,profile_id,configuration_json,aliases_json,?,?
        FROM lighting_node_revision_bindings WHERE revision_id = ?`,
        newRevisionID, actor, nowUS, sourceRevisionID); err != nil {
        return fmt.Errorf("duplicate DMX authoring configuration: %w", err)
    }

    type note struct {
        oldCue sql.NullString
        category, body, status, createdBy string
        resolvedAt sql.NullInt64
    }
    noteRows, err := tx.QueryContext(ctx, `SELECT cue_id,category,body,status,created_by,resolved_at_us
        FROM operator_notes WHERE project_id = ? ORDER BY created_at_us,note_id`, sourceProjectID)
    if err != nil { return err }
    var notes []note
    for noteRows.Next() {
        var n note
        if err := noteRows.Scan(&n.oldCue,&n.category,&n.body,&n.status,&n.createdBy,&n.resolvedAt); err != nil {
            noteRows.Close();return err
        }
        notes = append(notes,n)
    }
    if err := noteRows.Err(); err != nil { noteRows.Close();return err }
    noteRows.Close()
    for _, n := range notes {
        id, err := stageid.New()
        if err != nil { return err }
        var cueID any
        if n.oldCue.Valid {
            newCue, ok := cueIDs[n.oldCue.String]
            if !ok {
                // A Project note may point to a Cue in a historic revision.
                // Preserve the body, but never retain a source Project Cue FK.
                cueID = nil
            } else { cueID = newCue }
        }
        _,err = tx.ExecContext(ctx, `INSERT INTO operator_notes
          (note_id,project_id,session_id,cue_id,category,body,status,created_by,
           created_at_us,updated_at_us,resolved_at_us)
           VALUES (?,?,NULL,?,?,?,?,?,?,?,?)`,
          id,newProjectID,cueID,n.category,n.body,n.status,n.createdBy,nowUS,nowUS,
          func() any { if n.status == "RESOLVED" { return nowUS }; return nil }())
        if err != nil { return fmt.Errorf("duplicate operator note: %w",err) }
    }

    type asset struct { old,name,policy string }
    assetRows,err := tx.QueryContext(ctx,`SELECT media_asset_id,name,asset_policy FROM media_assets WHERE project_id = ?`,sourceProjectID)
    if err!=nil {return err}
    var assets []asset
    for assetRows.Next(){var a asset;if err:=assetRows.Scan(&a.old,&a.name,&a.policy);err!=nil{assetRows.Close();return err};assets=append(assets,a)}
    if err:=assetRows.Err();err!=nil{assetRows.Close();return err};assetRows.Close()
    versionIDs:=map[string]string{}
    for _,a:=range assets{
        id,err:=stageid.New();if err!=nil{return err}
        if _,err=tx.ExecContext(ctx,`INSERT INTO media_assets
            (media_asset_id,project_id,name,asset_policy,created_at_us,updated_at_us)
            VALUES (?,?,?,?,?,?)`,id,newProjectID,a.name,a.policy,nowUS,nowUS);err!=nil{return err}
        type version struct {old,hash,filename string;size int64}
        rows,err:=tx.QueryContext(ctx,`SELECT content_version_id,content_hash,original_filename,size_bytes
            FROM media_content_versions WHERE media_asset_id=?`,a.old)
        if err!=nil{return err}
        var versions []version
        for rows.Next(){var v version;if err:=rows.Scan(&v.old,&v.hash,&v.filename,&v.size);err!=nil{rows.Close();return err};versions=append(versions,v)}
        if err:=rows.Err();err!=nil{rows.Close();return err};rows.Close()
        for _,v:=range versions{
            newVersion,err:=stageid.New();if err!=nil{return err}
            if _,err=tx.ExecContext(ctx,`INSERT INTO media_content_versions
                (content_version_id,media_asset_id,content_hash,original_filename,size_bytes,created_at_us)
                VALUES (?,?,?,?,?,?)`,newVersion,id,v.hash,v.filename,v.size,nowUS);err!=nil{return err}
            versionIDs[v.old]=newVersion
            type location struct {typ,locator,status string;verified sql.NullInt64}
            locations,err:=tx.QueryContext(ctx,`SELECT location_type,locator,status,verified_at_us FROM media_locations WHERE content_version_id=?`,v.old)
            if err!=nil{return err}
            var records []location
            for locations.Next(){var l location;if err:=locations.Scan(&l.typ,&l.locator,&l.status,&l.verified);err!=nil{locations.Close();return err};records=append(records,l)}
            if err:=locations.Err();err!=nil{locations.Close();return err};locations.Close()
            for _,l:=range records {
                locID,err:=stageid.New();if err!=nil{return err}
                if _,err=tx.ExecContext(ctx,`INSERT INTO media_locations
                    (media_location_id,content_version_id,location_type,locator,status,verified_at_us)
                    VALUES (?,?,?,?,?,?)`,locID,newVersion,l.typ,l.locator,l.status,l.verified);err!=nil{return err}
            }
        }
    }

    type requirement struct{ oldRole,oldVersion string;required int }
    reqRows,err:=tx.QueryContext(ctx,`SELECT mr.machine_role_id,mr.content_version_id,mr.required
        FROM machine_role_media_requirements mr JOIN machine_roles role ON role.machine_role_id=mr.machine_role_id
        WHERE role.project_id=?`,sourceProjectID)
    if err!=nil{return err}
    var requirements []requirement
    for reqRows.Next(){var r requirement;if err:=reqRows.Scan(&r.oldRole,&r.oldVersion,&r.required);err!=nil{reqRows.Close();return err};requirements=append(requirements,r)}
    if err:=reqRows.Err();err!=nil{reqRows.Close();return err};reqRows.Close()
    for _,r:=range requirements {
        role,ok:=roleIDs[r.oldRole];if !ok{return fmt.Errorf("%w: missing copied Machine Role",domain.ErrConflict)}
        version,ok:=versionIDs[r.oldVersion]
        if !ok {return fmt.Errorf("%w: machine-role media must belong to the copied Project",domain.ErrConflict)}
        id,err:=stageid.New();if err!=nil{return err}
        if _,err=tx.ExecContext(ctx,`INSERT INTO machine_role_media_requirements
            (media_requirement_id,machine_role_id,content_version_id,required,created_at_us)
            VALUES (?,?,?,?,?)`,id,role,version,r.required,nowUS);err!=nil{return err}
    }

    type video struct {
        name,sourceClass,endpoint,caps,config string
        deviceID,profileID,roleID sql.NullString
        required,desired int
    }
    sourceRows,err:=tx.QueryContext(ctx,`SELECT name,source_class,execution_device_id,profile_id,endpoint_ref,
        capabilities_json,config_json,required,desired_enabled,execution_machine_role_id
        FROM live_video_sources WHERE project_id=?`,sourceProjectID)
    if err!=nil{return err}
    var sources []video
    for sourceRows.Next() {
        var v video
        if err:=sourceRows.Scan(&v.name,&v.sourceClass,&v.deviceID,&v.profileID,&v.endpoint,
          &v.caps,&v.config,&v.required,&v.desired,&v.roleID);err!=nil{sourceRows.Close();return err}
        sources=append(sources,v)
    }
    if err:=sourceRows.Err();err!=nil{sourceRows.Close();return err};sourceRows.Close()
    for _,v:=range sources{
        id,err:=stageid.New();if err!=nil{return err}
        var roleID any
        if v.roleID.Valid {
            mapped,ok:=roleIDs[v.roleID.String]
            if !ok{return fmt.Errorf("%w: live source refers to unknown Machine Role",domain.ErrConflict)}
            roleID=mapped
        }
        // Hardware Stage Device assignment and observed readiness never clone.
        if _,err=tx.ExecContext(ctx,`INSERT INTO live_video_sources
          (source_id,project_id,name,source_class,execution_device_id,profile_id,
           endpoint_ref,capabilities_json,config_json,required,desired_enabled,
           readiness,last_observed_at_us,created_at_us,updated_at_us,execution_machine_role_id)
           VALUES (?,?,?,?,NULL,?,?,?,?,?,?,'UNKNOWN',NULL,?,?,?)`,
          id,newProjectID,v.name,v.sourceClass,v.profileID,v.endpoint,v.caps,
          v.config,v.required,v.desired,nowUS,nowUS,roleID);err!=nil{
            return fmt.Errorf("duplicate live source %s: %w",v.name,err)
        }
    }
    return nil
}

