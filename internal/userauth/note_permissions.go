package userauth

const PermissionNoteWrite Permission = "note.write"

func init() {
	rolePermissions[RoleOwner][PermissionNoteWrite] = true
	rolePermissions[RoleTechnician][PermissionNoteWrite] = true
	rolePermissions[RoleOperator][PermissionNoteWrite] = true
}
