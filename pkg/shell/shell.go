package shell

import (
	"strings"
)

func QuoteSplit(s string) []string {
	var a []string

	for len(s) > 0 {
		is := strings.IndexByte(s, ' ')
		if is >= 0 {
			// skip prefix and double spaces
			if is == 0 {
				// goto next symbol
				s = s[1:]
				continue
			}

			// check if quote in word
			if i := strings.IndexByte(s[:is], '"'); i >= 0 {
				// search quote end
				if is = strings.Index(s, `" `); is > 0 {
					is += 1
				} else {
					is = -1
				}
			}
		}

		if is >= 0 {
			a = append(a, strings.ReplaceAll(s[:is], `"`, ""))
			s = s[is+1:]
		} else {
			//add last word
			a = append(a, s)
			break
		}
	}
	return a
}
