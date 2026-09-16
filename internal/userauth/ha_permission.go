package userauth

// PermissionHAManage controls changes to physical-output authority. It is kept
// separate from runtime.control because an OPERATOR may run an already-authorized
// show but must not promote or demote the Hub itself.
const PermissionHAManage Permission = "ha.manage"

func init() {
	rolePermissions[RoleOwner][PermissionHAManage] = true
}
