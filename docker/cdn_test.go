package docker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// cdnURLPattern is the same regex used by download_cdn.sh to extract CDN URLs.
var cdnURLPattern = regexp.MustCompile(`https://cdn\.jsdelivr\.net/npm/[^"' )\x60]*`)

// HTML fixtures that mirror the real www/*.html files.
var htmlFixtures = map[string]string{
	"hls.html": `<!doctype html>
<html lang="en">
<body>
<script src="https://cdn.jsdelivr.net/npm/hls.js@1"></script>
<video id="video"></video>
</body>
</html>`,

	"config.html": `<!DOCTYPE html>
<html lang="en">
<body>
<script src="https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs/loader.js"></script>
<script src="https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js"></script>
<script>
    const monacoRoot = 'https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min';
    window.MonacoEnvironment = {
        getWorkerUrl: function () {
            return ` + "`" + `data:text/javascript;charset=utf-8,${encodeURIComponent(` + "`" + `
                self.MonacoEnvironment = { baseUrl: '${monacoRoot}/' };
                importScripts('${monacoRoot}/vs/base/worker/workerMain.js');
            ` + "`" + `)}` + "`" + `;
        }
    };
    require.config({paths: {vs: ` + "`" + `${monacoRoot}/vs` + "`" + `}});
</script>
</body>
</html>`,

	"net.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <script src="https://cdn.jsdelivr.net/npm/vis-network@10.0.2/standalone/umd/vis-network.min.js"></script>
</head>
<body></body>
</html>`,

	"links.html": `<!DOCTYPE html>
<html lang="en">
<body>
<script>
    const script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/qrcodejs@1.0.0/qrcode.min.js';
    document.head.appendChild(script);
</script>
</body>
</html>`,
}

func parseCDNURL(rawURL string) (pkgName, filePath, localURL string) {
	npmPath := strings.TrimPrefix(rawURL, "https://cdn.jsdelivr.net/npm/")

	parts := strings.SplitN(npmPath, "/", 2)
	pkgVer := parts[0]

	if len(parts) > 1 {
		filePath = parts[1]
	}

	// Remove @version suffix to get package name
	if idx := strings.LastIndex(pkgVer, "@"); idx > 0 {
		pkgName = pkgVer[:idx]
	} else {
		pkgName = pkgVer
	}

	if filePath != "" {
		localURL = "cdn/" + pkgName + "/" + filePath
	} else {
		localURL = "cdn/" + pkgName + "/index.js"
	}
	return
}

// extractURLs finds all CDN URLs in the given HTML content.
func extractURLs(htmlFiles map[string]string) []string {
	seen := map[string]bool{}
	for _, content := range htmlFiles {
		for _, match := range cdnURLPattern.FindAllString(content, -1) {
			seen[match] = true
		}
	}
	urls := make([]string, 0, len(seen))
	for u := range seen {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	return urls
}

// patchHTML replaces all CDN URLs in content with local paths.
func patchHTML(content string, urls []string) string {
	for _, u := range urls {
		_, _, localURL := parseCDNURL(u)
		content = strings.ReplaceAll(content, u, localURL)
	}
	return content
}

func TestExtractURLs(t *testing.T) {
	urls := extractURLs(htmlFixtures)

	expected := []string{
		"https://cdn.jsdelivr.net/npm/hls.js@1",
		"https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js",
		"https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min",
		"https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs/loader.js",
		"https://cdn.jsdelivr.net/npm/qrcodejs@1.0.0/qrcode.min.js",
		"https://cdn.jsdelivr.net/npm/vis-network@10.0.2/standalone/umd/vis-network.min.js",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("URL[%d]: expected %q, got %q", i, expected[i], u)
		}
	}
}

func TestParseCDNURL(t *testing.T) {
	tests := []struct {
		url      string
		pkgName  string
		filePath string
		localURL string
	}{
		{
			url:      "https://cdn.jsdelivr.net/npm/hls.js@1",
			pkgName:  "hls.js",
			filePath: "",
			localURL: "cdn/hls.js/index.js",
		},
		{
			url:      "https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js",
			pkgName:  "js-yaml",
			filePath: "dist/js-yaml.min.js",
			localURL: "cdn/js-yaml/dist/js-yaml.min.js",
		},
		{
			url:      "https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs/loader.js",
			pkgName:  "monaco-editor",
			filePath: "min/vs/loader.js",
			localURL: "cdn/monaco-editor/min/vs/loader.js",
		},
		{
			url:      "https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min",
			pkgName:  "monaco-editor",
			filePath: "min",
			localURL: "cdn/monaco-editor/min",
		},
		{
			url:      "https://cdn.jsdelivr.net/npm/vis-network@10.0.2/standalone/umd/vis-network.min.js",
			pkgName:  "vis-network",
			filePath: "standalone/umd/vis-network.min.js",
			localURL: "cdn/vis-network/standalone/umd/vis-network.min.js",
		},
		{
			url:      "https://cdn.jsdelivr.net/npm/qrcodejs@1.0.0/qrcode.min.js",
			pkgName:  "qrcodejs",
			filePath: "qrcode.min.js",
			localURL: "cdn/qrcodejs/qrcode.min.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pkgName, func(t *testing.T) {
			pkgName, filePath, localURL := parseCDNURL(tt.url)
			if pkgName != tt.pkgName {
				t.Errorf("pkgName: expected %q, got %q", tt.pkgName, pkgName)
			}
			if filePath != tt.filePath {
				t.Errorf("filePath: expected %q, got %q", tt.filePath, filePath)
			}
			if localURL != tt.localURL {
				t.Errorf("localURL: expected %q, got %q", tt.localURL, localURL)
			}
		})
	}
}

func TestPatchHTML(t *testing.T) {
	urls := extractURLs(htmlFixtures)

	t.Run("hls", func(t *testing.T) {
		patched := patchHTML(htmlFixtures["hls.html"], urls)
		if !strings.Contains(patched, `src="cdn/hls.js/index.js"`) {
			t.Error("hls.js src not patched")
		}
		if strings.Contains(patched, "cdn.jsdelivr.net") {
			t.Error("CDN URL still present")
		}
		if !strings.Contains(patched, `<video id="video">`) {
			t.Error("non-CDN content damaged")
		}
	})

	t.Run("config", func(t *testing.T) {
		patched := patchHTML(htmlFixtures["config.html"], urls)
		if !strings.Contains(patched, `src="cdn/monaco-editor/min/vs/loader.js"`) {
			t.Error("monaco loader.js src not patched")
		}
		if !strings.Contains(patched, `src="cdn/js-yaml/dist/js-yaml.min.js"`) {
			t.Error("js-yaml src not patched")
		}
		if !strings.Contains(patched, "monacoRoot = 'cdn/monaco-editor/min'") {
			t.Error("monacoRoot variable not patched")
		}
		// Dynamic references via ${monacoRoot} must remain untouched
		if !strings.Contains(patched, "${monacoRoot}/") {
			t.Error("dynamic monacoRoot references damaged")
		}
		if strings.Contains(patched, "cdn.jsdelivr.net") {
			t.Error("CDN URL still present")
		}
		if !strings.Contains(patched, "require.config") {
			t.Error("non-CDN content damaged")
		}
	})

	t.Run("net", func(t *testing.T) {
		patched := patchHTML(htmlFixtures["net.html"], urls)
		if !strings.Contains(patched, `src="cdn/vis-network/standalone/umd/vis-network.min.js"`) {
			t.Error("vis-network src not patched")
		}
		if strings.Contains(patched, "cdn.jsdelivr.net") {
			t.Error("CDN URL still present")
		}
	})

	t.Run("links", func(t *testing.T) {
		patched := patchHTML(htmlFixtures["links.html"], urls)
		if !strings.Contains(patched, "src = 'cdn/qrcodejs/qrcode.min.js'") {
			t.Error("qrcodejs src not patched")
		}
		if strings.Contains(patched, "cdn.jsdelivr.net") {
			t.Error("CDN URL still present")
		}
		if !strings.Contains(patched, "document.createElement") {
			t.Error("non-CDN content damaged")
		}
	})
}

func TestExtractURLsFromRealFiles(t *testing.T) {
	// Verify the regex works against the actual www/*.html files
	wwwDir := filepath.Join("..", "www")
	entries, err := filepath.Glob(filepath.Join(wwwDir, "*.html"))
	if err != nil || len(entries) == 0 {
		t.Skip("www/*.html not found, skipping real file test")
	}

	realFiles := map[string]string{}
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		realFiles[filepath.Base(path)] = string(data)
	}

	urls := extractURLs(realFiles)
	if len(urls) < 5 {
		t.Errorf("expected at least 5 CDN URLs in real files, got %d: %v", len(urls), urls)
	}

	// Every URL must be parseable
	for _, u := range urls {
		pkgName, _, localURL := parseCDNURL(u)
		if pkgName == "" {
			t.Errorf("failed to parse package name from %q", u)
		}
		if localURL == "" {
			t.Errorf("failed to generate local URL for %q", u)
		}
	}
}

func TestMonacoVersionExtraction(t *testing.T) {
	urls := extractURLs(htmlFixtures)

	var monacoVer string
	for _, u := range urls {
		pkgName, _, _ := parseCDNURL(u)
		if pkgName == "monaco-editor" {
			npmPath := strings.TrimPrefix(u, "https://cdn.jsdelivr.net/npm/")
			pkgVer := strings.SplitN(npmPath, "/", 2)[0]
			if idx := strings.LastIndex(pkgVer, "@"); idx > 0 {
				monacoVer = pkgVer[idx+1:]
			}
			break
		}
	}

	if monacoVer != "0.55.1" {
		t.Errorf("expected monaco version 0.55.1, got %q", monacoVer)
	}
}

func TestEntrypoint(t *testing.T) {
	// Find the entrypoint.sh relative to the test file
	entrypoint := filepath.Join("entrypoint.sh")
	if _, err := os.Stat(entrypoint); err != nil {
		t.Skipf("entrypoint.sh not found: %v", err)
	}

	tmpDir := t.TempDir()

	// Create a mock go2rtc that prints its arguments
	mockBin := filepath.Join(tmpDir, "go2rtc")
	os.WriteFile(mockBin, []byte("#!/bin/sh\necho \"$@\"\n"), 0755)

	// Read the entrypoint script and adapt for testing:
	// replace "exec " with "" so the mock go2rtc output is captured,
	// replace hardcoded /var/www/go2rtc with our temp path.
	data, err := os.ReadFile(entrypoint)
	if err != nil {
		t.Fatal(err)
	}

	webDir := filepath.Join(tmpDir, "var", "www", "go2rtc")
	script := strings.ReplaceAll(string(data), "exec ", "")
	script = strings.ReplaceAll(script, "/var/www/go2rtc", webDir)
	script = strings.ReplaceAll(script, "/config/go2rtc.yaml", "/tmp/test.yaml")

	testScript := filepath.Join(tmpDir, "test_entrypoint.sh")
	os.WriteFile(testScript, []byte(script), 0755)

	run := func(extraArgs ...string) string {
		args := append([]string{testScript}, extraArgs...)
		cmd := exec.Command("sh", args...)
		cmd.Env = append(os.Environ(), "PATH="+tmpDir+":"+os.Getenv("PATH"))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("entrypoint failed: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	t.Run("with_web_dir", func(t *testing.T) {
		os.MkdirAll(webDir, 0755)
		defer os.RemoveAll(filepath.Join(tmpDir, "var"))

		result := run("--extra-flag")

		if !strings.Contains(result, "static_dir") {
			t.Error("static_dir not added when web dir exists")
		}
		if !strings.Contains(result, "-config /tmp/test.yaml") {
			t.Error("user config not present")
		}
		if !strings.Contains(result, "--extra-flag") {
			t.Error("extra args not passed through")
		}
	})

	t.Run("without_web_dir", func(t *testing.T) {
		os.RemoveAll(filepath.Join(tmpDir, "var"))

		result := run("--extra-flag")

		if strings.Contains(result, "static_dir") {
			t.Error("static_dir added when web dir absent")
		}
		if !strings.Contains(result, "-config /tmp/test.yaml") {
			t.Error("user config not present")
		}
		if !strings.Contains(result, "--extra-flag") {
			t.Error("extra args not passed through")
		}
	})

	t.Run("config_order", func(t *testing.T) {
		os.MkdirAll(webDir, 0755)
		defer os.RemoveAll(filepath.Join(tmpDir, "var"))

		result := run()

		// static_dir config must come BEFORE user config
		// so user config can override it
		staticIdx := strings.Index(result, "static_dir")
		yamlIdx := strings.Index(result, "/tmp/test.yaml")
		if staticIdx < 0 || yamlIdx < 0 {
			t.Fatalf("expected both configs in output: %q", result)
		}
		if staticIdx > yamlIdx {
			t.Errorf("static_dir config should come before user config for correct override order, got: %q", result)
		}
	})
}
