package www

import "embed"

//go:embed *.html
//go:embed *.js
var Static embed.FS
