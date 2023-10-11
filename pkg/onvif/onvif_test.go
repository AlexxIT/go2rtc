package onvif

import (
	"html"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetStreamUri(t *testing.T) {
	tests := []struct {
		name string
		xml  string
		url  string
	}{
		{
			name: "Dahua stream default",
			xml:  `<?xml version="1.0" encoding="utf-8" standalone="yes" ?><s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding" xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:trt="http://www.onvif.org/ver10/media/wsdl"><s:Header/><s:Body><trt:GetStreamUriResponse><trt:MediaUri><tt:Uri>rtsp://192.168.1.123:554/cam/realmonitor?channel=1&amp;subtype=1&amp;unicast=true&amp;proto=Onvif</tt:Uri><tt:InvalidAfterConnect>true</tt:InvalidAfterConnect><tt:InvalidAfterReboot>true</tt:InvalidAfterReboot><tt:Timeout>PT0S</tt:Timeout></trt:MediaUri></trt:GetStreamUriResponse></s:Body></s:Envelope>`,
			url:  "rtsp://192.168.1.123:554/cam/realmonitor?channel=1&subtype=1&unicast=true&proto=Onvif",
		},
		{
			name: "Dahua snapshot default",
			xml:  `<?xml version="1.0" encoding="utf-8" standalone="yes" ?><s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding" xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:trt="http://www.onvif.org/ver10/media/wsdl"><s:Header/><s:Body><trt:GetSnapshotUriResponse><trt:MediaUri><tt:Uri>http://192.168.1.123/onvifsnapshot/media_service/snapshot?channel=1&amp;subtype=1</tt:Uri><tt:InvalidAfterConnect>false</tt:InvalidAfterConnect><tt:InvalidAfterReboot>false</tt:InvalidAfterReboot><tt:Timeout>PT0S</tt:Timeout></trt:MediaUri></trt:GetSnapshotUriResponse></s:Body></s:Envelope>`,
			url:  "http://192.168.1.123/onvifsnapshot/media_service/snapshot?channel=1&subtype=1",
		},
		{
			name: "Dahua stream formatted",
			xml: `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding"
    xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema"
    xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
    <s:Header />
    <s:Body>
        <trt:GetStreamUriResponse>
            <trt:MediaUri>
                <tt:Uri>
                    rtsp://192.168.1.123:554/cam/realmonitor?channel=1&amp;subtype=1&amp;unicast=true&amp;proto=Onvif</tt:Uri>
                <tt:InvalidAfterConnect>true</tt:InvalidAfterConnect>
                <tt:InvalidAfterReboot>true</tt:InvalidAfterReboot>
                <tt:Timeout>PT0S</tt:Timeout>
            </trt:MediaUri>
        </trt:GetStreamUriResponse>
    </s:Body>
</s:Envelope>`,
			url: "rtsp://192.168.1.123:554/cam/realmonitor?channel=1&subtype=1&unicast=true&proto=Onvif",
		},
		{
			name: "Dahua snapshot formatted",
			xml: `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding"
    xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema"
    xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
    <s:Header />
    <s:Body>
        <trt:GetSnapshotUriResponse>
            <trt:MediaUri>
                <tt:Uri>
                    http://192.168.1.123/onvifsnapshot/media_service/snapshot?channel=1&amp;subtype=1</tt:Uri>
                <tt:InvalidAfterConnect>false</tt:InvalidAfterConnect>
                <tt:InvalidAfterReboot>false</tt:InvalidAfterReboot>
                <tt:Timeout>PT0S</tt:Timeout>
            </trt:MediaUri>
        </trt:GetSnapshotUriResponse>
    </s:Body>
</s:Envelope>`,
			url: "http://192.168.1.123/onvifsnapshot/media_service/snapshot?channel=1&subtype=1",
		},
		{
			name: "Unknown",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope ...>
   <SOAP-ENV:Header></SOAP-ENV:Header>
   <SOAP-ENV:Body>
	   <MC1:GetStreamUriResponse>
		   <MC1:MediaUri>
			   <MC2:Uri>
					rtsp://192.168.5.53:8090/profile1=r
				</MC2:Uri>
		   </MC1:MediaUri>
	   </MC1:GetStreamUriResponse>
   </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`,
			url: "rtsp://192.168.5.53:8090/profile1=r",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uri := FindTagValue([]byte(test.xml), "Uri")
			uri = strings.TrimSpace(html.UnescapeString(uri))
			u, err := url.Parse(uri)
			require.Nil(t, err)
			require.Equal(t, test.url, u.String())
		})
	}
}

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "Dahua default",
			xml:  `<?xml version="1.0" encoding="utf-8" standalone="yes" ?><s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding" xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tds="http://www.onvif.org/ver10/device/wsdl"><s:Header/><s:Body><tds:GetCapabilitiesResponse><tds:Capabilities><tt:Analytics><tt:XAddr>http://192.168.1.123/onvif/analytics_service</tt:XAddr><tt:RuleSupport>true</tt:RuleSupport><tt:AnalyticsModuleSupport>true</tt:AnalyticsModuleSupport></tt:Analytics><tt:Device><tt:XAddr>http://192.168.1.123/onvif/device_service</tt:XAddr><tt:Network><tt:IPFilter>false</tt:IPFilter><tt:ZeroConfiguration>false</tt:ZeroConfiguration><tt:IPVersion6>false</tt:IPVersion6><tt:DynDNS>false</tt:DynDNS><tt:Extension><tt:Dot11Configuration>false</tt:Dot11Configuration></tt:Extension></tt:Network><tt:System><tt:DiscoveryResolve>false</tt:DiscoveryResolve><tt:DiscoveryBye>true</tt:DiscoveryBye><tt:RemoteDiscovery>false</tt:RemoteDiscovery><tt:SystemBackup>false</tt:SystemBackup><tt:SystemLogging>true</tt:SystemLogging><tt:FirmwareUpgrade>true</tt:FirmwareUpgrade><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>00</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>10</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>20</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>30</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>40</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>2</tt:Major><tt:Minor>42</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>16</tt:Major><tt:Minor>12</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>18</tt:Major><tt:Minor>06</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>18</tt:Major><tt:Minor>12</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>19</tt:Major><tt:Minor>06</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>19</tt:Major><tt:Minor>12</tt:Minor></tt:SupportedVersions><tt:SupportedVersions><tt:Major>20</tt:Major><tt:Minor>06</tt:Minor></tt:SupportedVersions><tt:Extension><tt:HttpFirmwareUpgrade>true</tt:HttpFirmwareUpgrade><tt:HttpSystemBackup>false</tt:HttpSystemBackup><tt:HttpSystemLogging>false</tt:HttpSystemLogging><tt:HttpSupportInformation>false</tt:HttpSupportInformation></tt:Extension></tt:System><tt:IO><tt:InputConnectors>2</tt:InputConnectors><tt:RelayOutputs>1</tt:RelayOutputs><tt:Extension><tt:Auxiliary>false</tt:Auxiliary><tt:AuxiliaryCommands></tt:AuxiliaryCommands><tt:Extension></tt:Extension></tt:Extension></tt:IO><tt:Security><tt:TLS1.1>false</tt:TLS1.1><tt:TLS1.2>false</tt:TLS1.2><tt:OnboardKeyGeneration>false</tt:OnboardKeyGeneration><tt:AccessPolicyConfig>false</tt:AccessPolicyConfig><tt:X.509Token>false</tt:X.509Token><tt:SAMLToken>false</tt:SAMLToken><tt:KerberosToken>false</tt:KerberosToken><tt:RELToken>false</tt:RELToken><tt:Extension><tt:TLS1.0>false</tt:TLS1.0><tt:Extension><tt:Dot1X>false</tt:Dot1X><tt:SupportedEAPMethod>0</tt:SupportedEAPMethod><tt:RemoteUserHandling>false</tt:RemoteUserHandling></tt:Extension></tt:Extension></tt:Security></tt:Device><tt:Events><tt:XAddr>http://192.168.1.123/onvif/event_service</tt:XAddr><tt:WSSubscriptionPolicySupport>true</tt:WSSubscriptionPolicySupport><tt:WSPullPointSupport>true</tt:WSPullPointSupport><tt:WSPausableSubscriptionManagerInterfaceSupport>false</tt:WSPausableSubscriptionManagerInterfaceSupport></tt:Events><tt:Imaging><tt:XAddr>http://192.168.1.123/onvif/imaging_service</tt:XAddr></tt:Imaging><tt:Media><tt:XAddr>http://192.168.1.123/onvif/media_service</tt:XAddr><tt:StreamingCapabilities><tt:RTPMulticast>true</tt:RTPMulticast><tt:RTP_TCP>true</tt:RTP_TCP><tt:RTP_RTSP_TCP>true</tt:RTP_RTSP_TCP></tt:StreamingCapabilities><tt:Extension><tt:ProfileCapabilities><tt:MaximumNumberOfProfiles>6</tt:MaximumNumberOfProfiles></tt:ProfileCapabilities></tt:Extension></tt:Media><tt:Extension><tt:DeviceIO><tt:XAddr>http://192.168.1.123/onvif/deviceIO_service</tt:XAddr><tt:VideoSources>1</tt:VideoSources><tt:VideoOutputs>0</tt:VideoOutputs><tt:AudioSources>1</tt:AudioSources><tt:AudioOutputs>1</tt:AudioOutputs><tt:RelayOutputs>1</tt:RelayOutputs></tt:DeviceIO></tt:Extension></tds:Capabilities></tds:GetCapabilitiesResponse></s:Body></s:Envelope>`,
		},
		{
			name: "Dahua formatted",
			xml: `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<s:Envelope xmlns:sc="http://www.w3.org/2003/05/soap-encoding"
    xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema"
    xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <s:Header />
    <s:Body>
        <tds:GetCapabilitiesResponse>
            <tds:Capabilities>
                <tt:Analytics>
                    <tt:XAddr>http://192.168.1.123/onvif/analytics_service</tt:XAddr>
                    <tt:RuleSupport>true</tt:RuleSupport>
                    <tt:AnalyticsModuleSupport>true</tt:AnalyticsModuleSupport>
                </tt:Analytics>
                <tt:Device>
                    <tt:XAddr>http://192.168.1.123/onvif/device_service</tt:XAddr>
                    <tt:Network>
                        <tt:IPFilter>false</tt:IPFilter>
                        <tt:ZeroConfiguration>false</tt:ZeroConfiguration>
                        <tt:IPVersion6>false</tt:IPVersion6>
                        <tt:DynDNS>false</tt:DynDNS>
                        <tt:Extension>
                            <tt:Dot11Configuration>false</tt:Dot11Configuration>
                        </tt:Extension>
                    </tt:Network>
                    <tt:System>
                        ...
                    </tt:System>
                    <tt:IO>
                        <tt:InputConnectors>2</tt:InputConnectors>
                        <tt:RelayOutputs>1</tt:RelayOutputs>
                        <tt:Extension>
                            <tt:Auxiliary>false</tt:Auxiliary>
                            <tt:AuxiliaryCommands></tt:AuxiliaryCommands>
                            <tt:Extension></tt:Extension>
                        </tt:Extension>
                    </tt:IO>
                    <tt:Security>
                        ...
                    </tt:Security>
                </tt:Device>
                <tt:Events>
                    <tt:XAddr>http://192.168.1.123/onvif/event_service</tt:XAddr>
                    <tt:WSSubscriptionPolicySupport>true</tt:WSSubscriptionPolicySupport>
                    <tt:WSPullPointSupport>true</tt:WSPullPointSupport>
                    <tt:WSPausableSubscriptionManagerInterfaceSupport>false</tt:WSPausableSubscriptionManagerInterfaceSupport>
                </tt:Events>
                <tt:Imaging>
                    <tt:XAddr>http://192.168.1.123/onvif/imaging_service</tt:XAddr>
                </tt:Imaging>
                <tt:Media>
                    <tt:XAddr>http://192.168.1.123/onvif/media_service</tt:XAddr>
                    <tt:StreamingCapabilities>
                        <tt:RTPMulticast>true</tt:RTPMulticast>
                        <tt:RTP_TCP>true</tt:RTP_TCP>
                        <tt:RTP_RTSP_TCP>true</tt:RTP_RTSP_TCP>
                    </tt:StreamingCapabilities>
                    <tt:Extension>
                        <tt:ProfileCapabilities>
                            <tt:MaximumNumberOfProfiles>6</tt:MaximumNumberOfProfiles>
                        </tt:ProfileCapabilities>
                    </tt:Extension>
                </tt:Media>
                <tt:Extension>
                    <tt:DeviceIO>
                        <tt:XAddr>http://192.168.1.123/onvif/deviceIO_service</tt:XAddr>
                        <tt:VideoSources>1</tt:VideoSources>
                        <tt:VideoOutputs>0</tt:VideoOutputs>
                        <tt:AudioSources>1</tt:AudioSources>
                        <tt:AudioOutputs>1</tt:AudioOutputs>
                        <tt:RelayOutputs>1</tt:RelayOutputs>
                    </tt:DeviceIO>
                </tt:Extension>
            </tds:Capabilities>
        </tds:GetCapabilitiesResponse>
    </s:Body>
</s:Envelope>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawURL := FindTagValue([]byte(test.xml), "Media.+?XAddr")
			require.Equal(t, "http://192.168.1.123/onvif/media_service", rawURL)

			rawURL = FindTagValue([]byte(test.xml), "Imaging.+?XAddr")
			require.Equal(t, "http://192.168.1.123/onvif/imaging_service", rawURL)
		})
	}
}
