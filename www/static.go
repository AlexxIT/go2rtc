package www

import "embed"

//go:embed *.html
//go:embed *.js
//go:embed *.css
var Static embed.FS
