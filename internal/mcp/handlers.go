package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

func sourceScheme(source string) string {
	if i := strings.IndexByte(source, ':'); i > 0 {
		return source[:i]
	}
	return ""
}

func sourceValidationHint(source string, err error) string {
	if err == nil {
		return ""
	}

	switch {
	case strings.Contains(err.Error(), "streams: source from insecure producer"):
		scheme := sourceScheme(source)
		if scheme == "" {
			return "Runtime MCP tools block insecure producer schemes; configure this source in go2rtc.yaml under streams."
		}
		return fmt.Sprintf(
			"Runtime MCP tools block insecure producer scheme %q; configure this source in go2rtc.yaml under streams.",
			scheme+":",
		)
	case strings.Contains(err.Error(), "streams: source with spaces may be insecure"):
		return "Runtime MCP tools block sources with spaces; use URL-style sources without raw shell command arguments (for example ffmpeg:virtual?...)."
	default:
		return ""
	}
}

func validationErrorWithHint(source string, err error) error {
	if err == nil {
		return nil
	}
	if hint := sourceValidationHint(source, err); hint != "" {
		return fmt.Errorf("%w (%s)", err, hint)
	}
	return err
}

// Tool Handlers

func handleListStreams(ctx context.Context, args map[string]any) (any, error) {
	details := false
	if d, ok := args["details"].(bool); ok {
		details = d
	}

	allSources := streams.GetAllSources()
	names := streams.GetAllNames()

	if details {
		result := make([]map[string]any, 0, len(names))
		for _, name := range names {
			stream := streams.Get(name)
			if stream == nil {
				continue
			}

			info := map[string]any{
				"name":    name,
				"sources": stream.Sources(),
			}

			// Get producer info if available
			if producers := getProducersInfo(stream); len(producers) > 0 {
				info["producers"] = producers
			}

			result = append(result, info)
		}
		return result, nil
	}

	return map[string]any{
		"count":   len(names),
		"streams": allSources,
	}, nil
}

func handleGetStream(ctx context.Context, args map[string]any) (any, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	result := map[string]any{
		"name":    name,
		"sources": stream.Sources(),
	}

	if producers := getProducersInfo(stream); len(producers) > 0 {
		result["producers"] = producers
	}

	// Get stream state as JSON
	if jsonBytes, err := json.Marshal(stream); err == nil {
		var streamInfo map[string]any
		if json.Unmarshal(jsonBytes, &streamInfo) == nil {
			result["details"] = streamInfo
		}
	}

	return result, nil
}

func handleAddStream(ctx context.Context, args map[string]any) (any, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	url, ok := args["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("stream URL is required")
	}

	stream, err := streams.Patch(name, url)
	if err != nil {
		return nil, fmt.Errorf("failed to add stream: %w", validationErrorWithHint(url, err))
	}

	return map[string]any{
		"success": true,
		"name":    name,
		"url":     url,
		"sources": stream.Sources(),
	}, nil
}

func handleDeleteStream(ctx context.Context, args map[string]any) (any, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	// Check if stream exists
	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	streams.Delete(name)

	return map[string]any{
		"success": true,
		"name":    name,
	}, nil
}

func handleGetConfig(ctx context.Context, args map[string]any) (any, error) {
	section := ""
	if s, ok := args["section"].(string); ok {
		section = s
	}

	// Get current app info which includes config
	result := map[string]any{
		"info": app.Info,
	}

	if section != "" {
		// Try to get specific section from config
		var cfg any
		switch section {
		case "streams":
			cfg = streams.GetAllSources()
		default:
			return nil, fmt.Errorf("unknown config section: %s", section)
		}
		result["config"] = cfg
	}

	return result, nil
}

func handleGetInfo(ctx context.Context, args map[string]any) (any, error) {
	return app.Info, nil
}

func handleListSchemes(ctx context.Context, args map[string]any) (any, error) {
	schemes := streams.SupportedSchemes()
	sort.Strings(schemes)

	return map[string]any{
		"count":   len(schemes),
		"schemes": schemes,
	}, nil
}

func handleValidateSource(ctx context.Context, args map[string]any) (any, error) {
	source, err := getStringArg(args, "source", true)
	if err != nil {
		return nil, err
	}

	validationErr := streams.Validate(source)
	supported := streams.HasProducer(source)

	result := map[string]any{
		"source":    source,
		"supported": supported,
		"valid":     validationErr == nil,
	}
	if validationErr != nil {
		result["error"] = validationErr.Error()
		if hint := sourceValidationHint(source, validationErr); hint != "" {
			result["hint"] = hint
		}
	}

	return result, nil
}

func handlePlayStream(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}
	source, err := getStringArg(args, "source", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	if err = streams.Validate(source); err != nil {
		return nil, validationErrorWithHint(source, err)
	}

	if err = stream.Play(source); err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"name":    name,
		"source":  source,
	}, nil
}

func handlePublishStream(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}
	destination, err := getStringArg(args, "destination", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	if err = streams.Validate(destination); err != nil {
		return nil, validationErrorWithHint(destination, err)
	}

	if err = stream.Publish(destination); err != nil {
		return nil, err
	}

	return map[string]any{
		"success":     true,
		"name":        name,
		"destination": destination,
	}, nil
}

func handleListPreloads(ctx context.Context, args map[string]any) (any, error) {
	preloads := streams.GetPreloads()
	names := make([]string, 0, len(preloads))
	for name := range preloads {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]map[string]any, 0, len(names))
	for _, name := range names {
		p := preloads[name]
		items = append(items, map[string]any{
			"name":   name,
			"query":  p.Query,
			"active": p.Cons != nil,
		})
	}

	return map[string]any{
		"count":    len(items),
		"preloads": items,
	}, nil
}

