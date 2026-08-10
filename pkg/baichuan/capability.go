package baichuan

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	capabilityTokens  = "system, streaming, PTZ, IO, security, replay, disk, network, alarm, record, video, image"
	maxCapabilityBody = 256 << 10
)

// Capabilities is the bounded channel set observed in a capability response.
type Capabilities struct {
	ObservedChannels []uint8
}

func (c Capabilities) clone() Capabilities {
	c.ObservedChannels = append([]uint8(nil), c.ObservedChannels...)
	return c
}

type capabilityExtension struct {
	XMLName  xml.Name `xml:"Extension"`
	Version  string   `xml:"version,attr"`
	Username string   `xml:"userName"`
	Token    string   `xml:"token"`
}

type capabilityModule struct {
	Channel *uint8 `xml:"channelId"`
}

func (c *Client) Capabilities(ctx context.Context, channel uint8) (Capabilities, error) {
	// Serialize the first query so concurrent setup cannot issue or cache duplicates.
	c.capabilityMu.Lock()
	defer c.capabilityMu.Unlock()
	if value, ok := c.capabilities[channel]; ok {
		return value.clone(), nil
	}
	if err := c.Login(ctx); err != nil {
		return Capabilities{}, err
	}
	extension, err := marshalDocument(capabilityExtension{
		Version: "1.1", Username: c.cfg.username, Token: capabilityTokens,
	})
	if err != nil {
		return Capabilities{}, fmt.Errorf("baichuan: build capability request: %w", err)
	}
	response, err := c.roundTrip(ctx, request{
		command: commandAbility, channel: channel, class: classOffset, extension: extension,
	})
	if err != nil {
		return Capabilities{}, fmt.Errorf("baichuan: query capabilities: %w", err)
	}
	value, err := parseCapabilities(response.payload)
	if err != nil {
		return Capabilities{}, fmt.Errorf("baichuan: parse capabilities: %w", err)
	}
	if c.capabilities == nil {
		c.capabilities = make(map[uint8]Capabilities)
	}
	c.capabilities[channel] = value
	return value.clone(), nil
}

func parseCapabilities(body []byte) (Capabilities, error) {
	if len(body) > maxCapabilityBody {
		return Capabilities{}, fmt.Errorf("response exceeds %d bytes", maxCapabilityBody)
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	channels := make(map[uint8]struct{})
	found, inside := false, false
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Capabilities{}, err
		}
		if end, ok := token.(xml.EndElement); ok {
			if end.Name.Local == "AbilityInfo" {
				inside = false
			}
			continue
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "AbilityInfo" {
			found = true
			inside = true
			continue
		}
		if !inside || start.Name.Local != "subModule" {
			continue
		}
		var module capabilityModule
		if err = decoder.DecodeElement(&module, &start); err != nil {
			return Capabilities{}, err
		}
		if module.Channel != nil {
			channels[*module.Channel] = struct{}{}
		}
	}
	if !found {
		return Capabilities{}, fmt.Errorf("AbilityInfo missing from response")
	}
	var value Capabilities
	for id := range channels {
		value.ObservedChannels = append(value.ObservedChannels, id)
	}
	sort.Slice(value.ObservedChannels, func(i, j int) bool {
		return value.ObservedChannels[i] < value.ObservedChannels[j]
	})
	return value, nil
}
