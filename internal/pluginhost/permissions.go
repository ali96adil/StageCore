package pluginhost

// SetGrantedPermissions atomically replaces the runtime permission grant set.
// Existing plugin configuration is preserved; subsequent executions are
// authorized against the new set through authorizeLocked.
func (h *Host) SetGrantedPermissions(permissions []string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.manifest.GrantedPermissions = append([]string(nil), permissions...)
}

func (h *Host) GrantedPermissions() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.manifest.GrantedPermissions...)
}
