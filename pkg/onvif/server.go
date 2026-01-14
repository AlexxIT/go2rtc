package onvif

import (
	"bytes"
	"regexp"
	"time"
    "strconv"
    "strings"
)

type OnvifProfile struct {
    Name    string   `yaml:"name"`
    Streams []string `yaml:"streams"`
}

const ServiceGetServiceCapabilities = "GetServiceCapabilities"

const (
	DeviceGetCapabilities          = "GetCapabilities"
	DeviceGetDeviceInformation     = "GetDeviceInformation"
	DeviceGetDiscoveryMode         = "GetDiscoveryMode"
	DeviceGetDNS                   = "GetDNS"
	DeviceGetHostname              = "GetHostname"
	DeviceGetNetworkDefaultGateway = "GetNetworkDefaultGateway"
	DeviceGetNetworkInterfaces     = "GetNetworkInterfaces"
	DeviceGetNetworkProtocols      = "GetNetworkProtocols"
	DeviceGetNTP                   = "GetNTP"
	DeviceGetOSDs                  = "GetOSDs"
	DeviceGetOSDOptions            = "GetOSDOptions"
	DeviceGetScopes                = "GetScopes"
	DeviceGetServices              = "GetServices"
	DeviceGetSystemDateAndTime     = "GetSystemDateAndTime"
	DeviceSystemReboot             = "SystemReboot"
)

const (
	MediaGetAudioEncoderConfigurations = "GetAudioEncoderConfigurations"
	MediaGetAudioSources               = "GetAudioSources"
	MediaGetAudioSourceConfigurations  = "GetAudioSourceConfigurations"
	MediaGetProfile                    = "GetProfile"
	MediaGetProfiles                   = "GetProfiles"
	MediaGetSnapshotUri                = "GetSnapshotUri"
	MediaGetStreamUri                  = "GetStreamUri"
	MediaGetVideoEncoderConfigurations = "GetVideoEncoderConfigurations"
	MediaGetVideoSources               = "GetVideoSources"
	MediaGetVideoSourceConfiguration   = "GetVideoSourceConfiguration"
	MediaGetVideoSourceConfigurations  = "GetVideoSourceConfigurations"
)

func GetRequestAction(b []byte) string {
	// <soap-env:Body><ns0:GetCapabilities xmlns:ns0="http://www.onvif.org/ver10/device/wsdl">
	// <v:Body><GetSystemDateAndTime xmlns="http://www.onvif.org/ver10/device/wsdl" /></v:Body>
	re := regexp.MustCompile(`Body[^<]+<([^ />]+)`)
	m := re.FindSubmatch(b)
	if len(m) != 2 {
		return ""
	}
	if i := bytes.IndexByte(m[1], ':'); i > 0 {
		return string(m[1][i+1:])
	}
	return string(m[1])
}

func GetCapabilitiesResponse(host string) []byte {
	e := NewEnvelope()
	e.Append(`<tds:GetCapabilitiesResponse>
	<tds:Capabilities>
		<tt:Device>
			<tt:XAddr>http://`, host, `/onvif/device_service</tt:XAddr>
		</tt:Device>
		<tt:Media>
			<tt:XAddr>http://`, host, `/onvif/media_service</tt:XAddr>
			<tt:StreamingCapabilities>
				<tt:RTPMulticast>false</tt:RTPMulticast>
				<tt:RTP_TCP>false</tt:RTP_TCP>
				<tt:RTP_RTSP_TCP>true</tt:RTP_RTSP_TCP>
			</tt:StreamingCapabilities>
		</tt:Media>
	</tds:Capabilities>
</tds:GetCapabilitiesResponse>`)
	return e.Bytes()
}

func GetServicesResponse(host string) []byte {
	e := NewEnvelope()
	e.Append(`<tds:GetServicesResponse>
	<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver10/device/wsdl</tds:Namespace>
		<tds:XAddr>http://`, host, `/onvif/device_service</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>
	<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver10/media/wsdl</tds:Namespace>
		<tds:XAddr>http://`, host, `/onvif/media_service</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>
</tds:GetServicesResponse>`)
	return e.Bytes()
}

func GetSystemDateAndTimeResponse() []byte {
	loc := time.Now()
	utc := loc.UTC()

	e := NewEnvelope()
	e.Appendf(`<tds:GetSystemDateAndTimeResponse>
	<tds:SystemDateAndTime>
		<tt:DateTimeType>NTP</tt:DateTimeType>
		<tt:DaylightSavings>true</tt:DaylightSavings>
		<tt:TimeZone>
			<tt:TZ>%s</tt:TZ>
		</tt:TimeZone>
		<tt:UTCDateTime>
			<tt:Time><tt:Hour>%d</tt:Hour><tt:Minute>%d</tt:Minute><tt:Second>%d</tt:Second></tt:Time>
			<tt:Date><tt:Year>%d</tt:Year><tt:Month>%d</tt:Month><tt:Day>%d</tt:Day></tt:Date>
		</tt:UTCDateTime>
		<tt:LocalDateTime>
			<tt:Time><tt:Hour>%d</tt:Hour><tt:Minute>%d</tt:Minute><tt:Second>%d</tt:Second></tt:Time>
			<tt:Date><tt:Year>%d</tt:Year><tt:Month>%d</tt:Month><tt:Day>%d</tt:Day></tt:Date>
		</tt:LocalDateTime>
	</tds:SystemDateAndTime>
</tds:GetSystemDateAndTimeResponse>`,
		GetPosixTZ(loc),
		utc.Hour(), utc.Minute(), utc.Second(), utc.Year(), utc.Month(), utc.Day(),
		loc.Hour(), loc.Minute(), loc.Second(), loc.Year(), loc.Month(), loc.Day(),
	)
	return e.Bytes()
}