func handleAddPreload(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}
	query, err := getStringArg(args, "query", false)
	if err != nil {
		return nil, err
	}

	if err = streams.AddPreload(name, query); err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"name":    name,
		"query":   query,
	}, nil
}

func handleDeletePreload(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	if err = streams.DelPreload(name); err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"name":    name,
	}, nil
}

func handleGetStreamsDOT(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", false)
	if err != nil {
		return nil, err
	}

	dot := make([]byte, 0, 1024)
	dot = append(dot, "digraph {\n"...)

	if name != "" {
		stream := streams.Get(name)
		if stream == nil {
			return nil, fmt.Errorf("stream not found: %s", name)
		}
		dot = streams.AppendDOT(dot, stream)
	} else {
		names := streams.GetAllNames()
		sort.Strings(names)
		for _, streamName := range names {
			if stream := streams.Get(streamName); stream != nil {
				dot = streams.AppendDOT(dot, stream)
			}
		}
	}

	dot = append(dot, '}')

	return map[string]any{
		"name": name,
		"dot":  string(dot),
	}, nil
}

// Resource Handlers

func handleStreamsResource(ctx context.Context, uri string) (string, error) {
	names := streams.GetAllNames()

	result := map[string]any{
		"count": len(names),
	}

	for _, name := range names {
		stream := streams.Get(name)
		if stream != nil {
			result[name] = map[string]any{
				"sources":   stream.Sources(),
				"producers": getProducersInfo(stream),
			}
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleStreamResource(ctx context.Context, uri string) (string, error) {
	// Extract stream name from URI (format: streams://{name})
	name := strings.TrimPrefix(uri, "streams://")
	if name == "" {
		return "", fmt.Errorf("invalid stream URI")
	}

	stream := streams.Get(name)
	if stream == nil {
		return "", fmt.Errorf("stream not found: %s", name)
	}

	result := map[string]any{
		"name":    name,
		"sources": stream.Sources(),
	}

	if producers := getProducersInfo(stream); len(producers) > 0 {
		result["producers"] = producers
	}

	// Get full stream details
	if jsonBytes, err := json.Marshal(stream); err == nil {
		var details map[string]any
		if json.Unmarshal(jsonBytes, &details) == nil {
			result["details"] = details
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleLogResource(ctx context.Context, uri string) (string, error) {
	// Return current log as JSON Lines format
	var buf strings.Builder
	_, _ = app.MemoryLog.WriteTo(&buf)
	return buf.String(), nil
}

func handleEventsResource(ctx context.Context, uri string) (string, error) {
	// Return recent events as JSON
	events := eventLog.Get(50)

	result := map[string]any{
		"count":  len(events),
		"events": events,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// Prompt Handlers

func handleStreamAnalyzerPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	sources := stream.Sources()
	producers := getProducersInfo(stream)

	// Build analysis prompt
	var analysis strings.Builder

	fmt.Fprintf(&analysis, "# Stream Analysis: %s\n\n", name)
	fmt.Fprintf(&analysis, "## Sources (%d)\n", len(sources))
	for i, src := range sources {
		fmt.Fprintf(&analysis, "%d. `%s`\n", i+1, src)
	}

	fmt.Fprintf(&analysis, "\n## Producers (%d)\n", len(producers))
	for i, prod := range producers {
		fmt.Fprintf(&analysis, "%d. %s\n", i+1, prod["mode"])
		if state, ok := prod["state"].(string); ok {
			fmt.Fprintf(&analysis, "   State: %s\n", state)
		}
		if url, ok := prod["url"].(string); ok {
			fmt.Fprintf(&analysis, "   URL: %s\n", url)
		}
	}

	// Add helpful analysis questions
	analysis.WriteString("\n## Analysis Points\n\n")
	analysis.WriteString("- Are all sources accessible?\n")
	analysis.WriteString("- Are there any connection errors in the logs?\n")
	analysis.WriteString("- Is the stream producing media (check producers state)?\n")
	analysis.WriteString("- Are there any consumers connected?\n")

	messages := []PromptMessage{
		{
			Role: "user",
		},
	}
	messages[0].Content.Type = "text"
	messages[0].Content.Text = analysis.String()

	return messages, nil
}

func handleStreamSetupPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}
	source, err := getStringArg(args, "source", true)
	if err != nil {
		return nil, err
	}

	var prompt strings.Builder
	fmt.Fprintf(&prompt, "# go2rtc Stream Setup Plan: %s\n\n", name)
	fmt.Fprintf(&prompt, "Target source: `%s`\n\n", source)
	fmt.Fprintf(&prompt, "1. Validate source:\n")
	fmt.Fprintf(&prompt, "   - tool: `validate_source` with `{ \"source\": \"%s\" }`\n", source)
	fmt.Fprintf(&prompt, "2. Create or update stream:\n")
	fmt.Fprintf(&prompt, "   - tool: `add_stream` with `{ \"name\": \"%s\", \"url\": \"%s\" }`\n", name, source)
	fmt.Fprintf(&prompt, "3. Verify state:\n")
	fmt.Fprintf(&prompt, "   - tool: `get_stream` with `{ \"name\": \"%s\" }`\n", name)
	fmt.Fprintf(&prompt, "4. If startup latency matters, preload:\n")
	fmt.Fprintf(&prompt, "   - tool: `add_preload` with `{ \"name\": \"%s\", \"query\": \"video&audio\" }`\n", name)
	fmt.Fprintf(&prompt, "\nRespond with a concise checklist and the exact MCP tool calls you will run.\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleOperationsPlaybookPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	name, err := getStringArg(args, "name", false)
	if err != nil {
		return nil, err
	}

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Operations Playbook\n\n")
	if name != "" {
		fmt.Fprintf(&prompt, "Focus stream: `%s`\n\n", name)
		fmt.Fprintf(&prompt, "Recommended sequence:\n")
		fmt.Fprintf(&prompt, "1. `get_stream` for `%s`\n", name)
		fmt.Fprintf(&prompt, "2. `stream_analyzer` prompt for `%s`\n", name)
		fmt.Fprintf(&prompt, "3. `validate_source` for each source URL\n")
		fmt.Fprintf(&prompt, "4. `list_preloads` to verify warmup state\n")
		fmt.Fprintf(&prompt, "5. `get_streams_dot` for topology view\n")
	} else {
		prompt.WriteString("Recommended sequence:\n")
		prompt.WriteString("1. `list_streams` with `{ \"details\": true }`\n")
		prompt.WriteString("2. `list_preloads`\n")
		prompt.WriteString("3. `list_schemes`\n")
		prompt.WriteString("4. `get_streams_dot`\n")
		prompt.WriteString("5. For problematic streams, run `stream_analyzer`\n")
	}
	prompt.WriteString("\nGenerate a short actionable maintenance plan (prioritized).\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleSourceConfiguratorPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	sourceType, _ := getStringArg(args, "source_type", false)

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Source Configuration Guide\n\n")

	switch sourceType {
	case "rtsp":
		prompt.WriteString("## RTSP Source Configuration\n\n")
		prompt.WriteString("Basic format: `rtsp://user:password@host:port/path`\n\n")
		prompt.WriteString("Common options:\n")
		prompt.WriteString("- Add query params: `?transport=tcp` (force TCP)\n")
		prompt.WriteString("- Authentication: include username and password in URL\n")
		prompt.WriteString("- RTSP over TCP: use `rtsp://` with `?transport=tcp`\n\n")
		prompt.WriteString("Example: `rtsp://admin:pass@192.168.1.100:554/stream1`\n")
	case "http", "https", "http-flv", "http-mjpeg":
		prompt.WriteString("## HTTP/HTTPS Source Configuration\n\n")
		prompt.WriteString("Supported formats:\n")
		prompt.WriteString("- HTTP FLV: `http://host/port/stream.flv`\n")
		prompt.WriteString("- HTTP MJPEG: `http://host/port/stream.mjpg`\n")
		prompt.WriteString("- HTTP TS: `http://host/port/stream.ts`\n\n")
		prompt.WriteString("For authenticated sources, add credentials:\n")
		prompt.WriteString("`http://user:pass@host:port/path`\n")
	case "rtmp":
		prompt.WriteString("## RTMP Source Configuration\n\n")
		prompt.WriteString("Format: `rtmp://host:port/app/stream`\n\n")
		prompt.WriteString("Example: `rtmp://live.example.com/live/mystream`\n\n")
		prompt.WriteString("For authenticated streams:\n")
		prompt.WriteString("`rtmp://user:pass@host:1935/app/stream`\n")
	case "ffmpeg":
		prompt.WriteString("## FFmpeg Source Configuration\n\n")
		prompt.WriteString("Format: `ffmpeg:URL#options`\n\n")
		prompt.WriteString("Common options:\n")
		prompt.WriteString("- `#video` - video only\n")
		prompt.WriteString("- `#audio` - audio only\n")
		prompt.WriteString("- `#hardware` - use hardware acceleration\n")
		prompt.WriteString("- `#copy` - copy streams without re-encoding\n\n")
		prompt.WriteString("Example: `ffmpeg:rtsp://host/stream#video`\n")
	case "webrtc", "whip", "whep":
		prompt.WriteString("## WebRTC/WHIP/WHEP Source Configuration\n\n")
		prompt.WriteString("Formats:\n")
		prompt.WriteString("- WHIP (WebRTC-HTTP ingestion): `whip:https://host/whip`\n")
		prompt.WriteString("- WHEP (WebRTC-HTTP playback): `whep:https://host/whep`\n\n")
		prompt.WriteString("WebRTC requires ICE/STUN configuration for NAT traversal.\n")
	default:
		prompt.WriteString("## Supported Source Types\n\n")
		prompt.WriteString("1. **RTSP** - IP cameras, streaming servers\n")
		prompt.WriteString("2. **HTTP/HTTPS** - FLV, MJPEG, TS streams\n")
		prompt.WriteString("3. **RTMP** - Adobe Real-Time Messaging Protocol\n")
		prompt.WriteString("4. **FFmpeg** - Any source supported by FFmpeg\n")
		prompt.WriteString("5. **WebRTC** - WHIP/WHEP protocols\n")
		prompt.WriteString("6. **Local devices** - USB cameras, capture cards\n")
		prompt.WriteString("7. **Expresssions** - `expr:` for dynamic sources\n\n")
		prompt.WriteString("Use this prompt with a specific source_type parameter for detailed configuration.\n")
	}

	prompt.WriteString("\n## Configuration Steps\n\n")
	prompt.WriteString("1. Use `validate_source` to test your URL format\n")
	prompt.WriteString("2. Use `add_stream` to add the source to go2rtc\n")
	prompt.WriteString("3. Use `get_stream` to verify connection status\n")
	prompt.WriteString("4. Use `get_stream_urls` to get output URLs for playback\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleTroubleshootingPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	issue, _ := getStringArg(args, "issue", false)

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Troubleshooting Guide\n\n")

	switch issue {
	case "no_connection", "connection_failed":
		prompt.WriteString("## Connection Issues\n\n")
		prompt.WriteString("### Diagnostic Steps:\n")
		prompt.WriteString("1. `validate_source` - Check if URL format is correct\n")
		prompt.WriteString("2. `get_stream` - Check producer state (should be 'connected')\n")
		prompt.WriteString("3. `get_events` - Review recent connection attempts\n\n")
		prompt.WriteString("### Common Fixes:\n")
		prompt.WriteString("- For RTSP: Try `?transport=tcp` if UDP fails\n")
		prompt.WriteString("- Check network connectivity to source host\n")
		prompt.WriteString("- Verify username/password in URL\n")
		prompt.WriteString("- Check if source is already used by another client\n")
	case "no_audio", "no_video":
		prompt.WriteString("## Missing Audio or Video\n\n")
		prompt.WriteString("### Diagnostic Steps:\n")
		prompt.WriteString("1. `get_stream_stats` - Check producer media tracks\n")
		prompt.WriteString("2. `get_streams_dot` - Verify stream topology\n\n")
		prompt.WriteString("### Common Fixes:\n")
		prompt.WriteString("- Add `#video` or `#audio` to FFmpeg sources\n")
		prompt.WriteString("- Check source encoder settings\n")
		prompt.WriteString("- Verify codec compatibility (H.264 recommended)\n")
	case "high_latency", "delay":
		prompt.WriteString("## High Latency/Delay\n\n")
		prompt.WriteString("### Diagnostic Steps:\n")
		prompt.WriteString("1. `get_stream_stats` - Check producer/consumer lag\n")
		prompt.WriteString("2. `list_preloads` - Check if preload is active\n\n")
		prompt.WriteString("### Common Fixes:\n")
		prompt.WriteString("- Enable preloads: `add_preload` for faster startup\n")
		prompt.WriteString("- Use WebRTC or MSE for lower latency playback\n")
		prompt.WriteString("- Reduce source bitrate if bandwidth limited\n")
		prompt.WriteString("- Use TCP transport for RTSP to avoid packet loss\n")
	case "high_cpu", "performance":
		prompt.WriteString("## High CPU Usage\n\n")
		prompt.WriteString("### Diagnostic Steps:\n")
		prompt.WriteString("1. `list_streams` with details - Check active streams\n")
		prompt.WriteString("2. `get_connections` - Check consumer count\n\n")
		prompt.WriteString("### Common Fixes:\n")
		prompt.WriteString("- Reduce number of active consumers\n")
		prompt.WriteString("- Use hardware acceleration (`ffmpeg:url#hardware`)\n")
		prompt.WriteString("- Use copy mode where possible (`#copy`)\n")
		prompt.WriteString("- Disable unused streams\n")
	default:
		prompt.WriteString("## Common Issues\n\n")
		prompt.WriteString("Select a specific issue type:\n")
		prompt.WriteString("- `no_connection` - Source won't connect\n")
		prompt.WriteString("- `no_audio` / `no_video` - Missing media track\n")
		prompt.WriteString("- `high_latency` - Excessive delay in playback\n")
		prompt.WriteString("- `high_cpu` - Performance issues\n\n")
		prompt.WriteString("### General Diagnostics:\n")
		prompt.WriteString("1. Run `stream_analyzer` for the affected stream\n")
		prompt.WriteString("2. Check `get_events` for recent errors\n")
		prompt.WriteString("3. Review log output via `log://` resource\n")
	}

	prompt.WriteString("\n### Still Having Issues?\n\n")
	prompt.WriteString("Run `operations_playbook` for a systematic diagnostic approach.\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleIntegrationHelperPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	platform, _ := getStringArg(args, "platform", false)

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Integration Guide\n\n")

	switch platform {
	case "home_assistant":
		prompt.WriteString("## Home Assistant Integration\n\n")
		prompt.WriteString("### Configuration\n")
		prompt.WriteString("Add to your `configuration.yaml`:\n\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString("go2rtc:\n")
		prompt.WriteString("  url: http://go2rtc-host:1984\n")
		prompt.WriteString("```\n\n")
		prompt.WriteString("### Stream URLs for HA\n")
		prompt.WriteString("Camera configuration:\n")
		prompt.WriteString("- Camera Stream: `http://go2rtc-host:1984/mjpeg/camera_name`\n")
		prompt.WriteString("- HLS for Lovelace: `http://go2rtc-host:1984/hls/camera_name.m3u8`\n\n")
	case "frigate":
		prompt.WriteString("## Frigate Integration\n\n")
		prompt.WriteString("### go2rtc as Video Source\n")
		prompt.WriteString("In Frigate `config.yml`:\n\n")
		prompt.WriteString("```yaml\ncameras:\n")
		prompt.WriteString("  front_door:\n")
		prompt.WriteString("    ffmpeg:\n")
		prompt.WriteString("      inputs:\n")
		prompt.WriteString("        - path: rtsp://go2rtc:8554/camera_name\n")
		prompt.WriteString("          input_args: preset-rtsp-restful\n")
		prompt.WriteString("```\n\n")
	case "homebridge":
		prompt.WriteString("## HomeBridge Integration\n\n")
		prompt.WriteString("### FFmpeg Camera Plugin\n")
		prompt.WriteString("Use go2rtc as the source:\n\n")
		prompt.WriteString("```json\n")
		prompt.WriteString("{\n")
		prompt.WriteString("  \"platform\": \"Camera-ffmpeg\",\n")
		prompt.WriteString("  \"cameras\": [\n")
		prompt.WriteString("    {\n")
		prompt.WriteString("      \"name\": \"Camera\",\n")
		prompt.WriteString("      \"videoConfig\": {\n")
		prompt.WriteString("        \"source\": \"-re -i http://go2rtc-host:1984/mjpeg/camera_name\",\n")
		prompt.WriteString("        \"stillImageSource\": \"-i http://go2rtc-host:1984/mjpeg/camera_name\"\n")
		prompt.WriteString("      }\n")
		prompt.WriteString("    }\n")
		prompt.WriteString("  ]\n")
		prompt.WriteString("}\n")
		prompt.WriteString("```\n\n")
	case "mqtt":
		prompt.WriteString("## MQTT Integration\n\n")
		prompt.WriteString("### Publishing Stream URLs\n")
		prompt.WriteString("Use `get_stream_urls` to get available URLs for MQTT publishing.\n\n")
		prompt.WriteString("### Common Use Cases\n")
		prompt.WriteString("- Publish MJPEG snapshots to topics\n")
		prompt.WriteString("- Publish HLS URLs for dashboards\n")
		prompt.WriteString("- Subscribe to MQTT for stream control\n")
	case "nvr", "ispy", "blueiris":
		prompt.WriteString("## NVR Integration (iSpy, Blue Iris, etc.)\n\n")
		prompt.WriteString("### RTSP Output from go2rtc\n")
		prompt.WriteString("NVRs can pull from go2rtc:\n\n")
		prompt.WriteString("```\n")
		prompt.WriteString("rtsp://go2rtc-host:8554/stream_name\n")
		prompt.WriteString("```\n\n")
		prompt.WriteString("### Pushing to NVR\n")
		prompt.WriteString("Use `publish_stream` to push to NVR:\n")
		prompt.WriteString("```\n")
		prompt.WriteString("rtmp://nvr-host:1935/live/stream\n")
		prompt.WriteString("```\n")
	default:
		prompt.WriteString("## Supported Integrations\n\n")
		prompt.WriteString("Select a platform:\n")
		prompt.WriteString("- `home_assistant` - Home Assistant camera integration\n")
		prompt.WriteString("- `frigate` - NVR with object detection\n")
		prompt.WriteString("- `homebridge` - Apple HomeKit support\n")
		prompt.WriteString("- `mqtt` - Message bus integration\n")
		prompt.WriteString("- `nvr` - Generic NVR integration\n\n")
		prompt.WriteString("## General Integration Steps\n\n")
		prompt.WriteString("1. `add_stream` - Add your camera sources to go2rtc\n")
		prompt.WriteString("2. `get_stream_urls` - Get output URLs for your platform\n")
		prompt.WriteString("3. Configure external system to use go2rtc URLs\n")
		prompt.WriteString("4. `get_connections` - Verify active connections\n")
	}

	return singlePromptMessage(prompt.String()), nil
}

func handlePerformanceOptimizerPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	goal, _ := getStringArg(args, "goal", false)

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Performance Optimization\n\n")

	switch goal {
	case "low_latency":
		prompt.WriteString("## Minimize Latency\n\n")
		prompt.WriteString("### Configuration:\n")
		prompt.WriteString("1. Enable preloads for instant startup:\n")
		prompt.WriteString("   `add_preload` with `{\"name\": \"stream\", \"query\": \"video&audio\"}`\n\n")
		prompt.WriteString("2. Use low-latency protocols:\n")
		prompt.WriteString("   - WebRTC (lowest): `/api/stream?src=name`\n")
		prompt.WriteString("   - MSE (very low): `/mse/name`\n")
		prompt.WriteString("   - MP4 live (low): `/mp4/name.live.mp4`\n\n")
		prompt.WriteString("3. For RTSP sources, force TCP:\n")
		prompt.WriteString("   `rtsp://host/path?transport=tcp`\n\n")
		prompt.WriteString("4. Avoid HLS (high latency)\n")
	case "bandwidth":
		prompt.WriteString("## Optimize Bandwidth Usage\n\n")
		prompt.WriteString("### Strategies:\n")
		prompt.WriteString("1. Use WebRTC for single-consumer streams\n")
		prompt.WriteString("2. Enable preloads to reduce reconnection overhead\n")
		prompt.WriteString("3. Adjust source bitrate at the camera level\n")
		prompt.WriteString("4. Use MJPEG for snapshots only (lower bandwidth than video)\n")
	case "cpu", "resource":
		prompt.WriteString("## Optimize CPU/Resource Usage\n\n")
		prompt.WriteString("### Configuration:\n")
		prompt.WriteString("1. Use hardware acceleration:\n")
		prompt.WriteString("   `ffmpeg:url#hardware`\n\n")
		prompt.WriteString("2. Use copy mode where possible:\n")
		prompt.WriteString("   `ffmpeg:url#copy`\n\n")
		prompt.WriteString("3. Disable unused streams:\n")
		prompt.WriteString("   `delete_stream` for inactive sources\n\n")
		prompt.WriteString("4. Monitor connections:\n")
		prompt.WriteString("   `get_connections` - Check for excessive consumers\n")
	case "reliability":
		prompt.WriteString("## Improve Reliability\n\n")
		prompt.WriteString("### Configuration:\n")
		prompt.WriteString("1. Enable preloads for all critical streams:\n")
		prompt.WriteString("   `list_preloads` - Verify all critical streams are preloaded\n\n")
		prompt.WriteString("2. Use TCP transport for RTSP:\n")
		prompt.WriteString("   `rtsp://host/path?transport=tcp`\n\n")
		prompt.WriteString("3. Configure source priority with multiple URLs:\n")
		prompt.WriteString("   `add_stream` with comma-separated sources\n\n")
		prompt.WriteString("4. Set up connection monitoring:\n")
		prompt.WriteString("   Use `get_events` to track stream state changes\n")
	default:
		prompt.WriteString("## Optimization Goals\n\n")
		prompt.WriteString("Select a goal:\n")
		prompt.WriteString("- `low_latency` - Minimize playback delay\n")
		prompt.WriteString("- `bandwidth` - Reduce network usage\n")
		prompt.WriteString("- `cpu` - Lower CPU usage\n")
		prompt.WriteString("- `reliability` - Improve stability\n\n")
		prompt.WriteString("## Performance Diagnostics\n\n")
		prompt.WriteString("1. `get_connections` - Overview of all connections\n")
		prompt.WriteString("2. `get_stream_stats` - Per-stream performance data\n")
		prompt.WriteString("3. `get_events` - Recent state changes and errors\n")
		prompt.WriteString("4. `list_preloads` - Check preload configuration\n")
	}

	return singlePromptMessage(prompt.String()), nil
}

func handleSecurityAuditorPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	var prompt strings.Builder
	prompt.WriteString("# go2rtc Security Audit\n\n")
	prompt.WriteString("## Security Checklist\n\n")

	prompt.WriteString("### 1. Authentication & Access Control\n")
	prompt.WriteString("- [ ] API authentication enabled?\n")
	prompt.WriteString("- [ ] RTSP/RTMP sources have strong passwords?\n")
	prompt.WriteString("- [ ] Not exposing go2rtc directly to internet?\n")
	prompt.WriteString("- [ ] Using reverse proxy with auth?\n\n")

	prompt.WriteString("### 2. Source Configuration\n")
	prompt.WriteString("- [ ] Credentials in config files are secured?\n")
	prompt.WriteString("- [ ] No hardcoded passwords in stream URLs?\n")
	prompt.WriteString("- [ ] Using HTTPS for HTTP sources where possible?\n")
	prompt.WriteString("- [ ] RTSP sources use authenticated connections?\n\n")

	prompt.WriteString("### 3. Network Security\n")
	prompt.WriteString("- [ ] MCP server access restricted (HTTP/WebSocket)?\n")
	prompt.WriteString("- [ ] Unix socket used for local MCP only?\n")
	prompt.WriteString("- [ ] Firewall rules limit incoming connections?\n")
	prompt.WriteString("- [ ] TLS enabled for external access?\n\n")

	prompt.WriteString("### 4. Stream Exposure\n")
	prompt.WriteString("Run these checks:\n\n")
	prompt.WriteString("```javascript\n")
	prompt.WriteString("// MCP Tools to run:\n")
	prompt.WriteString("1. list_streams - Review all configured sources\n")
	prompt.WriteString("2. get_stream - Check each stream for exposed credentials\n")
	prompt.WriteString("3. get_config - Review configuration for sensitive data\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("### 5. Credential Audit\n")
	prompt.WriteString("Check all sources for:\n")
	prompt.WriteString("- URLs containing `user:pass@host`\n")
	prompt.WriteString("- Default passwords changed?\n")
	prompt.WriteString("- Unique credentials per source?\n\n")

	prompt.WriteString("### 6. Audit Actions\n\n")
	prompt.WriteString("1. Review all streams: `list_streams {\"details\": true}`\n")
	prompt.WriteString("2. Check for exposed passwords in source URLs\n")
	prompt.WriteString("3. Verify MCP server is not publicly accessible\n")
	prompt.WriteString("4. Check log for unauthorized access attempts\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleBackupRestorePrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	var prompt strings.Builder
	prompt.WriteString("# go2rtc Backup & Restore\n\n")

	prompt.WriteString("## Export Configuration\n\n")
	prompt.WriteString("To backup your go2rtc configuration:\n\n")
	prompt.WriteString("1. Export all streams:\n")
	prompt.WriteString("   `list_streams {\"details\": true}` - Get complete stream info\n\n")
	prompt.WriteString("2. Export preloads:\n")
	prompt.WriteString("   `list_preloads` - Get all preload configurations\n\n")
	prompt.WriteString("3. Get full config:\n")
	prompt.WriteString("   Resource: `config://` - Read main configuration\n\n")

	prompt.WriteString("## Restore Configuration\n\n")
	prompt.WriteString("To restore streams from backup:\n\n")
	prompt.WriteString("For each stream in your backup:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("add_stream {\"name\": \"stream_name\", \"url\": \"source_url\"}\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("For each preload:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("add_preload {\"name\": \"stream_name\", \"query\": \"video&audio\"}\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("## Configuration File Backup\n\n")
	prompt.WriteString("Main config location depends on your install:\n")
	prompt.WriteString("- Linux: `/etc/go2rtc/go2rtc.yaml`\n")
	prompt.WriteString("- Docker: Mounted volume `/config/go2rtc.yaml`\n")
	prompt.WriteString("- Windows: Same directory as executable\n\n")

	prompt.WriteString("## Quick Backup Commands\n\n")
	prompt.WriteString("Using MCP tools:\n")
	prompt.WriteString("1. `list_streams` - Save output to backup file\n")
	prompt.WriteString("2. `list_preloads` - Save preload configuration\n")
	prompt.WriteString("3. `get_info` - Record go2rtc version\n")

	return singlePromptMessage(prompt.String()), nil
}

func handleMigrationPrompt(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
	platform, _ := getStringArg(args, "platform", false)

	var prompt strings.Builder
	prompt.WriteString("# go2rtc Migration Guide\n\n")

	switch platform {
	case "rtsp2mp4":
		prompt.WriteString("## Migrating from Direct RTSP to go2rtc\n\n")
		prompt.WriteString("### Old URL:\n")
		prompt.WriteString("```\n")
		prompt.WriteString("rtsp://camera-ip:554/stream\n")
		prompt.WriteString("```\n\n")
		prompt.WriteString("### Migration Steps:\n")
		prompt.WriteString("1. Add source to go2rtc:\n")
		prompt.WriteString("   `add_stream {\"name\": \"camera\", \"url\": \"rtsp://camera-ip:554/stream\"}`\n\n")
		prompt.WriteString("2. Get new URLs:\n")
		prompt.WriteString("   `get_stream_urls {\"name\": \"camera\"}`\n\n")
		prompt.WriteString("3. Update client to use go2rtc URL:\n")
		prompt.WriteString("   `http://go2rtc-host:1984/mp4/camera.mp4`\n")
	case "ffmpeg":
		prompt.WriteString("## Migrating from FFmpeg Direct to go2rtc\n\n")
		prompt.WriteString("### Old FFmpeg command:\n")
		prompt.WriteString("```bash\n")
		prompt.WriteString("ffmpeg -i rtsp://camera/stream -f flv rtmp://server/live\n")
		prompt.WriteString("```\n\n")
		prompt.WriteString("### Migration Steps:\n")
		prompt.WriteString("1. Add RTSP source to go2rtc\n")
		prompt.WriteString("2. Use go2rtc's FFmpeg source if needed:\n")
		prompt.WriteString("   `ffmpeg:rtsp://camera/stream#video`\n")
		prompt.WriteString("3. Use `publish_stream` to send to RTMP:\n")
		prompt.WriteString("   `publish_stream {\"name\": \"camera\", \"destination\": \"rtmp://...\"}`\n")
	case "mqtt2go2rtc":
		prompt.WriteString("## Migrating from MQTT Camera to go2rtc\n\n")
		prompt.WriteString("### Before: MQTT URL in Home Assistant\n")
		prompt.WriteString("### After:\n")
		prompt.WriteString("1. Add camera to go2rtc\n")
		prompt.WriteString("2. Use go2rtc URL in Home Assistant config\n")
		prompt.WriteString("3. Remove old MQTT camera configuration\n")
	default:
		prompt.WriteString("## Migration Scenarios\n\n")
		prompt.WriteString("Select a source:\n")
		prompt.WriteString("- `rtsp2mp4` - Direct RTSP to go2rtc\n")
		prompt.WriteString("- `ffmpeg` - FFmpeg processes to go2rtc\n")
		prompt.WriteString("- `mqtt2go2rtc` - MQTT cameras to go2rtc\n\n")
		prompt.WriteString("## General Migration Process\n\n")
		prompt.WriteString("1. **Document current setup**\n")
		prompt.WriteString("   - List all camera URLs\n")
		prompt.WriteString("   - Note authentication credentials\n")
		prompt.WriteString("   - Record all client connections\n\n")
		prompt.WriteString("2. **Add sources to go2rtc**\n")
		prompt.WriteString("   - `add_stream` for each camera\n")
		prompt.WriteString("   - `validate_source` to verify each\n\n")
		prompt.WriteString("3. **Get new URLs**\n")
		prompt.WriteString("   - `get_stream_urls` for each stream\n")
		prompt.WriteString("   - Choose appropriate protocol (HLS, MP4, WebRTC)\n\n")
		prompt.WriteString("4. **Update clients**\n")
		prompt.WriteString("   - Replace old URLs with go2rtc URLs\n")
		prompt.WriteString("   - `get_connections` to verify\n\n")
		prompt.WriteString("5. **Enable preloads** (optional)\n")
		prompt.WriteString("   - `add_preload` for critical streams\n")
	}

	return singlePromptMessage(prompt.String()), nil
}

// Helper functions

func getStringArg(args map[string]any, name string, required bool) (string, error) {
	if args == nil {
		if required {
			return "", fmt.Errorf("%s is required", name)
		}
		return "", nil
	}

	value, ok := args[name]
	if !ok {
		if required {
			return "", fmt.Errorf("%s is required", name)
		}
		return "", nil
	}

	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}

	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return s, nil
}

func singlePromptMessage(text string) []PromptMessage {
	message := PromptMessage{Role: "user"}
	message.Content.Type = "text"
	message.Content.Text = text
	return []PromptMessage{message}
}

func getProducersInfo(stream *streams.Stream) []map[string]any {
	// Get producer information from stream
	if jsonBytes, err := json.Marshal(stream); err == nil {
		var info map[string]any
		if json.Unmarshal(jsonBytes, &info) == nil {
			if producers, ok := info["producers"].([]any); ok {
				result := make([]map[string]any, 0, len(producers))
				for _, p := range producers {
					if prod, ok := p.(map[string]any); ok {
						result = append(result, prod)
					}
				}
				return result
			}
		}
	}
	return nil
}

// Extended Tool Handlers

func handleGetStreamURLs(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	scheme, _ := getStringArg(args, "scheme", false)

	// Build available URLs for this stream
	result := map[string]any{
		"name":  name,
		"urls":  map[string]string{},
		"host":  app.Info["host"],
		"types": []string{},
	}

	// Common output formats
	host := fmt.Sprintf("%v", app.Info["host"])

	urls := map[string]string{
		"mp4":       fmt.Sprintf("%s/mp4/%s.mp4", host, name),
		"mp4_live":  fmt.Sprintf("%s/mp4/%s.live.mp4", host, name),
		"hls":       fmt.Sprintf("%s/hls/%s.m3u8", host, name),
		"webrtc":    fmt.Sprintf("%s/api/stream?src=%s", host, name),
		"mse":       fmt.Sprintf("%s/mse/%s", host, name),
		"rtsp":      fmt.Sprintf("rtsp://localhost:8554/%s", name),
		"rtmp":      fmt.Sprintf("rtmp://localhost:1935/%s", name),
		"flv":       fmt.Sprintf("%s/flv/%s.flv", host, name),
		"mjpeg":     fmt.Sprintf("%s/mjpeg/%s", host, name),
		"websocket": fmt.Sprintf("%s/api/ws?src=%s", host, name),
	}

	if scheme != "" {
		if url, ok := urls[scheme]; ok {
			return map[string]any{
				"name":   name,
				"scheme": scheme,
				"url":    url,
			}, nil
		}
		return nil, fmt.Errorf("unsupported scheme: %s", scheme)
	}

	result["urls"] = urls
	result["types"] = []string{"mp4", "mp4_live", "hls", "webrtc", "mse", "rtsp", "rtmp", "flv", "mjpeg", "websocket"}

	return result, nil
}

func handleGetStreamConsumers(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	// Get consumer info
	if jsonBytes, err := json.Marshal(stream); err == nil {
		var info map[string]any
		if json.Unmarshal(jsonBytes, &info) == nil {
			if consumers, ok := info["consumers"].([]any); ok {
				result := make([]map[string]any, 0, len(consumers))
				for _, c := range consumers {
					if consumer, ok := c.(map[string]any); ok {
						result = append(result, consumer)
					}
				}
				return map[string]any{
					"name":      name,
					"count":     len(result),
					"consumers": result,
				}, nil
			}
		}
	}

	return map[string]any{
		"name":      name,
		"count":     0,
		"consumers": []map[string]any{},
	}, nil
}

func handleRestartStream(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	// Stop and restart producers
	sources := stream.Sources()

	// Delete and recreate
	streams.Delete(name)

	var newStream *streams.Stream
	for _, source := range sources {
		newStream, err = streams.Patch(name, source)
		if err != nil {
			return nil, fmt.Errorf("failed to restart stream: %w", err)
		}
	}

	return map[string]any{
		"success": true,
		"name":    name,
		"sources": newStream.Sources(),
	}, nil
}

func handleGetEvents(ctx context.Context, args map[string]any) (any, error) {
	count := 10
	if c, ok := args["count"].(float64); ok {
		count = int(c)
	}

	events := eventLog.Get(count)

	result := make([]map[string]any, len(events))
	for i, event := range events {
		result[i] = map[string]any{
			"type":      event.Type,
			"timestamp": event.Timestamp,
			"data":      event.Data,
		}
	}

	return map[string]any{
		"count":  len(result),
		"events": result,
	}, nil
}

func handleGetConnections(ctx context.Context, args map[string]any) (any, error) {
	names := streams.GetAllNames()

	connections := make([]map[string]any, 0)

	for _, name := range names {
		stream := streams.Get(name)
		if stream == nil {
			continue
		}

		info := map[string]any{
			"name": name,
		}

		if jsonBytes, err := json.Marshal(stream); err == nil {
			var streamInfo map[string]any
			if json.Unmarshal(jsonBytes, &streamInfo) == nil {
				if producers, ok := streamInfo["producers"].([]any); ok {
					info["producers"] = len(producers)
				} else {
					info["producers"] = 0
				}

				if consumers, ok := streamInfo["consumers"].([]any); ok {
					info["consumers"] = len(consumers)
				} else {
					info["consumers"] = 0
				}
			}
		}

		connections = append(connections, info)
	}

	return map[string]any{
		"connections": connections,
	}, nil
}

func handleGetStreamStats(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	stats := map[string]any{
		"name":    name,
		"sources": stream.Sources(),
	}

	// Get detailed stream info including stats
	if jsonBytes, err := json.Marshal(stream); err == nil {
		var info map[string]any
		if json.Unmarshal(jsonBytes, &info) == nil {
			// Extract relevant stats
			if producers, ok := info["producers"].([]any); ok {
				producerStats := make([]map[string]any, 0)
				for _, p := range producers {
					if prod, ok := p.(map[string]any); ok {
						stat := map[string]any{
							"mode": prod["mode"],
						}
						if state, ok := prod["state"].(string); ok {
							stat["state"] = state
						}
						if url, ok := prod["url"].(string); ok {
							stat["url"] = url
						}
						producerStats = append(producerStats, stat)
					}
				}
				stats["producers"] = producerStats
			}

			if consumers, ok := info["consumers"].([]any); ok {
				stats["consumer_count"] = len(consumers)
			}
		}
	}

	return stats, nil
}

func handleStreamSnapshot(ctx context.Context, args map[string]any) (any, error) {
	name, err := getStringArg(args, "name", true)
	if err != nil {
		return nil, err
	}

	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("stream not found: %s", name)
	}

	// Return complete snapshot of stream state
	snapshot := map[string]any{
		"name":      name,
		"sources":   stream.Sources(),
		"timestamp": app.Info["time"],
	}

	if jsonBytes, err := json.Marshal(stream); err == nil {
		var info map[string]any
		if json.Unmarshal(jsonBytes, &info) == nil {
			snapshot["details"] = info
		}
	}

	return snapshot, nil
}
