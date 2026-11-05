package hds

import (
	"bytes"
	"net"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// newSessionPair creates a connected accessory/controller session pair for testing.
func newSessionPair(t *testing.T) (accessory *Session, controller *Session) {
	t.Helper()
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)

	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	accConn, err := NewConn(c1, key, salt, false) // accessory
	require.NoError(t, err)
	ctrlConn, err := NewConn(c2, key, salt, true) // controller
	require.NoError(t, err)

	return NewSession(accConn), NewSession(ctrlConn)
}

// readLargeMsg reads a message using a large buffer (for messages with 256KB+ chunks).
// Session.ReadMessage uses 64KB which is too small for media chunks in tests.
func readLargeMsg(t *testing.T, s *Session) *Message {
	t.Helper()
	buf := make([]byte, 512*1024) // 512KB
	n, err := s.conn.Read(buf)
	require.NoError(t, err)
	data := buf[:n]

	require.GreaterOrEqual(t, len(data), 2)
	headerLen := int(data[0])
	require.GreaterOrEqual(t, len(data), 1+headerLen)

	headerVal, err := OpackUnmarshal(data[1 : 1+headerLen])
	require.NoError(t, err)
	header := headerVal.(map[string]any)

	msg := &Message{Protocol: opackString(header["protocol"])}
	if topic, ok := header["event"]; ok {
		msg.IsEvent = true
		msg.Topic = opackString(topic)
	} else if topic, ok := header["response"]; ok {
		msg.Topic = opackString(topic)
		msg.ID = opackInt(header["id"])
		msg.Status = opackInt(header["status"])
	} else if topic, ok := header["request"]; ok {
		msg.Topic = opackString(topic)
		msg.ID = opackInt(header["id"])
	}

	bodyData := data[1+headerLen:]
	if len(bodyData) > 0 {
		bodyVal, err := OpackUnmarshal(bodyData)
		require.NoError(t, err)
		if m, ok := bodyVal.(map[string]any); ok {
			msg.Body = m
		}
	}
	return msg
}

// extractPacket extracts data and metadata from a dataSend.data message body.
func extractPacket(t *testing.T, body map[string]any) (data []byte, metadata map[string]any) {
	t.Helper()
	packets, ok := body["packets"].([]any)
	require.True(t, ok, "packets must be array")
	require.Len(t, packets, 1)

	pkt, ok := packets[0].(map[string]any)
	require.True(t, ok, "packet element must be dict")

	data, ok = pkt["data"].([]byte)
	require.True(t, ok, "data must be []byte")

	metadata, ok = pkt["metadata"].(map[string]any)
	require.True(t, ok, "metadata must be dict")
	return
}

// --- SendMediaInit tests ---