func GetDeviceInformationResponse(manuf, model, firmware, serial string) []byte {
	e := NewEnvelope()
	e.Append(`<tds:GetDeviceInformationResponse>
	<tds:Manufacturer>`, manuf, `</tds:Manufacturer>
	<tds:Model>`, model, `</tds:Model>
	<tds:FirmwareVersion>`, firmware, `</tds:FirmwareVersion>
	<tds:SerialNumber>`, serial, `</tds:SerialNumber>
	<tds:HardwareId>1.00</tds:HardwareId>
</tds:GetDeviceInformationResponse>`)
	return e.Bytes()
}

func GetMediaServiceCapabilitiesResponse() []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetServiceCapabilitiesResponse>
	<trt:Capabilities SnapshotUri="true" Rotation="false" VideoSourceMode="false" OSD="false" TemporaryOSDText="false" EXICompression="false">
		<trt:StreamingCapabilities RTPMulticast="false" RTP_TCP="false" RTP_RTSP_TCP="true" NonAggregateControl="false" NoRTSPStreaming="false" />
	</trt:Capabilities>
</trt:GetServiceCapabilitiesResponse>`)
	return e.Bytes()
}

func GetProfilesResponse(OnvifProfiles []OnvifProfile) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetProfilesResponse>
`)
	for _, cam := range OnvifProfiles {
		appendProfile(e, "Profiles", cam)
	}
	e.Append(`</trt:GetProfilesResponse>`)
	return e.Bytes()
}

func GetProfileResponse(cam OnvifProfile) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetProfileResponse>
`)
	appendProfile(e, "Profile", cam)
	e.Append(`</trt:GetProfileResponse>`)
	return e.Bytes()
}

// Parsing stream name to: name, width, height, codec
func ParseStream(stream string) (string, int, int, string) {
    parts := strings.Split(stream, "#")
    name := parts[0]
    width, height := 1920, 1080 // default resolution
    codec := "H264" // default codec

    resRegex := regexp.MustCompile(`res=(\d+)x(\d+)`)
    codecRegex := regexp.MustCompile(`codec=([a-zA-Z0-9]+)`)

    for _, part := range parts[1:] {
        if matches := resRegex.FindStringSubmatch(part); len(matches) == 3 {
            width, _ = strconv.Atoi(matches[1])
            height, _ = strconv.Atoi(matches[2])
        }
        if matches := codecRegex.FindStringSubmatch(part); len(matches) == 2 {
            codec = matches[1]
        }
    }

    return name, width, height, codec
}

func appendProfile(e *Envelope, tag string, profile OnvifProfile) {
    if len(profile.Streams) == 0 {
        return
    }

    // get first stream as main stream
    firstStream := profile.Streams[0]
    firstName, firstWidth, firstHeight, _ := ParseStream(firstStream)

    for _, stream := range profile.Streams {
        streamName, width, height, codec := ParseStream(stream)

        e.Append(`<trt:`, tag, ` token="`, streamName, `" fixed="true">
        <tt:Name>`, streamName, `</tt:Name>
        <tt:VideoSourceConfiguration token="`, firstName, `">
            <tt:Name>VSC</tt:Name>
            <tt:SourceToken>`, firstName, `</tt:SourceToken>
            <tt:Bounds x="0" y="0" width="`, strconv.Itoa(firstWidth), `" height="`, strconv.Itoa(firstHeight), `"></tt:Bounds>
        </tt:VideoSourceConfiguration>
        <tt:VideoEncoderConfiguration token="`, streamName, `">
            <tt:Name>SubStream</tt:Name>
            <tt:Encoding>`, codec, `</tt:Encoding>
            <tt:Resolution><tt:Width>`, strconv.Itoa(width), `</tt:Width><tt:Height>`, strconv.Itoa(height), `</tt:Height></tt:Resolution>
            <tt:RateControl />
        </tt:VideoEncoderConfiguration>
        </trt:`, tag, `>
        `)
    }
}

func GetVideoSourceConfigurationsResponse(OnvifProfiles []OnvifProfile) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourceConfigurationsResponse>
`)
	for _, cam := range OnvifProfiles {
		appendProfile(e, "Configurations", cam)
	}
	e.Append(`</trt:GetVideoSourceConfigurationsResponse>`)
	return e.Bytes()
}

