package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
)

const defaultProxyURL = "http://127.0.0.1:1984/mcp"

// RunProxy starts MCP stdio transport and proxies all JSON-RPC messages to an
// already running go2rtc instance over HTTP.
func RunProxy() {
	log = app.GetLogger("mcp")
	upstream := getProxyURL()

	log.Info().Str("upstream", upstream).Msg("[mcp] starting stdio proxy transport")
	runStdioProxyTransport(upstream)
}

func runStdioProxyTransport(upstream string) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	writer := bufio.NewWriter(os.Stdout)

	client := &http.Client{Timeout: 30 * time.Second}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		log.Trace().Msgf("[mcp] proxy recv: %s", string(line))

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			sendError(writer, nil, ParseError, err.Error())
			continue
		}

		respBody, err := proxyRequest(client, upstream, line)
		if err != nil {
			sendError(writer, msg.ID, InternalError, err.Error())
			continue
		}

		// Upstream notifications can return 204/empty body.
		if len(respBody) == 0 {
			continue
		}

		if !json.Valid(respBody) {
			sendError(writer, msg.ID, InternalError, "invalid JSON response from MCP upstream")
			continue
		}

		log.Trace().Msgf("[mcp] proxy send: %s", string(respBody))

		_, _ = writer.Write(respBody)
		_ = writer.WriteByte('\n')
		_ = writer.Flush()
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Error().Err(err).Msg("[mcp] stdio proxy error")
	}
}

func proxyRequest(client *http.Client, upstream string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, upstream, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", app.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return bytes.TrimSpace(respBody), nil
}

func getProxyURL() string {
	var cfg struct {
		Mod struct {
			Proxy string `yaml:"proxy"`
		} `yaml:"mcp"`
		API struct {
			Listen string `yaml:"listen"`
		} `yaml:"api"`
	}

	app.LoadConfig(&cfg)

	return resolveProxyURL(app.MCPProxyURL, cfg.Mod.Proxy, cfg.API.Listen)
}

func resolveProxyURL(cliURL, configProxy, apiListen string) string {
	if s := strings.TrimSpace(cliURL); s != "" {
		return normalizeProxyURL(s)
	}
	if s := strings.TrimSpace(configProxy); s != "" {
		return normalizeProxyURL(s)
	}
	if s := normalizeListenAddress(strings.TrimSpace(apiListen)); s != "" {
		return normalizeProxyURL(s)
	}
	return defaultProxyURL
}

func normalizeListenAddress(addr string) string {
	if addr == "" {
		return ""
	}

	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}

	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}

	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}

	return net.JoinHostPort(host, port)
}

func normalizeProxyURL(raw string) string {
	if raw == "" {
		return defaultProxyURL
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	if u.Path == "" || u.Path == "/" {
		u.Path = "/mcp"
	}

	return u.String()
}
