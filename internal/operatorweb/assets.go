package operatorweb

import "embed"

//go:embed static/index.html static/app.css static/guided-ux.css static/localization.css static/app.js static/bootstrap.js static/preflight.js static/memory.js static/security.js static/configuration.js static/show-lock.js static/guided-ux.js static/localization.js
var assets embed.FS

func Read(name string) ([]byte, error) {
	return assets.ReadFile("static/" + name)
}
