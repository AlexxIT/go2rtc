package expr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/antonmedv/expr"
)

func newRequest(method, url string, headers map[string]any) (*http.Request, error) {
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}

	return req, nil
}

func regExp(params ...any) (*regexp.Regexp, error) {
	exp := params[0].(string)
	if len(params) >= 2 {
		// support:
		//   i  case-insensitive (default false)
		//   m  multi-line mode: ^ and $ match begin/end line (default false)
		//   s  let . match \n (default false)
		// https://pkg.go.dev/regexp/syntax
		flags := params[1].(string)
		exp = "(?" + flags + ")" + exp
	}
	return regexp.Compile(exp)
}

var Options = []expr.Option{
	expr.Function(
		"fetch",
		func(params ...any) (any, error) {
			var req *http.Request
			var err error

			url := params[0].(string)

			if len(params) == 2 {
				options := params[1].(map[string]any)
				method, _ := options["method"].(string)
				headers, _ := options["headers"].(map[string]any)
				req, err = newRequest(method, url, headers)
			} else {
				req, err = http.NewRequest("GET", url, nil)
			}

			if err != nil {
				return nil, err
			}

			res, err := tcp.Do(req)
			if err != nil {
				return nil, err
			}

			b, _ := io.ReadAll(res.Body)

			return map[string]any{
				"ok":     res.StatusCode < 400,
				"status": res.Status,
				"text":   string(b),
				"json": func() (v any) {
					_ = json.Unmarshal(b, &v)
					return
				},
			}, nil
		},
		//new(func(url string) map[string]any),
		//new(func(url string, options map[string]any) map[string]any),
	),
	expr.Function(
		"match",
		func(params ...any) (any, error) {
			re, err := regExp(params[1:]...)
			if err != nil {
				return nil, err
			}
			str := params[0].(string)
			return re.FindStringSubmatch(str), nil
		},
		//new(func(str, expr string) []string),
		//new(func(str, expr, flags string) []string),
	),
	expr.Function(
		"RegExp",
		func(params ...any) (any, error) {
			return regExp(params)
		},
	),
}

func Run(input string) (any, error) {
	program, err := expr.Compile(input, Options...)
	if err != nil {
		return nil, err
	}

	return expr.Run(program, nil)
}
