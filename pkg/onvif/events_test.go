package onvif

import (
	"net/url"
	"testing"
)

func TestParseMotionEvents_CellMotionDetector_True(t *testing.T) {
	// Dahua-style CellMotionDetector/Motion with IsMotion=true
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="IsMotion" Value="true"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if !motion {
		t.Fatal("expected motion=true")
	}
}

func TestParseMotionEvents_CellMotionDetector_False(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="IsMotion" Value="false"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if motion {
		t.Fatal("expected motion=false")
	}
}

func TestParseMotionEvents_VideoSourceMotionAlarm_True(t *testing.T) {
	// Hikvision-style VideoSource/MotionAlarm with State=true
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:VideoSource/MotionAlarm</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="State" Value="true"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if !motion {
		t.Fatal("expected motion=true")
	}
}

func TestParseMotionEvents_VideoSourceMotionAlarm_False(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:VideoSource/MotionAlarm</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="State" Value="false"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if motion {
		t.Fatal("expected motion=false")
	}
}

func TestParseMotionEvents_NoMotionTopic(t *testing.T) {
	// Response with a non-motion topic (e.g., tampering)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:VideoSource/ImageTooBlurry/ImagingService</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="State" Value="true"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	_, found := ParseMotionEvents([]byte(xml))
	if found {
		t.Fatal("expected found=false for non-motion topic")
	}
}

func TestParseMotionEvents_EmptyResponse(t *testing.T) {
	// PullMessages response with no notifications (timeout, no events)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:CurrentTime>2025-01-15T10:00:00Z</tev:CurrentTime>
<tev:TerminationTime>2025-01-15T10:01:00Z</tev:TerminationTime>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	_, found := ParseMotionEvents([]byte(xml))
	if found {
		t.Fatal("expected found=false for empty response")
	}
}

func TestParseMotionEvents_MotionRegionDetector(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:RuleEngine/MotionRegionDetector/Motion</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="State" Value="1"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if !motion {
		t.Fatal("expected motion=true for Value=1")
	}
}

func TestParseMotionEvents_MultipleNotifications(t *testing.T) {
	// Multiple notifications: first motion=true, then motion=false. Should return last.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body>
<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="IsMotion" Value="true"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
<tev:NotificationMessage>
<wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
 xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
<wsnt:Message>
<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
<tt:Data>
<tt:SimpleItem Name="IsMotion" Value="false"/>
</tt:Data>
</tt:Message>
</wsnt:Message>
</tev:NotificationMessage>
</tev:PullMessagesResponse>
</env:Body>
</env:Envelope>`

	motion, found := ParseMotionEvents([]byte(xml))
	if !found {
		t.Fatal("expected found=true")
	}
	if motion {
		t.Fatal("expected motion=false (last notification)")
	}
}

func TestResolveEventAddress_RelativePath(t *testing.T) {
	u, _ := url.Parse("http://camera.example/onvif/device_service")
	client := &Client{url: u}

	got := client.resolveEventAddress("/onvif/Subscription?Idx=1")
	want := "http://camera.example/onvif/Subscription?Idx=1"

	if got != want {
		t.Fatalf("unexpected resolved address: got %q want %q", got, want)
	}
}

func TestResolveEventAddress_DockerInternalIP(t *testing.T) {
	u, _ := url.Parse("http://localhost:18080/onvif/device_service")
	client := &Client{url: u}

	got := client.resolveEventAddress("http://172.17.0.2:8080/onvif/events_service?sub=1")
	want := "http://localhost:18080/onvif/events_service?sub=1"

	if got != want {
		t.Fatalf("unexpected resolved address: got %q want %q", got, want)
	}
}
