package projectaccess

import (
	"fmt"
	"sort"
)

// AccessLevel is the small operator-facing vocabulary for Project grants.
// These levels expand to the existing fine-grained capabilities; they do not
// grant the right to administer memberships, which remains Owner-only.
type AccessLevel string

const (
	AccessLevelView    AccessLevel = "VIEW"
	AccessLevelEdit    AccessLevel = "EDIT"
	AccessLevelOperate AccessLevel = "OPERATE"
	AccessLevelManage  AccessLevel = "MANAGE"
	AccessLevelCustom  AccessLevel = "CUSTOM"
	AccessLevelNone    AccessLevel = "NONE"
)

var accessLevelPermissions = map[AccessLevel][]Permission{
	AccessLevelView: {
		ProjectView,
		MediaRead,
	},
	AccessLevelEdit: {
		ProjectView,
		MediaRead,
		ProjectEdit,
		CueEdit,
		MediaUpload,
	},
	AccessLevelOperate: {
		ProjectView,
		MediaRead,
		RehearsalControl,
		ShowGO,
		ShowStop,
		ShowBlackoutEngage,
		ShowEnterExit,
		LightingManual,
		AudioControl,
	},
	AccessLevelManage: {
		ProjectView,
		MediaRead,
		ProjectRename,
		ProjectEdit,
		CueEdit,
		MediaUpload,
		MediaManage,
		SnapshotPublish,
		RehearsalControl,
		ShowGO,
		ShowStop,
		ShowBlackoutEngage,
		ShowEnterExit,
		DeviceConfigure,
		LightingManual,
		AudioControl,
		CapsuleExport,
	},
}

// PermissionsForAccessLevel returns a fresh capability slice. NONE revokes
// the Project membership. CUSTOM is an output-only marker and cannot be
// expanded into grants.
func PermissionsForAccessLevel(level AccessLevel) ([]Permission, error) {
	if level == AccessLevelNone {
		return []Permission{}, nil
	}
	permissions, ok := accessLevelPermissions[level]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported access level", ErrInvalid)
	}
	return append([]Permission(nil), permissions...), nil
}

// AccessLevelForPermissions reports the matching preset, NONE for an empty
// grant, or CUSTOM when the Owner chose a different fine-grained combination.
func AccessLevelForPermissions(permissions []Permission) AccessLevel {
	if len(permissions) == 0 {
		return AccessLevelNone
	}
	actual := append([]Permission(nil), permissions...)
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	for level, expectedPermissions := range accessLevelPermissions {
		expected := append([]Permission(nil), expectedPermissions...)
		sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
		if len(actual) != len(expected) {
			continue
		}
		matches := true
		for i := range actual {
			if actual[i] != expected[i] {
				matches = false
				break
			}
		}
		if matches {
			return level
		}
	}
	return AccessLevelCustom
}