func GetVideoSourceConfigurationResponse(name string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourceConfigurationResponse>
`)
	appendVideoSourceConfiguration(e, "Configuration", name)
	e.Append(`</trt:GetVideoSourceConfigurationResponse>`)
	return e.Bytes()
}

func appendVideoSourceConfiguration(e *Envelope, tag, name string) {
	e.Append(`<trt:`, tag, ` token="`, name, `" fixed="true">
	<tt:Name>VSC</tt:Name>
	<tt:SourceToken>`, name, `</tt:SourceToken>
	<tt:Bounds x="0" y="0" width="1920" height="1080"></tt:Bounds>
</trt:`, tag, `>
`)
}

func GetVideoSourcesResponse(names []string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourcesResponse>
`)
	for _, name := range names {
		e.Append(`<trt:VideoSources token="`, name, `">
	<tt:Framerate>30.000000</tt:Framerate>
	<tt:Resolution><tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:Resolution>
</trt:VideoSources>
`)
	}
	e.Append(`</trt:GetVideoSourcesResponse>`)
	return e.Bytes()
}

func GetStreamUriResponse(uri string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetStreamUriResponse><trt:MediaUri><tt:Uri>`, uri, `</tt:Uri></trt:MediaUri></trt:GetStreamUriResponse>`)
	return e.Bytes()
}

func GetSnapshotUriResponse(uri string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetSnapshotUriResponse><trt:MediaUri><tt:Uri>`, uri, `</tt:Uri></trt:MediaUri></trt:GetSnapshotUriResponse>`)
	return e.Bytes()
}

func GetOSDOptionsResponse() []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetOSDOptionsResponse>
	<trt:OSDOptions>
		<tt:MaximumNumberOfOSDs Total="1" Image="0" PlainText="1" Date="0" Time="0" DateAndTime="0"/>
		<tt:Type>Text</tt:Type>
		<tt:PositionOption>Custom</tt:PositionOption>
		<tt:TextOption>
			<tt:Type>Plain</tt:Type>
		</tt:TextOption>
	</trt:OSDOptions>
</trt:GetOSDOptionsResponse>`)
	return e.Bytes()
}

func GetOSDsResponse(configurationToken string, cameraName string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetOSDsResponse>
	<trt:OSDs token="OSD00000">
		<tt:VideoSourceConfigurationToken>`, configurationToken, `</tt:VideoSourceConfigurationToken>
		<tt:Type>Text</tt:Type>
		<tt:Position>
			<tt:Type>Custom</tt:Type>
			<tt:Pos x="0" y="0"/>
		</tt:Position>
		<tt:TextString>
			<tt:Type>Plain</tt:Type>
			<tt:PlainText>`, cameraName, `</tt:PlainText>
		</tt:TextString>
	</trt:OSDs>
</trt:GetOSDsResponse>`)
	return e.Bytes()
}

func StaticResponse(operation string) []byte {
	switch operation {
	case DeviceGetSystemDateAndTime:
		return GetSystemDateAndTimeResponse()
	}

	e := NewEnvelope()
	e.Append(responses[operation])
	return e.Bytes()
}

var responses = map[string]string{
	DeviceGetDiscoveryMode:         `<tds:GetDiscoveryModeResponse><tds:DiscoveryMode>Discoverable</tds:DiscoveryMode></tds:GetDiscoveryModeResponse>`,
	DeviceGetDNS:                   `<tds:GetDNSResponse><tds:DNSInformation /></tds:GetDNSResponse>`,
	DeviceGetHostname:              `<tds:GetHostnameResponse><tds:HostnameInformation /></tds:GetHostnameResponse>`,
	DeviceGetNetworkDefaultGateway: `<tds:GetNetworkDefaultGatewayResponse><tds:NetworkGateway /></tds:GetNetworkDefaultGatewayResponse>`,
	DeviceGetNTP:                   `<tds:GetNTPResponse><tds:NTPInformation /></tds:GetNTPResponse>`,
	DeviceSystemReboot:             `<tds:SystemRebootResponse><tds:Message>OK</tds:Message></tds:SystemRebootResponse>`,

	DeviceGetNetworkInterfaces: `<tds:GetNetworkInterfacesResponse />`,
	DeviceGetNetworkProtocols:  `<tds:GetNetworkProtocolsResponse />`,
	DeviceGetScopes: `<tds:GetScopesResponse>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/name/go2rtc</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/location/github</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/Profile/Streaming</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/type/Network_Video_Transmitter</tt:ScopeItem></tds:Scopes>
</tds:GetScopesResponse>`,

	MediaGetVideoEncoderConfigurations: `<trt:GetVideoEncoderConfigurationsResponse>
	<tt:VideoEncoderConfiguration token="vec">
		<tt:Name>VEC</tt:Name>
		<tt:Encoding>H264</tt:Encoding>
		<tt:Resolution><tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:Resolution>
		<tt:RateControl />
	</tt:VideoEncoderConfiguration>
</trt:GetVideoEncoderConfigurationsResponse>`,

	MediaGetAudioEncoderConfigurations: `<trt:GetAudioEncoderConfigurationsResponse />`,
	MediaGetAudioSources:               `<trt:GetAudioSourcesResponse />`,
	MediaGetAudioSourceConfigurations:  `<trt:GetAudioSourceConfigurationsResponse />`,
}
