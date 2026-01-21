package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/udp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

const (
	keepaliveInterval    = 20 * time.Millisecond
	inactiveThreshold    = 100 * time.Millisecond
	realPacketTimeout    = 500 * time.Millisecond // time window after last real packet to forward FFmpeg output
	timestampDivisor20ms = 50
	restartDelay         = 3 * time.Second
	defaultMTU           = 1472
)

type RTPMixer struct {
	Node

	Media *Media
	Codec *Codec

	parents     []*Node
	parentPorts map[uint32]int // Map parent ID to its FFmpeg port

	// Per-parent RTP state for FFmpeg communication (each parent needs independent seq/ts)
	parentSequencer map[uint32]rtp.Sequencer
	parentTimestamp map[uint32]uint32

	// RTP packet normalization (for single-parent mode output)
	sequencer rtp.Sequencer
	timestamp uint32

	Bytes   int `json:"bytes,omitempty"`
	Packets int `json:"packets,omitempty"`

	ffmpegBinary string
	ffmpegCmd    *shell.Command
	udpServer    *udp.UDPServer

	// Keepalive for inactive parents (prevents FFmpeg from waiting)
	lastPacketTime     map[uint32]int64
	lastRealPacketTime atomic.Int64 // tracks when last real (non-keepalive) packet arrived
	keepaliveDone      chan struct{}

	closing            bool
	intentionalRestart bool

	mu sync.Mutex
}

func NewRTPMixer(ffmpegBinary string, media *Media, codec *Codec) *RTPMixer {
	m := &RTPMixer{
		Node:            Node{id: NewID(), Codec: codec},
		Media:           media,
		Codec:           codec,
		parentPorts:     make(map[uint32]int),
		parentSequencer: make(map[uint32]rtp.Sequencer),
		parentTimestamp: make(map[uint32]uint32),
		lastPacketTime:  make(map[uint32]int64),
		sequencer:       rtp.NewRandomSequencer(),
		ffmpegBinary:    ffmpegBinary,
	}

	m.Node.SetOwner(m)
	atomic.StoreUint32(&m.timestamp, 0)

	return m
}

func (m *RTPMixer) AddParent(parent *Node) {
	m.mu.Lock()
	oldCount := len(m.parents)
	m.parents = append(m.parents, parent)
	newCount := len(m.parents)
	m.mu.Unlock()

	// Add mixer as child of parent (so it appears in consumer receiver's childs)
	parent.AppendChild(&m.Node)

	// Set Forward hook to route packets with parentID context
	parentID := parent.id
	parent.Forward = func(packet *Packet) {
		m.handlePacketFromParent(packet, parentID)
	}

	m.handleTopologyChange(oldCount, newCount)
}

func (m *RTPMixer) RemoveParent(parent *Node) {
	m.mu.Lock()
	oldCount := len(m.parents)

	// Remove parent from list
	for i, p := range m.parents {
		if p == parent {
			m.parents = append(m.parents[:i], m.parents[i+1:]...)
			break
		}
	}

	// Clear Forward hook to restore default forwarding behavior
	parent.Forward = nil

	// Remove mixer from parent's children
	parent.RemoveChild(&m.Node)

	// Clear parent state
	delete(m.parentPorts, parent.id)
	delete(m.parentSequencer, parent.id)
	delete(m.parentTimestamp, parent.id)
	delete(m.lastPacketTime, parent.id)

	newCount := len(m.parents)
	m.mu.Unlock()

	// Handle topology change
	m.handleTopologyChange(oldCount, newCount)
}

func (m *RTPMixer) Close() {
	m.mu.Lock()

	if m.closing {
		m.mu.Unlock()
		return
	}

	m.closing = true
	m.mu.Unlock()

	m.stopFFmpeg()
	m.Node.Close()
}

func (m *RTPMixer) MarshalJSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up dead parents before marshaling
	m.parents = m.getActiveParents()

	data := struct {
		ID      uint32   `json:"id"`
		Codec   *Codec   `json:"codec"`
		Parents []uint32 `json:"parents,omitempty"`
		Childs  []uint32 `json:"childs,omitempty"`
		Bytes   int      `json:"bytes,omitempty"`
		Packets int      `json:"packets,omitempty"`
	}{
		ID:      m.id,
		Codec:   m.Codec,
		Parents: nodeIDs(m.parents),
		Childs:  nodeIDs(m.childs),
		Bytes:   m.Bytes,
		Packets: m.Packets,
	}

	return json.Marshal(data)
}

func (m *RTPMixer) handlePacketFromParent(packet *Packet, parentID uint32) {
	now := time.Now().UnixNano()

	// Track that we received a real packet (not keepalive)
	m.lastRealPacketTime.Store(now)

	// Update last packet time for keepalive tracking
	m.mu.Lock()
	m.lastPacketTime[parentID] = now
	ffmpegRunning := m.ffmpegCmd != nil
	m.mu.Unlock()

	if ffmpegRunning {
		m.sendToFFmpeg(packet, parentID)
	} else {
		m.forwardDirect(packet)
	}

	m.mu.Lock()
	m.Bytes += len(packet.Payload)
	m.Packets++
	m.mu.Unlock()
}

