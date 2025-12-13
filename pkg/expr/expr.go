package expr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

func newRequest(rawURL string, options map[string]any) (*http.Request, error) {
	var method, contentType string
	var rd io.Reader

	// method from js fetch
	if s, ok := options["method"].(string); ok {
		method = s
	} else {
		method = "GET"
	}

	// params key from python requests
	if kv, ok := options["params"].(map[string]any); ok {
		rawURL += "?" + url.Values(kvToString(kv)).Encode()
	}

	// json key from python requests
	// data key from python requests
	// body key from js fetch
	if v, ok := options["json"]; ok {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		contentType = "application/json"
		rd = bytes.NewReader(b)
	} else if kv, ok := options["data"].(map[string]any); ok {
		contentType = "application/x-www-form-urlencoded"
		rd = strings.NewReader(url.Values(kvToString(kv)).Encode())
	} else if s, ok := options["body"].(string); ok {
		rd = strings.NewReader(s)
	}

	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		return nil, err
	}

	if kv, ok := options["headers"].(map[string]any); ok {
		req.Header = kvToString(kv)
	}

	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req, nil
}

func kvToString(kv map[string]any) map[string][]string {
	dst := make(map[string][]string, len(kv))
	for k, v := range kv {
		dst[k] = []string{fmt.Sprintf("%v", v)}
	}
	return dst
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

func Compile(input string) (*vm.Program, error) {
	// support http sessions
	jar, _ := cookiejar.New(nil)
	client := http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}

	return expr.Compile(
		input,
		expr.Function(
			"fetch",
			func(params ...any) (any, error) {
				var req *http.Request
				var err error

				rawURL := params[0].(string)

				if len(params) == 2 {
					options := params[1].(map[string]any)
					req, err = newRequest(rawURL, options)
				} else {
					req, err = http.NewRequest("GET", rawURL, nil)
				}

				if err != nil {
					return nil, err
				}

				res, err := client.Do(req)
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
	)
}

func Eval(input string, env any) (any, error) {
	program, err := Compile(input)
	if err != nil {
		return nil, err
	}

	return expr.Run(program, env)
}

func Run(program *vm.Program, env any) (any, error) {
	return vm.Run(program, env)
}
