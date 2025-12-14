package www

import "embed"

//go:embed *.html
//go:embed *.js
//go:embed *.json
var Static embed.FS