func (m *RTPMixer) handleTopologyChange(oldCount, newCount int) {
	if newCount == 0 {
		m.Close()
	} else if newCount >= 2 && oldCount < 2 {
		if err := m.restartFFmpeg(); err != nil {
			fmt.Fprintf(os.Stderr, "[mixer id=%d] Error starting FFmpeg: %v\n", m.id, err)
		} else {
			go m.monitorFFmpeg()
		}
	} else if oldCount >= 2 && newCount < 2 {
		m.stopFFmpeg()
	} else if newCount >= 2 {
		if err := m.restartFFmpeg(); err != nil {
			fmt.Fprintf(os.Stderr, "[mixer id=%d] Error restarting FFmpeg: %v\n", m.id, err)
		}
	}
}

func (m *RTPMixer) forwardDirect(packet *Packet) {
	m.mu.Lock()
	packet.SequenceNumber = m.sequencer.NextSequenceNumber()
	children := m.childs
	m.mu.Unlock()

	packet.Timestamp = atomic.AddUint32(&m.timestamp, m.frameSize())
	packet.Marker = true

	for _, child := range children {
		child.Input(packet)
	}
}

func (m *RTPMixer) getActiveParents() []*Node {
	var activeParents []*Node
	for _, parent := range m.parents {
		parent.mu.Lock()
		hasMixer := slices.Contains(parent.childs, &m.Node)
		parent.mu.Unlock()

		if hasMixer {
			activeParents = append(activeParents, parent)
		}
	}
	return activeParents
}

func (m *RTPMixer) restartFFmpeg() error {
	// Mark as intentional restart so monitor doesn't try to restart too
	m.mu.Lock()
	m.intentionalRestart = true
	m.mu.Unlock()

	m.stopFFmpeg()

	// Check if we have enough parents to mix
	activeParents := m.getActiveParents()
	if len(activeParents) < 2 {
		return nil
	}

	return m.startFFmpeg()
}

func (m *RTPMixer) startFFmpeg() error {
	sdpContent, err := m.generateSDP()
	if err != nil {
		return fmt.Errorf("failed to generate SDP: %w", err)
	}

	udpServer, err := udp.NewUDPServer()
	if err != nil {
		return fmt.Errorf("failed to create UDP server: %w", err)
	}
	m.udpServer = udpServer

	m.mu.Lock()
	numParents := len(m.parents)
	m.mu.Unlock()

	outputCodec := FFmpegCodecName(m.Codec.Name)
	switch m.Codec.Name {
	case CodecELD:
		outputCodec = "libfdk_aac"
	case CodecG722:
		outputCodec = "pcm_s16le"
	}

	sampleRate := m.Codec.ClockRate
	if sampleRate == 0 {
		sampleRate = 8000
	}

	args := &ffmpeg.Args{
		Bin:           m.ffmpegBinary,
		Global:        "-hide_banner",
		Input:         "-protocol_whitelist pipe,rtp,udp,file,crypto -listen_timeout 1 -f sdp -i pipe:0",
		FilterComplex: fmt.Sprintf("amix=inputs=%d:duration=longest:dropout_transition=0", numParents),
		Output:        fmt.Sprintf("-f rtp rtp://127.0.0.1:%d", udpServer.Port()),
	}

	args.AddCodec(fmt.Sprintf("-ar %d -c:a %s", sampleRate, outputCodec))

	m.ffmpegCmd = shell.NewCommand(args.String())
	m.ffmpegCmd.Stdin = bytes.NewReader(sdpContent)

	if err := m.ffmpegCmd.Start(); err != nil {
		udpServer.Close()
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	// Start reader
	go m.readFromFFmpeg()

	// Start keepalive
	m.keepaliveDone = make(chan struct{})
	go m.runKeepalive()

	return nil
}

func (m *RTPMixer) monitorFFmpeg() {
	for {
		m.mu.Lock()
		cmd := m.ffmpegCmd
		m.mu.Unlock()

		if cmd == nil {
			return // FFmpeg was intentionally stopped
		}

		// Wait for FFmpeg to exit
		_ = cmd.Wait()

		m.mu.Lock()
		wasIntentional := m.intentionalRestart
		m.intentionalRestart = false // Reset flag
		stillNeeded := len(m.parents) >= 2 && !m.closing
		m.mu.Unlock()

		if !stillNeeded {
			return
		}

		if wasIntentional {
			continue // Don't restart, handleTopologyChange already did it
		}

		time.Sleep(restartDelay)

		// Attempt restart
		if err := m.restartFFmpeg(); err != nil {
			continue
		}
	}
}

func (m *RTPMixer) stopFFmpeg() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.keepaliveDone != nil {
		close(m.keepaliveDone)
		m.keepaliveDone = nil
	}

	if m.ffmpegCmd != nil {
		if m.ffmpegCmd.Process != nil {
			m.ffmpegCmd.Process.Kill()
		}
		m.ffmpegCmd.Wait()
		m.ffmpegCmd = nil
	}

	if m.udpServer != nil {
		m.udpServer.Close()
		m.udpServer = nil
	}

	m.parentPorts = make(map[uint32]int)
	m.parentSequencer = make(map[uint32]rtp.Sequencer)
}

