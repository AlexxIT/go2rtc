package onvif

import "time"

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Minimal ONVIF PTZ service: just enough for an NVR (Frigate) to show pan/tilt
// arrows for streams whose source can move, and to send ContinuousMove / Stop.
// Only continuous pan/tilt in the generic velocity space is advertised: no zoom,
// no relative/absolute moves, no presets.

const PathPTZ = "/onvif/ptz_service"

const (
	PTZGetConfigurationOptions = "GetConfigurationOptions"
	PTZGetConfigurations       = "GetConfigurations"
	PTZGetConfiguration        = "GetConfiguration"
	PTZGetNodes                = "GetNodes"
	PTZGetNode                 = "GetNode"
	PTZGetPresets              = "GetPresets"
	PTZGetStatus               = "GetStatus"
	PTZContinuousMove          = "ContinuousMove"
	PTZStop                    = "Stop"
)

const spaceVelocityGeneric = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocityGenericSpace"

// PTZ reports whether the stream `name` can pan/tilt; it decides which profiles
// carry a PTZ configuration. PTZAny reports whether any stream currently can;
// it decides whether the PTZ service is advertised at all (GetCapabilities,
// GetServices). Both nil means no PTZ.
var (
	PTZ    func(name string) bool
	PTZAny func() bool
)

func hasPTZ(name string) bool {
	return PTZ != nil && PTZ(name)
}

func hasAnyPTZ() bool {
	return PTZAny != nil && PTZAny()
}

func appendPTZConfiguration(e *Envelope, tag, name string) {
	e.Appendf(`<tt:%s token="%s">
	<tt:Name>PTZ</tt:Name>
	<tt:UseCount>1</tt:UseCount>
	<tt:NodeToken>%s</tt:NodeToken>
	<tt:DefaultContinuousPanTiltVelocitySpace>%s</tt:DefaultContinuousPanTiltVelocitySpace>
	<tt:DefaultPTZTimeout>PT10S</tt:DefaultPTZTimeout>
</tt:%s>`, tag, name, name, spaceVelocityGeneric, tag)
}

func GetPTZConfigurationOptionsResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<tptz:GetConfigurationOptionsResponse>
	<tptz:PTZConfigurationOptions>
		<tt:Spaces>
			<tt:ContinuousPanTiltVelocitySpace>
				<tt:URI>%s</tt:URI>
				<tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange>
				<tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange>
			</tt:ContinuousPanTiltVelocitySpace>
		</tt:Spaces>
		<tt:PTZTimeout><tt:Min>PT1S</tt:Min><tt:Max>PT10S</tt:Max></tt:PTZTimeout>
	</tptz:PTZConfigurationOptions>
</tptz:GetConfigurationOptionsResponse>`, spaceVelocityGeneric)
	return e.Bytes()
}

func GetPTZConfigurationsResponse(names []string) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetConfigurationsResponse>`)
	for _, name := range names {
		if hasPTZ(name) {
			appendPTZConfiguration(e, "PTZConfiguration", name)
		}
	}
	e.Append(`</tptz:GetConfigurationsResponse>`)
	return e.Bytes()
}

func GetPTZConfigurationResponse(name string) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetConfigurationResponse>`)
	appendPTZConfiguration(e, "PTZConfiguration", name)
	e.Append(`</tptz:GetConfigurationResponse>`)
	return e.Bytes()
}

func appendPTZNode(e *Envelope, tag, name string) {
	e.Appendf(`<tptz:%s token="%s" FixedHomePosition="false">
	<tt:Name>%s</tt:Name>
	<tt:SupportedPTZSpaces>
		<tt:ContinuousPanTiltVelocitySpace>
			<tt:URI>%s</tt:URI>
			<tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange>
			<tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange>
		</tt:ContinuousPanTiltVelocitySpace>
	</tt:SupportedPTZSpaces>
	<tt:MaximumNumberOfPresets>0</tt:MaximumNumberOfPresets>
	<tt:HomeSupported>false</tt:HomeSupported>
</tptz:%s>`, tag, name, name, spaceVelocityGeneric, tag)
}

func GetPTZNodesResponse(names []string) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetNodesResponse>`)
	for _, name := range names {
		if hasPTZ(name) {
			appendPTZNode(e, "PTZNode", name)
		}
	}
	e.Append(`</tptz:GetNodesResponse>`)
	return e.Bytes()
}

func GetPTZNodeResponse(name string) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetNodeResponse>`)
	appendPTZNode(e, "PTZNode", name)
	e.Append(`</tptz:GetNodeResponse>`)
	return e.Bytes()
}

func GetPTZStatusResponse(moving bool) []byte {
	state := "IDLE"
	if moving {
		state = "MOVING"
	}
	e := NewEnvelope()
	e.Appendf(`<tptz:GetStatusResponse>
	<tptz:PTZStatus>
		<tt:MoveStatus><tt:PanTilt>%s</tt:PanTilt></tt:MoveStatus>
		<tt:UtcTime>%s</tt:UtcTime>
	</tptz:PTZStatus>
</tptz:GetStatusResponse>`, state, nowUTC())
	return e.Bytes()
}

var ptzEmptyResponses = map[string]string{
	PTZGetPresets:     `<tptz:GetPresetsResponse />`,
	PTZContinuousMove: `<tptz:ContinuousMoveResponse />`,
	PTZStop:           `<tptz:StopResponse />`,
}

func PTZStaticResponse(operation string) []byte {
	e := NewEnvelope()
	e.Append(ptzEmptyResponses[operation])
	return e.Bytes()
}
