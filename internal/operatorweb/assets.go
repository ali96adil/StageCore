package operatorweb

import "embed"

// assets are compiled into stagecore-hub so the Operator Web remains usable
// with the Internet disconnected and without a separate frontend server.
//
//go:embed static/index.html static/app.css static/app.js static/preflight.js static/memory.js
var assets embed.FS

func Read(name string) ([]byte, error) {
	return assets.ReadFile("static/" + name)
}