func (m *RTPMixer) generateSDP() ([]byte, error) {
	m.mu.Lock()
	parents := m.parents
	m.mu.Unlock()

	if len(parents) == 0 {
		return nil, fmt.Errorf("no parents")
	}

	sd := &sdp.SessionDescription{
		Origin: sdp.Origin{
			Username: "-", SessionID: 1, SessionVersion: 1,
			NetworkType: "IN", AddressType: "IP4", UnicastAddress: "0.0.0.0",
		},
		SessionName: "go2rtc-mixer",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN", AddressType: "IP4",
			Address: &sdp.Address{Address: "127.0.0.1"},
		},
		TimeDescriptions: []sdp.TimeDescription{{Timing: sdp.Timing{}}},
	}

	for _, parent := range parents {
		port, err := udp.GetFreePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}

		m.mu.Lock()
		m.parentPorts[parent.id] = port
		m.mu.Unlock()

		// Codec name for SDP
		codecName := m.Codec.Name
		switch codecName {
		case CodecELD:
			codecName = CodecAAC
		case CodecPCML:
			codecName = CodecPCM
		}

		md := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:  KindAudio,
				Port:   sdp.RangedPort{Value: port},
				Protos: []string{"RTP", "AVP"},
			},
		}

		md.WithCodec(m.Codec.PayloadType, codecName, m.Codec.ClockRate, uint16(m.Codec.Channels), m.Codec.FmtpLine)
		md.WithPropertyAttribute(DirectionRecvonly)

		sd.MediaDescriptions = append(sd.MediaDescriptions, md)
	}

	return sd.Marshal()
}

func (m *RTPMixer) readFromFFmpeg() {
	buf := make([]byte, defaultMTU)

	for {
		m.mu.Lock()
		server := m.udpServer
		m.mu.Unlock()

		if server == nil {
			return
		}

		n, _, err := server.ReadFrom(buf)
		if err != nil {
			return
		}

		packet := &Packet{}
		if err := packet.Unmarshal(buf[:n]); err != nil {
			continue
		}

		// Only forward if we received real packets from parents recently
		// This filters out FFmpeg output that's based purely on keepalive silence
		lastReal := m.lastRealPacketTime.Load()
		if time.Now().UnixNano()-lastReal > int64(realPacketTimeout) {
			continue
		}

		m.forwardDirect(packet)
	}
}

func (m *RTPMixer) sendToFFmpeg(packet *Packet, parentID uint32) error {
	m.mu.Lock()
	port, exists := m.parentPorts[parentID]
	server := m.udpServer

	sequencer, hasSequencer := m.parentSequencer[parentID]
	if !hasSequencer {
		sequencer = rtp.NewRandomSequencer()
		m.parentSequencer[parentID] = sequencer
	}

	packet.SequenceNumber = sequencer.NextSequenceNumber()

	timestamp := m.parentTimestamp[parentID]
	m.parentTimestamp[parentID] = timestamp + m.frameSize()
	packet.Timestamp = timestamp

	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("no port for parent %d", parentID)
	}

	if server == nil {
		return fmt.Errorf("no UDP server")
	}

	data, err := packet.Marshal()
	if err != nil {
		return err
	}

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
	_, err = server.WriteTo(data, addr)
	return err
}

func (m *RTPMixer) runKeepalive() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.keepaliveDone:
			return
		case <-ticker.C:
			m.sendKeepalive()
		}
	}
}

func (m *RTPMixer) sendKeepalive() {
	now := time.Now().UnixNano()

	m.mu.Lock()
	parents := m.parents
	lastTimes := maps.Clone(m.lastPacketTime)
	m.mu.Unlock()

	for _, parent := range parents {
		lastTime, exists := lastTimes[parent.id]
		if !exists || (now-lastTime) > int64(inactiveThreshold) {
			m.sendSilence(parent.id)
		}
	}
}

func (m *RTPMixer) sendSilence(parentID uint32) error {
	packet := &Packet{}
	packet.Version = 2
	packet.PayloadType = m.Codec.PayloadType
	packet.SSRC = 0
	packet.Payload = make([]byte, m.frameSize())

	return m.sendToFFmpeg(packet, parentID)
}

func (m *RTPMixer) frameSize() uint32 {
	switch m.Codec.Name {
	case CodecOpus:
		return 960 // Opus @ 48kHz: 20ms = 960 samples
	case CodecAAC, CodecELD:
		return 1024 // AAC always uses 1024 samples per frame
	}

	// For all other codecs, calculate based on ClockRate
	// 20ms = ClockRate / 50
	if m.Codec.ClockRate > 0 {
		return m.Codec.ClockRate / timestampDivisor20ms
	}

	// Fallback to 8kHz default (160 samples)
	return 160
}

func nodeIDs(nodes []*Node) []uint32 {
	if len(nodes) == 0 {
		return nil
	}
	ids := make([]uint32, len(nodes))
	for i, node := range nodes {
		ids[i] = node.id
	}
	return ids
}
