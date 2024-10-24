package streams

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/kballard/go-shellquote"
)

// Redact takes a raw stream URL and tries to redact sensitive values.
//
// The returned string is only meant to be used for display purposes.
func Redact(rawURL string) string {
	if scheme, rest, ok := strings.Cut(rawURL, ":"); ok {
		if scheme == "exec" {
			return fmt.Sprintf("%s:%s", scheme, redactCmd(rest))
		}

		if _, ok := redirects[scheme]; ok {
			// We're not actually following the redirect here because that
			// would alter the URL and might cause confusion when users try to
			// reason about log entries. E.g. an `ffmpeg:` URL would expand to
			// a very long ffmpeg command that the user did not configure
			// explictly.
			//
			// Instead, we just redact the remaining URL after the redirect
			// scheme and add it back afterwards.
			return fmt.Sprintf("%s:%s", scheme, redact(rest))
		}

		return redact(rawURL)
	}

	// Not a URL, leave as it.
	return rawURL
}

// RedactSlice takes a slice of strings and tries to redact sensitive values
// from URLs contained in them.
//
// The returned slice is only meant to be used for display purposes.
func RedactSlice(args []string) []string {
	redacted := make([]string, len(args))
	for i, arg := range args {
		redacted[i] = Redact(arg)
	}

	return redacted
}

func redact(rawURL string) string {
	if url, options, ok := strings.Cut(rawURL, "#"); ok {
		return fmt.Sprintf("%s%s", redactURL(url), redactOptions(options))
	}

	return redactURL(rawURL)
}

func redactCmd(rawCmd string) string {
	args, err := shellquote.Split(rawCmd)
	if err != nil {
		return rawCmd
	}

	return shellquote.Join(RedactSlice(args)...)
}

func redactURL(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		u.RawQuery = redactSensitiveValues(u.Query()).Encode()
		return u.Redacted()
	}

	return rawURL
}

func redactOptions(rawOptions string) string {
	options := ParseQuery(rawOptions)
	if len(options) == 0 {
		return ""
	}

	redactedOptions := redactSensitiveValues(options)

	// Sort keys to ensure the formatted result is stable.
	keys := make([]string, 0, len(redactedOptions))
	for k := range redactedOptions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder

	for _, key := range keys {
		for _, value := range redactedOptions[key] {
			sb.WriteRune('#')
			sb.WriteString(key)
			sb.WriteRune('=')
			sb.WriteString(value)
		}
	}

	return sb.String()
}

var (
	sensitiveKeysRe   = regexp.MustCompile("(?i).*(secret|pass|token).*")
	sensitiveValuesRe = regexp.MustCompile("(?i)Authorization:.*")
)

func redactSensitiveValues(values url.Values) url.Values {
	for key, vals := range values {
		for i, val := range vals {
			if sensitiveKeysRe.MatchString(key) || sensitiveValuesRe.MatchString(val) {
				values[key][i] = "xxxxx" // Same placeholder as emitted by (net/url.URL).Redacted().
			}
		}
	}

	return values
}
