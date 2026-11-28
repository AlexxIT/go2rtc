package baichuan

import "encoding/binary"

type mediaMarkerUnit struct {
	start, bodyStart, plainEnd, paddedEnd int
	kind                                  MediaKind
	valid, continued, emitted             bool
}

func (p *MediaParser) appendMessage(msg message, packets []MediaPacket) ([]MediaPacket, error) {
	return p.appendMedia(msg.payload, packets, &msg)
}

func (p *MediaParser) noteUnmarked(offset, size int) {
	u := &p.markerUnit
	if !u.valid || size == 0 {
		return
	}
	end := offset + size
	if u.start <= offset && offset < u.plainEnd && end > u.bodyStart {
		u.continued = true
	}
	if offset > u.paddedEnd || end > u.paddedEnd {
		u.valid = false
	}
}

func (p *MediaParser) noteMarker(offset int) {
	if offset > len(p.buf) {
		p.markerQualified = false
		p.markerUnit = mediaMarkerUnit{}
		return
	}
	b := p.buf[offset:]
	kind, bodyStart, plainEnd, paddedEnd, valid := mediaMarkerSpan(b, p.limits.MaxMediaFrame)
	confirmed := confirmRecoveryBoundary(b, p.limits.MaxMediaFrame)
	if !valid || confirmed < 0 {
		p.markerQualified = false
		p.markerUnit = mediaMarkerUnit{}
		return
	}
	prior := p.markerUnit
	exact := prior.valid && (offset == prior.plainEnd || offset == prior.paddedEnd)
	complete := prior.emitted || (prior.start >= 0 && exact)
	shortHint := p.markerQualified && prior.valid && prior.kind == MediaVideoI && prior.start == 0 &&
		offset >= prior.bodyStart && offset < prior.plainEnd && confirmedVideoMarker(b, p.limits.MaxMediaFrame)
	if !p.markerQualified && exact && complete && prior.continued &&
		(prior.kind == MediaVideoI || prior.kind == MediaVideoP) && confirmed == 0 {
		p.markerQualified = true
	} else if p.markerQualified && !exact && !shortHint {
		p.markerQualified = false
	}
	if shortHint {
		p.markerHintAt = offset
	}
	if uint64(offset)+uint64(paddedEnd) > uint64(p.limits.MaxMediaBuffer) {
		p.markerUnit = mediaMarkerUnit{}
		return
	}
	p.markerUnit = mediaMarkerUnit{
		start: offset, bodyStart: offset + bodyStart, plainEnd: offset + plainEnd,
		paddedEnd: offset + paddedEnd, kind: kind, valid: true,
	}
}

func mediaMarkerSpan(b []byte, maxFrame uint32) (MediaKind, int, int, int, bool) {
	if len(b) < 4 {
		return 0, 0, 0, 0, false
	}
	magic := binary.LittleEndian.Uint32(b)
	switch {
	case magic == mediaInfoV1 || magic == mediaInfoV2:
		if len(b) < 32 || binary.LittleEndian.Uint32(b[4:8]) != 32 {
			return 0, 0, 0, 0, false
		}
		return MediaInfo, 0, 32, 32, true
	case magic >= mediaIFrameMin && magic <= mediaIFrameMax || magic >= mediaPFrameMin && magic <= mediaPFrameMax:
		if len(b) < mediaVideoHeaderSize || (binary.LittleEndian.Uint32(b[4:8]) != codecH264 && binary.LittleEndian.Uint32(b[4:8]) != codecH265) {
			return 0, 0, 0, 0, false
		}
		payload, extra := uint64(binary.LittleEndian.Uint32(b[8:12])), uint64(binary.LittleEndian.Uint32(b[12:16]))
		bodyStart := uint64(mediaVideoHeaderSize) + extra
		plain := bodyStart + payload
		padded := plain + uint64(padding(uint32(payload)))
		if padded > uint64(maxFrame) || padded > uint64(^uint(0)>>1) {
			return 0, 0, 0, 0, false
		}
		kind := MediaVideoP
		if magic <= mediaIFrameMax {
			kind = MediaVideoI
		}
		return kind, int(bodyStart), int(plain), int(padded), true
	case magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM:
		if len(b) < 8 || binary.LittleEndian.Uint16(b[4:6]) != binary.LittleEndian.Uint16(b[6:8]) {
			return 0, 0, 0, 0, false
		}
		payload := uint64(binary.LittleEndian.Uint16(b[4:6]))
		plain := uint64(8) + payload
		padded := plain + uint64(padding(uint32(payload)))
		if padded > uint64(maxFrame) || padded > uint64(^uint(0)>>1) {
			return 0, 0, 0, 0, false
		}
		if magic == mediaADPCM {
			return MediaADPCM, 8, int(plain), int(padded), true
		}
		return MediaAAC, 8, int(plain), int(padded), true
	default:
		return 0, 0, 0, 0, false
	}
}

func confirmedVideoMarker(b []byte, maxFrame uint32) bool {
	if len(b) < 4 {
		return false
	}
	magic := binary.LittleEndian.Uint32(b)
	return (magic >= mediaIFrameMin && magic <= mediaIFrameMax || magic >= mediaPFrameMin && magic <= mediaPFrameMax) &&
		confirmRecoveryBoundary(b, maxFrame) == 0
}

func (p *MediaParser) consumeMarkerUnit(size int, packet MediaPacket) {
	u := &p.markerUnit
	if !u.valid {
		return
	}
	if packet.Kind == MediaDiscontinuity || packet.boundaryAdjusted {
		u.valid = false
		return
	}
	if u.start == 0 && packet.Kind == u.kind && !packet.boundaryAdjusted &&
		(size == u.plainEnd || size == u.paddedEnd) {
		u.emitted = true
	}
	u.start -= size
	u.bodyStart -= size
	u.plainEnd -= size
	u.paddedEnd -= size
	if u.paddedEnd < 0 || u.plainEnd < 0 &&
		(!u.emitted || u.paddedEnd != 0 || u.plainEnd < -(mediaAlignment-1)) {
		u.valid = false
	}
}
