package baichuan

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"
)

func TestParseExtensionFastPathAndFallback(t *testing.T) {
	fast := []byte(`<Extension version="1.1"><binaryData>1</binaryData><encryptLen>+32</encryptLen><checkPos>-7</checkPos></Extension>`)
	if _, ok := fastExtensionAtoi(fast); !ok {
		t.Fatal("valid integer extension missed fast path")
	}
	wantFast := extension{BinaryData: intPointer(1), EncryptLen: intPointer(32), CheckPos: intPointer(-7)}
	got, err := parseExtension(fast)
	if err != nil || !reflect.DeepEqual(got, wantFast) {
		t.Fatalf("fast extension = %+v, %v; want %+v", got, err, wantFast)
	}

	fallback := []byte(`<body><!-- retained XML behavior --><binaryData>1</binaryData><encryptLen>16</encryptLen></body>`)
	var wantFallback extension
	if err := xml.Unmarshal(fallback, &wantFallback); err != nil {
		t.Fatal(err)
	}
	if _, ok := fastExtensionAtoi(fallback); ok {
		t.Fatal("comment unexpectedly matched fast path")
	}
	got, err = parseExtension(fallback)
	if err != nil || !reflect.DeepEqual(got, wantFallback) {
		t.Fatalf("fallback extension = %+v, %v; want %+v", got, err, wantFallback)
	}

	partial := []byte(`<body><binaryData>1</binaryData><encryptLen>bad</encryptLen></body>`)
	var wantPartial extension
	wantErr := xml.Unmarshal(partial, &wantPartial)
	if wantErr == nil {
		t.Fatal("invalid integer unexpectedly decoded")
	}
	got, err = parseExtension(partial)
	if err == nil || err.Error() != "decode extension XML: "+wantErr.Error() ||
		!reflect.DeepEqual(got, wantPartial) {
		t.Fatalf("partial fallback = %+v, %v; want %+v, %v", got, err, wantPartial, wantErr)
	}
}

func intPointer(value int) *int { return &value }

func TestParseNonceLimits(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(strings.Repeat(" ", maxNonceBody+1)),
		[]byte(`<body><Encryption><nonce>` + strings.Repeat("n", maxNonceSize+1) +
			`</nonce></Encryption></body>`),
	} {
		if _, err := parseNonce(body); err == nil {
			t.Fatal("accepted oversized login nonce")
		}
	}
}

func FuzzXMLParsers(f *testing.F) {
	f.Add([]byte(`<body><Encryption><nonce>nonce</nonce></Encryption></body>`))
	f.Add([]byte(`<Extension><binaryData>1</binaryData><encryptLen>8</encryptLen></Extension>`))
	f.Add([]byte(`<body><TalkAbility version="1.1"><duplexList><duplex>fullDuplex</duplex></duplexList></TalkAbility></body>`))
	f.Add([]byte(`<body><AbilityInfo><system><subModule><abilityValue>version_ro</abilityValue></subModule></system></AbilityInfo></body>`))
	f.Add([]byte(`<body><binaryData>1</binaryData><binaryData>0</binaryData></body>`))
	f.Add([]byte(`<body><encryptLen>9223372036854775808</encryptLen></body>`))
	f.Add([]byte(`<body><binaryData>1</binaryData><encryptLen>`))
	f.Add([]byte(`<body><checkPos>-2147483648</checkPos></body>`))
	f.Add([]byte(`<body><binaryData>1</binaryData><futureMode>active</futureMode></body>`))
	f.Fuzz(func(t *testing.T, body []byte) {
		var want extension
		wantError := ""
		if len(body) != 0 {
			if err := xml.Unmarshal(body, &want); err != nil {
				wantError = "decode extension XML: " + err.Error()
			}
		}
		got, gotErr := parseExtension(body)
		gotError := ""
		if gotErr != nil {
			gotError = gotErr.Error()
		}
		if !reflect.DeepEqual(got, want) || gotError != wantError {
			t.Fatalf("extension mismatch for %d bytes: error=%q, want %q", len(body), gotError, wantError)
		}
		_, _ = parseNonce(body)
		_, _ = decodeTalkAbility(body)
		_, _ = parseCapabilities(body)
		_, _ = parseDeviceInfo(body)
	})
}