func TestSendMediaInit_Structure(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	initData := bytes.Repeat([]byte{0xAB}, 100)

	go func() {
		require.NoError(t, acc.SendMediaInit(1, initData))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	require.Equal(t, ProtoDataSend, msg.Protocol)
	require.Equal(t, TopicData, msg.Topic)
	require.True(t, msg.IsEvent)
	require.Equal(t, int64(1), opackInt(msg.Body["streamId"]))

	data, meta := extractPacket(t, msg.Body)
	require.Equal(t, initData, data)
	require.Equal(t, "mediaInitialization", opackString(meta["dataType"]))
	require.Equal(t, int64(1), opackInt(meta["dataSequenceNumber"]))
	require.Equal(t, int64(1), opackInt(meta["dataChunkSequenceNumber"]))
	require.Equal(t, true, meta["isLastDataChunk"])
	require.Equal(t, int64(len(initData)), opackInt(meta["dataTotalSize"]))
}

func TestSendMediaInit_AlwaysSeqOne(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		require.NoError(t, acc.SendMediaInit(42, []byte{1, 2, 3}))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	_, meta := extractPacket(t, msg.Body)
	require.Equal(t, int64(1), opackInt(meta["dataSequenceNumber"]))
	require.Equal(t, int64(42), opackInt(msg.Body["streamId"]))
}

// --- SendMediaFragment single chunk tests ---

func TestSendMediaFragment_SingleChunk(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	fragment := bytes.Repeat([]byte{0xCD}, 1000) // well under 256KB

	go func() {
		require.NoError(t, acc.SendMediaFragment(5, fragment, 3))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	data, meta := extractPacket(t, msg.Body)
	require.Equal(t, fragment, data)
	require.Equal(t, "mediaFragment", opackString(meta["dataType"]))
	require.Equal(t, int64(3), opackInt(meta["dataSequenceNumber"]))
	require.Equal(t, int64(1), opackInt(meta["dataChunkSequenceNumber"]))
	require.Equal(t, true, meta["isLastDataChunk"])
	require.Equal(t, int64(1000), opackInt(meta["dataTotalSize"]))
}

// --- SendMediaFragment multi-chunk tests (using readLargeMsg) ---

func TestSendMediaFragment_MultipleChunks(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	totalSize := maxChunkSize*2 + 100 // 2 full chunks + partial
	fragment := make([]byte, totalSize)
	for i := range fragment {
		fragment[i] = byte(i % 251) // use prime to verify no data corruption
	}

	go func() {
		require.NoError(t, acc.SendMediaFragment(1, fragment, 7))
	}()

	var assembled []byte

	// Chunk 1: full 256KB
	msg1 := readLargeMsg(t, ctrl)
	data1, meta1 := extractPacket(t, msg1.Body)
	require.Len(t, data1, maxChunkSize)
	require.Equal(t, int64(1), opackInt(meta1["dataChunkSequenceNumber"]))
	require.Equal(t, false, meta1["isLastDataChunk"])
	require.Equal(t, int64(totalSize), opackInt(meta1["dataTotalSize"]))
	require.Equal(t, int64(7), opackInt(meta1["dataSequenceNumber"]))
	assembled = append(assembled, data1...)

	// Chunk 2: full 256KB
	msg2 := readLargeMsg(t, ctrl)
	data2, meta2 := extractPacket(t, msg2.Body)
	require.Len(t, data2, maxChunkSize)
	require.Equal(t, int64(2), opackInt(meta2["dataChunkSequenceNumber"]))
	require.Equal(t, false, meta2["isLastDataChunk"])
	// dataTotalSize only in first chunk
	_, hasTotalSize := meta2["dataTotalSize"]
	require.False(t, hasTotalSize, "dataTotalSize should only be in first chunk")
	assembled = append(assembled, data2...)

	// Chunk 3: remaining 100 bytes
	msg3 := readLargeMsg(t, ctrl)
	data3, meta3 := extractPacket(t, msg3.Body)
	require.Len(t, data3, 100)
	require.Equal(t, int64(3), opackInt(meta3["dataChunkSequenceNumber"]))
	require.Equal(t, true, meta3["isLastDataChunk"])
	assembled = append(assembled, data3...)

	require.Equal(t, fragment, assembled, "reassembled data must match original")
}

func TestSendMediaFragment_ExactChunkBoundary(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	fragment := bytes.Repeat([]byte{0xAA}, maxChunkSize) // exactly 256KB

	go func() {
		require.NoError(t, acc.SendMediaFragment(1, fragment, 2))
	}()

	msg := readLargeMsg(t, ctrl)
	data, meta := extractPacket(t, msg.Body)
	require.Len(t, data, maxChunkSize)
	require.Equal(t, int64(1), opackInt(meta["dataChunkSequenceNumber"]))
	require.Equal(t, true, meta["isLastDataChunk"]) // single chunk
}

func TestSendMediaFragment_TwoExactChunks(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	fragment := bytes.Repeat([]byte{0xBB}, maxChunkSize*2) // exactly 2 chunks

	go func() {
		require.NoError(t, acc.SendMediaFragment(1, fragment, 4))
	}()

	msg1 := readLargeMsg(t, ctrl)
	_, meta1 := extractPacket(t, msg1.Body)
	require.Equal(t, false, meta1["isLastDataChunk"])
	require.Equal(t, int64(1), opackInt(meta1["dataChunkSequenceNumber"]))

	msg2 := readLargeMsg(t, ctrl)
	_, meta2 := extractPacket(t, msg2.Body)
	require.Equal(t, true, meta2["isLastDataChunk"])
	require.Equal(t, int64(2), opackInt(meta2["dataChunkSequenceNumber"]))
}

func TestSendMediaFragment_SequencePreserved(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	// All chunks of a multi-chunk fragment share the same dataSequenceNumber
	totalSize := maxChunkSize + 50
	fragment := bytes.Repeat([]byte{0x11}, totalSize)

	go func() {
		require.NoError(t, acc.SendMediaFragment(1, fragment, 42))
	}()

	msg1 := readLargeMsg(t, ctrl)
	_, meta1 := extractPacket(t, msg1.Body)
	require.Equal(t, int64(42), opackInt(meta1["dataSequenceNumber"]))

	msg2, err := ctrl.ReadMessage() // second chunk is small (50 bytes)
	require.NoError(t, err)
	_, meta2 := extractPacket(t, msg2.Body)
	require.Equal(t, int64(42), opackInt(meta2["dataSequenceNumber"]))
}

// --- WriteEvent / WriteResponse / WriteRequest round-trip tests ---

func TestWriteEvent_ReadMessage(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		require.NoError(t, acc.WriteEvent("testProto", "testTopic", map[string]any{
			"key": "value",
		}))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	require.Equal(t, "testProto", msg.Protocol)
	require.Equal(t, "testTopic", msg.Topic)
	require.True(t, msg.IsEvent)
	require.Equal(t, "value", msg.Body["key"])
}

func TestWriteResponse_ReadMessage(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		require.NoError(t, acc.WriteResponse("proto", "topic", 5, 0, map[string]any{"ok": true}))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	require.Equal(t, "proto", msg.Protocol)
	require.Equal(t, "topic", msg.Topic)
	require.Equal(t, int64(5), msg.ID)
	require.Equal(t, int64(0), msg.Status)
	require.False(t, msg.IsEvent)
	require.Equal(t, true, msg.Body["ok"])
}

func TestWriteRequest_ReadMessage(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		id, err := acc.WriteRequest("proto", "topic", map[string]any{"x": int64(10)})
		require.NoError(t, err)
		require.Equal(t, int64(1), id) // first request
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)

	require.Equal(t, "proto", msg.Protocol)
	require.Equal(t, "topic", msg.Topic)
	require.Equal(t, int64(1), msg.ID)
	require.False(t, msg.IsEvent)
}

func TestWriteRequest_IncrementingIDs(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		id1, _ := acc.WriteRequest("p", "t", nil)
		id2, _ := acc.WriteRequest("p", "t", nil)
		id3, _ := acc.WriteRequest("p", "t", nil)
		require.Equal(t, int64(1), id1)
		require.Equal(t, int64(2), id2)
		require.Equal(t, int64(3), id3)
	}()

	for expected := int64(1); expected <= 3; expected++ {
		msg, err := ctrl.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, expected, msg.ID)
	}
}

