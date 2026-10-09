package store

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
)

// remapCopiedProjectMediaJSONTx updates exact media-asset/content-version IDs
// inside freshly copied authoring JSON. It never touches original Project rows,
// never rewrites arbitrary substrings (including OSC addresses or file names),
// and does not alter content-addressed vault hashes.
func remapCopiedProjectMediaJSONTx(ctx context.Context, tx *sql.Tx, projectID, revisionID string, mediaIDs map[string]string) error {
    if len(mediaIDs) == 0 { return nil }
    type field struct {
        label string
        selectSQL string
        updateSQL string
        scopeID string
    }
    fields := []field{
        {"Cue policy",
            "SELECT cue_id,execution_policy_json FROM cues WHERE revision_id = ?",
            "UPDATE cues SET execution_policy_json=? WHERE cue_id=?",revisionID},
        {"Cue action parameters",
            "SELECT a.action_id,a.parameters_json FROM actions a JOIN cues c ON c.cue_id=a.cue_id WHERE c.revision_id=?",
            "UPDATE actions SET parameters_json=? WHERE action_id=?",revisionID},
        {"Input value schema",
            "SELECT input_id,value_schema_json FROM input_definitions WHERE revision_id=?",
            "UPDATE input_definitions SET value_schema_json=? WHERE input_id=?",revisionID},
        {"Output value schema",
            "SELECT output_id,value_schema_json FROM output_definitions WHERE revision_id=?",
            "UPDATE output_definitions SET value_schema_json=? WHERE output_id=?",revisionID},
        {"Route condition",
            "SELECT route_id,condition_definition_json FROM routes WHERE revision_id=?",
            "UPDATE routes SET condition_definition_json=? WHERE route_id=?",revisionID},
        {"Route transform",
            "SELECT route_id,transform_definition_json FROM routes WHERE revision_id=?",
            "UPDATE routes SET transform_definition_json=? WHERE route_id=?",revisionID},
        {"Route action parameters",
            "SELECT a.route_action_id,a.parameters_json FROM route_actions a JOIN routes r ON r.route_id=a.route_id WHERE r.revision_id=?",
            "UPDATE route_actions SET parameters_json=? WHERE route_action_id=?",revisionID},
        {"Project alias configuration",
            "SELECT alias_id,project_config_json FROM project_device_aliases WHERE project_id=?",
            "UPDATE project_device_aliases SET project_config_json=? WHERE alias_id=?",projectID},
        {"Live video configuration",
            "SELECT source_id,config_json FROM live_video_sources WHERE project_id=?",
            "UPDATE live_video_sources SET config_json=? WHERE source_id=?",projectID},
    }
    for _, field := range fields {
        rows, err := tx.QueryContext(ctx,field.selectSQL,field.scopeID)
        if err != nil {return fmt.Errorf("read %s: %w",field.label,err)}
        type update struct{id, jsonText string}
        var updates []update
        for rows.Next(){
            var row update
            if err:=rows.Scan(&row.id,&row.jsonText);err!=nil{rows.Close();return fmt.Errorf("scan %s: %w",field.label,err)}
            rewritten,changed,err:=remapMediaJSONReferences(row.jsonText,mediaIDs)
            if err!=nil{rows.Close();return fmt.Errorf("%s: %w",field.label,err)}
            if changed{updates=append(updates,update{id:row.id,jsonText:rewritten})}
        }
        if err:=rows.Err();err!=nil{rows.Close();return err}
        rows.Close()
        for _,row:=range updates {
            if _,err:=tx.ExecContext(ctx,field.updateSQL,row.jsonText,row.id);err!=nil{return fmt.Errorf("rewrite %s: %w",field.label,err)}
        }
    }
    return nil
}

func remapMediaJSONReferences(raw string, mediaIDs map[string]string) (string,bool,error) {
    if len(mediaIDs)==0{return raw,false,nil}
    var value any
    if err:=json.Unmarshal([]byte(raw),&value);err!=nil{return "",false,err}
    rewritten,changed:=remapMediaJSONValue(value,mediaIDs)
    if !changed{return raw,false,nil}
    data,err:=json.Marshal(rewritten)
    if err!=nil{return "",false,err}
    return string(data),true,nil
}

func remapMediaJSONValue(value any,mediaIDs map[string]string)(any,bool){
    switch v:=value.(type){
    case string:
        mapped,ok:=mediaIDs[v]
        if ok {return mapped,true}
        return v,false
    case []any:
        changed:=false
        for i:=range v {
            next,replaced:=remapMediaJSONValue(v[i],mediaIDs)
            v[i]=next
            changed=changed||replaced
        }
        return v,changed
    case map[string]any:
        changed:=false
        for key,element:=range v {
            next,replaced:=remapMediaJSONValue(element,mediaIDs)
            v[key]=next
            changed=changed||replaced
        }
        return v,changed
    default:
        return value,false
    }
}
