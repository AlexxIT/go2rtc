package amf

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewReader(t *testing.T) {
	tests := []struct {
		name   string
		actual string
		expect []any
	}{
		{
			name:   "ffmpeg-http",
			actual: "02000a6f6e4d65746144617461080000001000086475726174696f6e000000000000000000000577696474680040940000000000000006686569676874004086800000000000000d766964656f646174617261746500409e62770000000000096672616d6572617465004038000000000000000c766964656f636f646563696400401c000000000000000d617564696f646174617261746500405ea93000000000000f617564696f73616d706c65726174650040e5888000000000000f617564696f73616d706c6573697a65004030000000000000000673746572656f0101000c617564696f636f6465636964004024000000000000000b6d616a6f725f6272616e640200046d703432000d6d696e6f725f76657273696f6e020001300011636f6d70617469626c655f6272616e647302000c69736f6d617663316d7034320007656e636f64657202000c4c61766636302e352e313030000866696c6573697a65000000000000000000000009",
			expect: []any{
				"onMetaData",
				map[string]any{
					"compatible_brands": "isomavc1mp42",
					"major_brand":       "mp42",
					"minor_version":     "0",
					"encoder":           "Lavf60.5.100",

					"filesize": float64(0),
					"duration": float64(0),

					"videocodecid":  float64(7),
					"width":         float64(1280),
					"height":        float64(720),
					"framerate":     float64(24),
					"videodatarate": 1944.6162109375,

					"audiocodecid":    float64(10),
					"audiosamplerate": float64(44100),
					"stereo":          true,
					"audiosamplesize": float64(16),
					"audiodatarate":   122.6435546875,
				},
			},
		},
		{
			name:   "ffmpeg-file",
			actual: "02000a6f6e4d65746144617461080000000800086475726174696f6e004000000000000000000577696474680040940000000000000006686569676874004086800000000000000d766964656f646174617261746500000000000000000000096672616d6572617465004039000000000000000c766964656f636f646563696400401c0000000000000007656e636f64657202000c4c61766636302e352e313030000866696c6573697a6500411f541400000000000009",
			expect: []any{
				"onMetaData",
				map[string]any{
					"encoder": "Lavf60.5.100",

					"filesize": float64(513285),
					"duration": float64(2),

					"videocodecid":  float64(7),
					"width":         float64(1280),
					"height":        float64(720),
					"framerate":     float64(25),
					"videodatarate": float64(0),
				},
			},
		},
		{
			name:   "reolink-1",
			actual: "0200075f726573756c74003ff0000000000000030006666d7356657202000d464d532f332c302c312c313233000c6361706162696c697469657300403f0000000000000000090300056c6576656c0200067374617475730004636f646502001d4e6574436f6e6e656374696f6e2e436f6e6e6563742e53756363657373000b6465736372697074696f6e020015436f6e6e656374696f6e207375636365656465642e000e6f626a656374456e636f64696e67000000000000000000000009",
			expect: []any{
				"_result", float64(1),
				map[string]any{
					"capabilities": float64(31),
					"fmsVer":       "FMS/3,0,1,123",
				},
				map[string]any{
					"code":           "NetConnection.Connect.Success",
					"description":    "Connection succeeded.",
					"level":          "status",
					"objectEncoding": float64(0),
				},
			},
		},
		{
			name:   "reolink-2",
			actual: "0200075f726573756c7400400000000000000005003ff0000000000000",
			expect: []any{
				"_result", float64(2), nil, float64(1),
			},
		},
		{
			name:   "reolink-3",
			actual: "0200086f6e537461747573000000000000000000050300056c6576656c0200067374617475730004636f64650200144e657453747265616d2e506c61792e5374617274000b6465736372697074696f6e020015537461727420766964656f206f6e2064656d616e64000009",
			expect: []any{
				"onStatus", float64(0), nil,
				map[string]any{
					"code":        "NetStream.Play.Start",
					"description": "Start video on demand",
					"level":       "status",
				},
			},
		},
		{
			name:   "reolink-4",
			actual: "0200117c52746d7053616d706c6541636365737301010101",
			expect: []any{
				"|RtmpSampleAccess", true, true,
			},
		},
		{
			name:   "reolink-5",
			actual: "02000a6f6e4d6574614461746103000577696474680040a4000000000000000668656967687400409e000000000000000c646973706c617957696474680040a4000000000000000d646973706c617948656967687400409e00000000000000086475726174696f6e000000000000000000000c766964656f636f646563696400401c000000000000000c617564696f636f6465636964004024000000000000000f617564696f73616d706c65726174650040cf40000000000000096672616d657261746500403e000000000000000009",
			expect: []any{
				"onMetaData",
				map[string]any{
					"duration": float64(0),

					"videocodecid":  float64(7),
					"width":         float64(2560),
					"height":        float64(1920),
					"displayWidth":  float64(2560),
					"displayHeight": float64(1920),
					"framerate":     float64(30),

					"audiocodecid":    float64(10),
					"audiosamplerate": float64(16000),
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "02000d40736574446174614672616d6502000a6f6e4d6574614461746103000d766964656f6461746172617465000000000000000000000c766964656f636f646563696400401c000000000000000d617564696f6461746172617465000000000000000000000c617564696f636f6465636964004024000000000000000009",
			expect: []any{
				"@setDataFrame",
				"onMetaData",
				map[string]any{
					"videocodecid":  float64(7),
					"videodatarate": float64(0),
					"audiocodecid":  float64(10),
					"audiodatarate": float64(0),
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "0200075f726573756c74003ff0000000000000030006666d7356657202000d4c4e5820392c302c3132342c32000c6361706162696c697469657300403f0000000000000000090300056c6576656c0200067374617475730004636f646502001d4e6574436f6e6e656374696f6e2e436f6e6e6563742e53756363657373000b6465736372697074696f6e020015436f6e6e656374696f6e207375636365656465642e000e6f626a656374456e636f64696e67000000000000000000000009",
			expect: []any{
				"_result", float64(1), map[string]any{
					"capabilities": float64(31),
					"fmsVer":       "LNX 9,0,124,2",
				}, map[string]any{
					"code":           "NetConnection.Connect.Success",
					"description":    "Connection succeeded.",
					"level":          "status",
					"objectEncoding": float64(0),
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "0200075f726573756c7400401000000000000005003ff0000000000000",
			expect: []any{"_result", float64(4), any(nil), float64(1)},
		},
		{
			name:   "mediamtx",
			actual: "0200086f6e537461747573004014000000000000050300056c6576656c0200067374617475730004636f64650200144e657453747265616d2e506c61792e5265736574000b6465736372697074696f6e02000a706c6179207265736574000009",
			expect: []any{
				"onStatus", float64(5), any(nil), map[string]any{
					"code":        "NetStream.Play.Reset",
					"description": "play reset",
					"level":       "status",
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "0200086f6e537461747573004014000000000000050300056c6576656c0200067374617475730004636f64650200144e657453747265616d2e506c61792e5374617274000b6465736372697074696f6e02000a706c6179207374617274000009",
			expect: []any{
				"onStatus", float64(5), any(nil), map[string]any{
					"code":        "NetStream.Play.Start",
					"description": "play start",
					"level":       "status",
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "0200086f6e537461747573004014000000000000050300056c6576656c0200067374617475730004636f64650200144e657453747265616d2e446174612e5374617274000b6465736372697074696f6e02000a64617461207374617274000009",
			expect: []any{
				"onStatus", float64(5), any(nil), map[string]any{
					"code":        "NetStream.Data.Start",
					"description": "data start",
					"level":       "status",
				},
			},
		},
		{
			name:   "mediamtx",
			actual: "0200086f6e537461747573004014000000000000050300056c6576656c0200067374617475730004636f646502001c4e657453747265616d2e506c61792e5075626c6973684e6f74696679000b6465736372697074696f6e02000e7075626c697368206e6f74696679000009",
			expect: []any{
				"onStatus", float64(5), any(nil), map[string]any{
					"code":        "NetStream.Play.PublishNotify",
					"description": "publish notify",
					"level":       "status",
				},
			},
		},
		{
			name:   "obs-connect",
			actual: "020007636f6e6e656374003ff000000000000003000361707002000c617070312f73747265616d3100047479706502000a6e6f6e70726976617465000e737570706f727473476f4177617901010008666c61736856657202001f464d4c452f332e302028636f6d70617469626c653b20464d53632f312e3029000673776655726c02002272746d703a2f2f3139322e3136382e31302e3130312f617070312f73747265616d310005746355726c02002272746d703a2f2f3139322e3136382e31302e3130312f617070312f73747265616d31000009",
			expect: []any{
				"connect", float64(1),
				map[string]any{
					"app":            "app1/stream1",
					"flashVer":       "FMLE/3.0 (compatible; FMSc/1.0)",
					"supportsGoAway": true,
					"swfUrl":         "rtmp://192.168.10.101/app1/stream1",
					"tcUrl":          "rtmp://192.168.10.101/app1/stream1",
					"type":           "nonprivate",
				},
			},
		},
		{
			name:   "obs-key",
			actual: "02000d72656c6561736553747265616d004000000000000000050200046b657931",
			expect: []any{
				"releaseStream", float64(2), nil, "key1",
			},
		},
		{
			name:   "obs",
			actual: "02000d40736574446174614672616d6502000a6f6e4d65746144617461080000001400086475726174696f6e000000000000000000000866696c6553697a65000000000000000000000577696474680040840000000000000006686569676874004076800000000000000c766964656f636f646563696400401c000000000000000d766964656f64617461726174650040a388000000000000096672616d6572617465004039000000000000000c617564696f636f6465636964004024000000000000000d617564696f6461746172617465004064000000000000000f617564696f73616d706c65726174650040e5888000000000000f617564696f73616d706c6573697a65004030000000000000000d617564696f6368616e6e656c73004000000000000000000673746572656f01010003322e3101000003332e3101000003342e3001000003342e3101000003352e3101000003372e3101000007656e636f6465720200376f62732d6f7574707574206d6f64756c6520286c69626f62732076657273696f6e2032392e302e302d36322d6739303031323131663829000009",
			expect: []any{
				"@setDataFrame", "onMetaData", map[string]any{
					"2.1":             false,
					"3.1":             false,
					"4.0":             false,
					"4.1":             false,
					"5.1":             false,
					"7.1":             false,
					"audiochannels":   float64(2),
					"audiocodecid":    float64(10),
					"audiodatarate":   float64(160),
					"audiosamplerate": float64(44100),
					"audiosamplesize": float64(16),
					"duration":        float64(0),
					"encoder":         "obs-output module (libobs version 29.0.0-62-g9001211f8)",
					"fileSize":        float64(0),
					"framerate":       float64(25),
					"height":          float64(360),
					"stereo":          true,
					"videocodecid":    float64(7),
					"videodatarate":   float64(2500),
					"width":           float64(640),
				},
			},
		},
		{
			name:   "telegram-2",
			actual: "0200075f726573756c7400400000000000000005",
			expect: []any{
				"_result", float64(2), nil,
			},
		},
		{
			name:   "telegram-4",
			actual: "0200075f726573756c7400401000000000000005003ff0000000000000",
			expect: []any{
				"_result", float64(4), nil, float64(1),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, err := hex.DecodeString(test.actual)
			require.Nil(t, err)

			rd := NewReader(b)
			v, err := rd.ReadItems()
			require.Nil(t, err)

			require.Equal(t, test.expect, v)
		})
	}
}
