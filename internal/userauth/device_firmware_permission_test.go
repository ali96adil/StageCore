package userauth

import "testing"

func TestDeviceFirmwareManagePermission(t *testing.T) {
	for _, role := range []string{RoleOwner, RoleTechnician} {
		if err := Authorize(role, PermissionDeviceFirmwareManage); err != nil {
			t.Fatalf("%s should manage device firmware: %v", role, err)
		}
	}
	for _, role := range []string{RoleOperator, RoleViewer} {
		if err := Authorize(role, PermissionDeviceFirmwareManage); err == nil {
			t.Fatalf("%s unexpectedly allowed to manage device firmware", role)
		}
	}
}
