package www

import "embed"

//go:embed *.html
var Static embed.FS
