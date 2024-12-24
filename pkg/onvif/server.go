package onvif

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

const (
	ActionGetCapabilities        = "GetCapabilities"
	ActionGetSystemDateAndTime   = "GetSystemDateAndTime"
	ActionGetNetworkInterfaces   = "GetNetworkInterfaces"
	ActionGetDeviceInformation   = "GetDeviceInformation"
	ActionGetServiceCapabilities = "GetServiceCapabilities"
	ActionGetProfiles            = "GetProfiles"
	ActionGetStreamUri           = "GetStreamUri"
	ActionGetSnapshotUri         = "GetSnapshotUri"
	ActionSystemReboot           = "SystemReboot"

	ActionGetServices                   = "GetServices"
	ActionGetScopes                     = "GetScopes"
	ActionGetVideoSources               = "GetVideoSources"
	ActionGetAudioSources               = "GetAudioSources"
	ActionGetVideoSourceConfigurations  = "GetVideoSourceConfigurations"
	ActionGetAudioSourceConfigurations  = "GetAudioSourceConfigurations"
	ActionGetVideoEncoderConfigurations = "GetVideoEncoderConfigurations"
	ActionGetAudioEncoderConfigurations = "GetAudioEncoderConfigurations"
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

func GetCapabilitiesResponse(host string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
	<s:Body>
		<tds:GetCapabilitiesResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
			<tds:Capabilities xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Device>
					<tt:XAddr>http://` + host + `/onvif/device_service</tt:XAddr>
				</tt:Device>
				<tt:Media>
					<tt:XAddr>http://` + host + `/onvif/media_service</tt:XAddr>
					<tt:StreamingCapabilities>
						<tt:RTPMulticast>false</tt:RTPMulticast>
						<tt:RTP_TCP>false</tt:RTP_TCP>
						<tt:RTP_RTSP_TCP>true</tt:RTP_RTSP_TCP>
					</tt:StreamingCapabilities>
				</tt:Media>
			</tds:Capabilities>
		</tds:GetCapabilitiesResponse>
	</s:Body>
</s:Envelope>`
}

func GetServicesResponse(host string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <tds:GetServicesResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
            <tds:Service>
                <tds:Namespace>http://www.onvif.org/ver10/device/wsdl</tds:Namespace>
                <tds:XAddr>http://` + host + `/onvif/device_service</tds:XAddr>
                <tds:Version>
                    <tds:Major>2</tds:Major>
                    <tds:Minor>5</tds:Minor>
                </tds:Version>
            </tds:Service>
            <tds:Service>
                <tds:Namespace>http://www.onvif.org/ver10/media/wsdl</tds:Namespace>
                <tds:XAddr>http://` + host + `/onvif/media_service</tds:XAddr>
                <tds:Version>
                    <tds:Major>2</tds:Major>
                    <tds:Minor>5</tds:Minor>
                </tds:Version>
            </tds:Service>
        </tds:GetServicesResponse>
    </s:Body>
</s:Envelope>`
}

func GetSystemDateAndTimeResponse() string {
	loc := time.Now()
	utc := loc.UTC()

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <tds:GetSystemDateAndTimeResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
            <tds:SystemDateAndTime xmlns:tt="http://www.onvif.org/ver10/schema">
                <tt:DateTimeType>NTP</tt:DateTimeType>
                <tt:DaylightSavings>false</tt:DaylightSavings>
                <tt:TimeZone>
                    <tt:TZ>GMT%s</tt:TZ>
                </tt:TimeZone>
                <tt:UTCDateTime>
                    <tt:Time>
                        <tt:Hour>%d</tt:Hour>
                        <tt:Minute>%d</tt:Minute>
                        <tt:Second>%d</tt:Second>
                    </tt:Time>
                    <tt:Date>
                        <tt:Year>%d</tt:Year>
                        <tt:Month>%d</tt:Month>
                        <tt:Day>%d</tt:Day>
                    </tt:Date>
                </tt:UTCDateTime>
                <tt:LocalDateTime>
                    <tt:Time>
                        <tt:Hour>%d</tt:Hour>
                        <tt:Minute>%d</tt:Minute>
                        <tt:Second>%d</tt:Second>
                    </tt:Time>
                    <tt:Date>
                        <tt:Year>%d</tt:Year>
                        <tt:Month>%d</tt:Month>
                        <tt:Day>%d</tt:Day>
                    </tt:Date>
                </tt:LocalDateTime>
            </tds:SystemDateAndTime>
        </tds:GetSystemDateAndTimeResponse>
    </s:Body>
</s:Envelope>`,
		loc.Format("-07:00"),
		utc.Hour(), utc.Minute(), utc.Second(), utc.Year(), utc.Month(), utc.Day(),
		loc.Hour(), loc.Minute(), loc.Second(), loc.Year(), loc.Month(), loc.Day(),
	)
}

func GetNetworkInterfacesResponse() string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <tds:GetNetworkInterfacesResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
    </s:Body>
</s:Envelope>`
}

func GetDeviceInformationResponse(manuf, model, firmware, serial string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <tds:GetDeviceInformationResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
            <tds:Manufacturer>` + manuf + `</tds:Manufacturer>
            <tds:Model>` + model + `</tds:Model>
            <tds:FirmwareVersion>` + firmware + `</tds:FirmwareVersion>
            <tds:SerialNumber>` + serial + `</tds:SerialNumber>
            <tds:HardwareId>1.00</tds:HardwareId>
        </tds:GetDeviceInformationResponse>
    </s:Body>
</s:Envelope>`
}

func GetServiceCapabilitiesResponse() string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <trt:GetServiceCapabilitiesResponse xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
            <trt:Capabilities SnapshotUri="true" Rotation="false" VideoSourceMode="false" OSD="false" TemporaryOSDText="false" EXICompression="false">
                <trt:StreamingCapabilities RTPMulticast="false" RTP_TCP="false" RTP_RTSP_TCP="true" NonAggregateControl="false" NoRTSPStreaming="false" />
            </trt:Capabilities>
        </trt:GetServiceCapabilitiesResponse>
    </s:Body>
</s:Envelope>`
}

func SystemRebootResponse() string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <tds:SystemRebootResponse xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
            <tds:Message>system reboot in 1 second...</tds:Message>
        </tds:SystemRebootResponse>
    </s:Body>
</s:Envelope>`
}

func GetProfilesResponse(names []string) string {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <trt:GetProfilesResponse xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">`)

	for i, name := range names {
		buf.WriteString(`
			<trt:Profiles token="` + name + `" fixed="true">
				<trt:Name>` + name + `</trt:Name>
				<trt:VideoEncoderConfiguration token="` + strconv.Itoa(i) + `">
                    <trt:Name>` + name + `</trt:Name>
					<trt:Encoding>H264</trt:Encoding>
					<trt:Resolution>
						<trt:Width>1920</trt:Width>
                        <trt:Height>1080</trt:Height>
                    </trt:Resolution>
					<trt:RateControl>
                        <trt:FrameRateLimit>29.97003</trt:FrameRateLimit>
                        <trt:EncodingInterval>1</trt:EncodingInterval>
                        <trt:BitrateLimit>5000</trt:BitrateLimit>
                    </trt:RateControl>
					<trt:Quality>4</trt:Quality>
                    <trt:SessionTimeout>PT1000S</trt:SessionTimeout>
				</trt:VideoEncoderConfiguration>
                <trt:VideoSourceConfiguration token="` + strconv.Itoa(i) + `">
                    <trt:Name>` + name + `</trt:Name>
                    <trt:SourceToken>` + strconv.Itoa(i) + `</trt:SourceToken>
                    <trt:Bounds x="0" y="0" width="1920" height="1080"></trt:Bounds>
                </trt:VideoSourceConfiguration>
			</trt:Profiles>`)
	}

	buf.WriteString(`
		</trt:GetProfilesResponse>
	</s:Body>
</s:Envelope>`)

	return buf.String()
}


