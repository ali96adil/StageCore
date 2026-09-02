package operatorweb

import "embed"

//go:embed static/index.html static/app.css static/guided-ux.css static/localization.css static/theme.css static/workspace-profile.css static/first-run.css static/extensions.css static/app.js static/bootstrap.js static/preflight.js static/memory.js static/security.js static/configuration.js static/show-lock.js static/guided-ux.js static/device-profiles.js static/localization.js static/theme.js static/workspace-profile.js static/first-run.js static/extensions.js static/extensions-uninstall.js static/extensions-maintenance.js static/extensions-set-manifest.js static/execution-environments.js static/execution-environment-capture.js static/execution-environments-workspace.js
var assets embed.FS

func Read(name string) ([]byte, error) {
	return assets.ReadFile("static/" + name)
}
