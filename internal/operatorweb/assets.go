package operatorweb

import "embed"

//go:embed static/index.html static/app.css static/app.js static/bootstrap.js static/preflight.js static/memory.js static/security.js static/configuration.js
var assets embed.FS

func Read(name string) ([]byte, error) {
	return assets.ReadFile("static/" + name)
}