func GetVideoSourcesResponse(names []string) string {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <trt:GetVideoSourcesResponse xmlns:trt="http://www.onvif.org/ver10/media/wsdl">`)

	for i, _ := range names {
		buf.WriteString(`
			<trt:VideoSources token="` + strconv.Itoa(i) + `">
				<trt:Framerate>29.97003</trt:Framerate>
                <trt:Resolution>
                    <trt:Width>1920</trt:Width>
                    <trt:Height>1080</trt:Height>
                </trt:Resolution>
            </trt:VideoSources>`)
	}

	buf.WriteString(`
		</trt:GetVideoSourcesResponse >
	</s:Body>
</s:Envelope>`)

	return buf.String()
}

func GetStreamUriResponse(uri string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <trt:GetStreamUriResponse xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
            <trt:MediaUri>
                <trt:Uri>` + uri + `</trt:Uri>
            </trt:MediaUri>
        </trt:GetStreamUriResponse>
    </s:Body>
</s:Envelope>`
}

func GetSnapshotUriResponse(uri string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <trt:GetSnapshotUriResponse xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
            <trt:MediaUri>
                <trt:Uri>` + uri + `</trt:Uri>
            </trt:MediaUri>
        </trt:GetSnapshotUriResponse>
    </s:Body>
</s:Envelope>`
}