func TestWriteEvent_NilBody(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		require.NoError(t, acc.WriteEvent("p", "t", nil))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)
	require.NotNil(t, msg.Body) // nil is replaced with empty map
}

func TestWriteResponse_NilBody(t *testing.T) {
	acc, ctrl := newSessionPair(t)

	go func() {
		require.NoError(t, acc.WriteResponse("p", "t", 1, 0, nil))
	}()

	msg, err := ctrl.ReadMessage()
	require.NoError(t, err)
	require.NotNil(t, msg.Body)
}

// --- Helper tests ---

func TestOpackHelpers(t *testing.T) {
	require.Equal(t, "", opackString(nil))
	require.Equal(t, "", opackString(42))
	require.Equal(t, "hello", opackString("hello"))

	require.Equal(t, int64(0), opackInt(nil))
	require.Equal(t, int64(0), opackInt("not a number"))
	require.Equal(t, int64(42), opackInt(int64(42)))
	require.Equal(t, int64(7), opackInt(int(7)))
	require.Equal(t, int64(3), opackInt(float64(3.9)))
}

// --- Benchmarks ---

func BenchmarkSendMediaFragment_Small(b *testing.B) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	accConn, _ := NewConn(c1, key, salt, false)
	ctrlConn, _ := NewConn(c2, key, salt, true)

	acc := NewSession(accConn)
	fragment := bytes.Repeat([]byte{0xAA}, 2000) // 2KB typical P-frame fragment

	go func() {
		buf := make([]byte, 64*1024)
		for {
			if _, err := ctrlConn.Read(buf); err != nil {
				return
			}
		}
	}()

	b.SetBytes(int64(len(fragment)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = acc.SendMediaFragment(1, fragment, i)
	}
}

func BenchmarkSendMediaFragment_Large(b *testing.B) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	accConn, _ := NewConn(c1, key, salt, false)
	ctrlConn, _ := NewConn(c2, key, salt, true)

	acc := NewSession(accConn)
	fragment := bytes.Repeat([]byte{0xBB}, 5*1024*1024) // 5MB typical GOP

	go func() {
		buf := make([]byte, 512*1024)
		for {
			if _, err := ctrlConn.Read(buf); err != nil {
				return
			}
		}
	}()

	b.SetBytes(int64(len(fragment)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = acc.SendMediaFragment(1, fragment, i)
	}
}

func BenchmarkOpackMarshal_MediaBody(b *testing.B) {
	data := bytes.Repeat([]byte{0xCC}, maxChunkSize)
	body := map[string]any{
		"streamId": 1,
		"packets": []any{
			map[string]any{
				"data": data,
				"metadata": map[string]any{
					"dataType":                "mediaFragment",
					"dataSequenceNumber":      42,
					"dataChunkSequenceNumber": 1,
					"isLastDataChunk":         true,
					"dataTotalSize":           len(data),
				},
			},
		},
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		OpackMarshal(body)
	}
}

func BenchmarkWriteMessage(b *testing.B) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	accConn, _ := NewConn(c1, key, salt, false)
	ctrlConn, _ := NewConn(c2, key, salt, true)

	acc := NewSession(accConn)

	go func() {
		buf := make([]byte, 64*1024)
		for {
			if _, err := ctrlConn.Read(buf); err != nil {
				return
			}
		}
	}()

	header := map[string]any{"protocol": "dataSend", "event": "data"}
	body := map[string]any{"streamId": 1, "test": true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = acc.WriteMessage(header, body)
	}
}
