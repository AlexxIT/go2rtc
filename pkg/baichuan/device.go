package baichuan

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

const (
	maxDeviceInfoBody  = 64 << 10
	maxDeviceInfoField = 128
)

// DeviceInfo contains non-secret camera identity reported by the firmware.
type DeviceInfo struct {
	Type     string
	Model    string
	Hardware string
	Firmware string
}

func (d DeviceInfo) DisplayModel() string {
	if d.Model != "" {
		return d.Model
	}
	return d.Type
}

type deviceInfoEnvelope struct {
	Info *struct {
		Type     string `xml:"type"`
		Model    string `xml:"itemNo"`
		Hardware string `xml:"hardwareVersion"`
		Firmware string `xml:"firmwareVersion"`
	} `xml:"VersionInfo"`
}

func (c *Client) DeviceInfo(ctx context.Context) (DeviceInfo, error) {
	// Serialize the first query so concurrent setup cannot issue or cache duplicates.
	c.deviceMu.Lock()
	defer c.deviceMu.Unlock()
	if c.deviceKnown {
		return c.device, nil
	}
	if err := c.Login(ctx); err != nil {
		return DeviceInfo{}, err
	}
	response, err := c.roundTrip(ctx, request{command: commandDeviceInfo, class: classOffset})
	if err != nil {
		return DeviceInfo{}, fmt.Errorf("baichuan: query device information: %w", err)
	}
	value, err := parseDeviceInfo(response.payload)
	if err != nil {
		return DeviceInfo{}, fmt.Errorf("baichuan: parse device information: %w", err)
	}
	c.device = value
	c.deviceKnown = true
	return value, nil
}

func parseDeviceInfo(body []byte) (DeviceInfo, error) {
	if len(body) > maxDeviceInfoBody {
		return DeviceInfo{}, fmt.Errorf("response exceeds %d bytes", maxDeviceInfoBody)
	}
	var envelope deviceInfoEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return DeviceInfo{}, err
	}
	if envelope.Info == nil {
		return DeviceInfo{}, fmt.Errorf("VersionInfo missing from response")
	}
	value := DeviceInfo{
		Type:     strings.TrimSpace(envelope.Info.Type),
		Model:    strings.TrimSpace(envelope.Info.Model),
		Hardware: strings.TrimSpace(envelope.Info.Hardware),
		Firmware: strings.TrimSpace(envelope.Info.Firmware),
	}
	for _, field := range []string{value.Type, value.Model, value.Hardware, value.Firmware} {
		if len(field) > maxDeviceInfoField {
			return DeviceInfo{}, fmt.Errorf("device information field exceeds %d bytes", maxDeviceInfoField)
		}
	}
	if value.DisplayModel() == "" {
		return DeviceInfo{}, fmt.Errorf("camera model missing from response")
	}
	return value, nil
}
