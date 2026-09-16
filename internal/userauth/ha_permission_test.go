package userauth

import "testing"

func TestHAManagePermissionIsOwnerOnly(t *testing.T) {
	if err := Authorize(RoleOwner, PermissionHAManage); err != nil {
		t.Fatalf("OWNER HA management authorization error = %v", err)
	}
	for _, role := range []string{RoleTechnician, RoleOperator, RoleViewer} {
		if err := Authorize(role, PermissionHAManage); err == nil {
			t.Fatalf("role %s unexpectedly has HA management permission", role)
		}
	}
}
