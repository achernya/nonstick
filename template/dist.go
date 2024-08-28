package template

import (
	"embed"
)

//go:embed *.tmpl
var Fs embed.FS
