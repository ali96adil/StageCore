package showcapsule

import (
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/executionenv"
)

func validateObjectReferences(manifest Manifest) error {
	objects := make(map[string]ObjectEntry, len(manifest.Objects))
	for _, object := range manifest.Objects {
		objects[strings.ToLower(object.ContentSHA256)] = object
	}
	referenced := make(map[string]bool, len(objects))
	require := func(hash string, size int64, label string) error {
		hash = strings.ToLower(strings.TrimSpace(hash))
		object, ok := objects[hash]
		if !ok {
			return fmt.Errorf("show capsule dependency %s requires object %s but no ObjectEntry exists", label, hash)
		}
		if object.SizeBytes != size {
			return fmt.Errorf("show capsule dependency %s object %s size mismatch: manifest=%d dependency=%d", label, hash, object.SizeBytes, size)
		}
		referenced[hash] = true
		return nil
	}

	runtimeMedia := make(map[string]struct{}, len(manifest.RuntimeSnapshot.Manifest.RequiredMedia))
	for _, required := range manifest.RuntimeSnapshot.Manifest.RequiredMedia {
		key := mediaIdentity(required.MachineRoleID, required.MediaAssetID, required.ContentVersionID)
		runtimeMedia[key] = struct{}{}
		found := false
		for _, media := range manifest.Media {
			if mediaIdentity(media.MachineRoleID, media.MediaAssetID, media.ContentVersionID) != key {
				continue
			}
			found = true
			if !strings.EqualFold(media.ContentSHA256, required.ContentHash) || media.SizeBytes != required.SizeBytes || media.Required != required.Required || media.RoleKey != required.RoleKey {
				return fmt.Errorf("show capsule media %s does not match immutable Runtime Snapshot requirement", key)
			}
			break
		}
		if !found {
			return fmt.Errorf("show capsule omits immutable Runtime Snapshot media requirement %s", key)
		}
	}
	for _, media := range manifest.Media {
		key := mediaIdentity(media.MachineRoleID, media.MediaAssetID, media.ContentVersionID)
		if _, ok := runtimeMedia[key]; !ok {
			return fmt.Errorf("show capsule contains media %s not declared by immutable Runtime Snapshot", key)
		}
		if !strings.EqualFold(media.AssetPolicy, "REFERENCE_ONLY") {
			if err := require(media.ContentSHA256, media.SizeBytes, "media:"+key); err != nil {
				return err
			}
		}
	}

	for _, environment := range manifest.ExecutionEnvironments {
		key := environment.Manifest.EnvironmentKey
		for _, asset := range environment.Manifest.Assets {
			if asset.CapturePolicy != executionenv.CaptureContentBound {
				continue
			}
			if asset.SizeBytes == nil {
				return fmt.Errorf("execution environment %s asset %s has no content size", key, asset.Key)
			}
			if err := require(asset.ContentHash, *asset.SizeBytes, "environment:"+key+":asset:"+asset.Key); err != nil {
				return err
			}
		}
		if environment.LatestSnapshot == nil {
			continue
		}
		for _, item := range environment.LatestSnapshot.Snapshot.Items {
			if item.Portability != executionenv.SnapshotContentBound {
				continue
			}
			if item.SizeBytes == nil {
				return fmt.Errorf("execution environment %s snapshot item %s has no content size", key, item.Key)
			}
			if err := require(item.ContentHash, *item.SizeBytes, "environment:"+key+":snapshot:"+item.Key); err != nil {
				return err
			}
		}
	}

	for _, extension := range manifest.Extensions {
		if err := require(extension.Software.ContentSHA256, extension.Software.SizeBytes, "extension:"+extension.ExtensionID+"@"+extension.Version); err != nil {
			return err
		}
	}

	for hash := range objects {
		if !referenced[hash] {
			return fmt.Errorf("show capsule object %s is not referenced by media, execution-environment or extension requirements", hash)
		}
	}
	return nil
}

func mediaIdentity(machineRoleID, mediaAssetID, contentVersionID string) string {
	return strings.TrimSpace(machineRoleID) + ":" + strings.TrimSpace(mediaAssetID) + ":" + strings.TrimSpace(contentVersionID)
}
