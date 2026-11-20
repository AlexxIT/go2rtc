package dtls

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
)

const rdtFileTestPath = "/media/mmc/record/20261030/12/34.mp4"

// Derived directly from firmware's application writes, not from the encoder.
// File bytes 00 7f 80 ff sum to -2 (signed bytes), wire checksum feffffff.
const rdtFileGoldenHex = "0100000000000000000000000500000000000000" + "0204000000" +
	"10000000040000000000000004000000feffffff" + "007f80ff"
const rdtFileRequestHex = "0000000000000000000000002d00000000000000" +
	"242f6d656469612f6d6d632f7265636f72642f32303236313033302f31322f33342e6d70340000000000000000"

func rdtFileUnhex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

type rdtFileFakeStream struct {
	reader   io.Reader
	request  bytes.Buffer
	short    bool
	writeErr error
}

func (f *rdtFileFakeStream) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *rdtFileFakeStream) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.short {
		return f.request.Write(p[:len(p)-1])
	}
	return f.request.Write(p)
}
func rdtFileFake(b []byte) *rdtFileFakeStream { return &rdtFileFakeStream{reader: bytes.NewReader(b)} }
func TestRDTFileGoldenRequestAndSignedChecksum(t *testing.T) {
	remaining := bytes.NewReader(append(rdtFileUnhex(rdtFileGoldenHex), []byte("next")...))
	stream := &rdtFileFakeStream{reader: remaining}
	var output bytes.Buffer
	n, err := downloadRDTFile(stream, rdtFileTestPath, &output, 1024)
	if err != nil || n != 4 || hex.EncodeToString(output.Bytes()) != "007f80ff" {
		t.Fatalf("download=%d,%v,%x", n, err, output.Bytes())
	}
	if hex.EncodeToString(stream.request.Bytes()) != rdtFileRequestHex {
		t.Fatalf("request differs from native format: %x", stream.request.Bytes())
	}
	tail, _ := io.ReadAll(remaining)
	if string(tail) != "next" {
		t.Fatal("read past exact file boundary")
	}
	if signedRDTFileChecksum([]byte{0x80}) != 0xffffff80 {
		t.Fatal("checksum treated 0x80 as unsigned")
	}
}
func rdtFileAppendHeader(b []byte, words [5]uint32) []byte {
	var header [20]byte
	for i, word := range words {
		binary.LittleEndian.PutUint32(header[i*4:], word)
	}
	return append(b, header[:]...)
}

type rdtFileFragmentReader struct{ reader io.Reader }

func (r rdtFileFragmentReader) Read(p []byte) (int, error) {
	if len(p) > 7 {
		p = p[:7]
	}
	return r.reader.Read(p)
}
func TestRDTFileMultipleBlocksAndFragmentedReads(t *testing.T) {
	fixture := rdtFileAppendHeader(nil, [5]uint32{1, 0, 0, 5, 0})
	fixture = append(fixture, 2, 4, 16, 0, 0) //4100bytes
	fixture = rdtFileAppendHeader(fixture, [5]uint32{16, 4100, 0, 4096, 0xfff80000})
	fixture = append(fixture, bytes.Repeat([]byte{0x80}, 4096)...)
	fixture = rdtFileAppendHeader(fixture, [5]uint32{16, 4100, 4096, 4, 0xfffffffe})
	fixture = append(fixture, 0, 0x7f, 0x80, 0xff)
	stream := &rdtFileFakeStream{reader: rdtFileFragmentReader{bytes.NewReader(fixture)}}
	var output bytes.Buffer
	n, err := downloadRDTFile(stream, "/media/mmc/record/20240229/23/59_new.mp4", &output, 4100)
	if err != nil || n != 4100 || output.Len() != 4100 || output.Bytes()[4096+1] != 0x7f {
		t.Fatalf("fragmented multi-block download=%d,%v", n, err)
	}
}

func TestRDTFileDataBeforeSyncGoldenOrders(t *testing.T) {
	// Independent application frames: two-byte file "AB", split into two DATA
	// blocks, with the successful SYNC interleaved by the firmware's other worker.
	syncFrame := "01000000000000000000000005000000000000000202000000"
	dataA := "100000000200000000000000010000004100000041"
	dataB := "100000000200000001000000010000004200000042"
	for _, test := range []struct {
		name, frames string
	}{
		{"before data", syncFrame + dataA + dataB},
		{"between data", dataA + syncFrame + dataB},
		{"after all data", dataA + dataB + syncFrame},
	} {
		t.Run(test.name, func(t *testing.T) {
			remaining := bytes.NewReader(append(rdtFileUnhex(test.frames), []byte("next")...))
			stream := &rdtFileFakeStream{reader: rdtFileFragmentReader{remaining}}
			var output bytes.Buffer
			n, err := downloadRDTFile(stream, rdtFileTestPath, &output, 2)
			if err != nil || n != 2 || output.String() != "AB" {
				t.Fatalf("download=%d,%v,%q", n, err, output.String())
			}
			tail, _ := io.ReadAll(remaining)
			if string(tail) != "next" {
				t.Fatal("consumed bytes after completed DATA and SYNC")
			}
		})
	}
}

func TestRDTFilePreSyncMetadataAndChecksumAreValidated(t *testing.T) {
	golden := rdtFileUnhex(rdtFileGoldenHex)
	for _, test := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"zero total", 4, 0}, {"total exceeds limit", 4, 1025},
		{"nonzero first offset", 8, 1}, {"zero chunk", 12, 0},
		{"oversized chunk", 12, 4097}, {"beyond total", 12, 5},
		{"wrong signed checksum", 16, 510},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(nil), golden[25:]...)
			binary.LittleEndian.PutUint32(data[test.offset:], test.value)
			fixture := append(data, golden[:25]...)
			var output bytes.Buffer
			n, err := downloadRDTFile(rdtFileFake(fixture), rdtFileTestPath, &output, 1024)
			if err == nil || n != 0 || output.Len() != 0 {
				t.Fatalf("invalid pre-SYNC DATA accepted: %d,%v,%x", n, err, output.Bytes())
			}
		})
	}
}

func TestRDTFileDataRequiresMatchingSuccessfulSync(t *testing.T) {
	golden := rdtFileUnhex(rdtFileGoldenHex)
	for _, test := range []struct {
		name string
		sync []byte
	}{
		{"missing", nil},
		{"truncated", golden[:24]},
		{"contradictory total", rdtFileUnhex("01000000000000000000000005000000000000000205000000")},
		{"rejected", rdtFileUnhex("01000000000000000000000005000000000000000504000000")},
		{"malformed", rdtFileUnhex("01000000010000000000000005000000000000000204000000")},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := append(append([]byte(nil), golden[25:]...), test.sync...)
			var output bytes.Buffer
			n, err := downloadRDTFile(rdtFileFake(fixture), rdtFileTestPath, &output, 1024)
			if err == nil || n != 4 || !bytes.Equal(output.Bytes(), golden[45:]) {
				t.Fatalf("DATA claimed completion without valid SYNC: %d,%v,%x", n, err, output.Bytes())
			}
			if (test.name == "missing" || test.name == "truncated") && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("absent SYNC did not report incomplete input: %v", err)
			}
		})
	}
	// Duplicate SYNC must fail while a transfer is incomplete. DATA totals may
	// likewise never change, including before the first SYNC has arrived.
	syncFrame := "01000000000000000000000005000000000000000202000000"
	dataA := "100000000200000000000000010000004100000041"
	for _, frames := range []string{
		syncFrame + dataA + syncFrame,
		dataA + "100000000300000001000000010000004200000042" + syncFrame,
	} {
		var output bytes.Buffer
		n, err := downloadRDTFile(rdtFileFake(rdtFileUnhex(frames)), rdtFileTestPath, &output, 1024)
		if err == nil || n != 1 || output.String() != "A" {
			t.Fatalf("contradictory frames accepted: %d,%v,%q", n, err, output.String())
		}
	}
}

func TestRDTFileRejectsInvalidPathsBeforeIO(t *testing.T) {
	paths := []string{
		"/media/mmc/other/20261030/12/34.mp4", "/media/mmc/time_lapse/time_Task_1/record.h264",
		"/media/mmc/record/20260229/12/34.mp4", "/media/mmc/record/20260431/12/34.mp4",
		"/media/mmc/record/00000101/12/34.mp4", "/media/mmc/record/20261030/24/34.mp4",
		"/media/mmc/record/20261030/12/60.mp4", "/media/mmc/record/20261030/1/34.mp4",
		"/media/mmc/record/20261030/12/34.MP4", "/media/mmc/record/20261030/12/34_new_new.mp4",
		"/media/mmc/record/20261030/12/../34.mp4", "/media/mmc/record/20261030/12/34.mp4\x00",
		"/media/mmc/record/20261030/12/34.mp4/", "/media/mmc/record/20261030/12/34.mp4\n",
		"/media/mmc/record/20261030/12/%33%34.mp4", "/media/mmc/record/２０２６１０３０/12/34.mp4",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			stream := rdtFileFake(nil)
			var output bytes.Buffer
			if _, err := downloadRDTFile(stream, path, &output, 1024); err == nil {
				t.Fatal("invalid path accepted")
			}
			if stream.request.Len() != 0 || output.Len() != 0 {
				t.Fatal("invalid path performed IO")
			}
		})
	}
}
func TestRDTFileRejectsLimitsAndMalformedHeaders(t *testing.T) {
	for _, limit := range []uint32{0, rdtFileMaxBytes + 1} {
		stream := rdtFileFake(nil)
		if _, err := downloadRDTFile(stream, rdtFileTestPath, io.Discard, limit); err == nil || stream.request.Len() != 0 {
			t.Fatal("invalid size limit performed IO")
		}
	}
	changes := []struct {
		name   string
		offset int
		value  uint32
	}{
		{"sync command", 0, 16}, {"sync filesize reserved", 4, 4}, {"sync index reserved", 8, 1}, {"sync payload oversized", 12, 6}, {"sync CRC reserved", 16, 1},
		{"size above limit", 21, 1025}, {"zero size", 21, 0}, {"data command", 25, 1}, {"changed total", 29, 5},
		{"post-block index", 33, 4}, {"zero chunk", 37, 0}, {"oversize chunk", 37, 4097}, {"beyond remaining", 37, 5}, {"unsigned CRC", 41, 510},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			fixture := rdtFileUnhex(rdtFileGoldenHex)
			binary.LittleEndian.PutUint32(fixture[change.offset:], change.value)
			var output bytes.Buffer
			n, err := downloadRDTFile(rdtFileFake(fixture), rdtFileTestPath, &output, 1024)
			if err == nil || n != 0 || output.Len() != 0 {
				t.Fatal("malformed response wrote bytes or succeeded")
			}
		})
	}
	for _, status := range []byte{0, 3, 5, 99} {
		fixture := rdtFileUnhex(rdtFileGoldenHex)
		fixture[20] = status
		if status == 3 {
			binary.LittleEndian.PutUint32(fixture[21:], 0)
		}
		_, err := downloadRDTFile(rdtFileFake(fixture), rdtFileTestPath, io.Discard, 1024)
		if err == nil {
			t.Fatal("rejected/missing start accepted")
		}
		if (status == 0 || status == 3) && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("unavailable/empty reply not distinguished")
		}
	}
}
func TestRDTFileAllTruncationsFailWithoutWritingBlock(t *testing.T) {
	fixture := rdtFileUnhex(rdtFileGoldenHex)
	for length := 0; length < len(fixture); length++ {
		var output bytes.Buffer
		n, err := downloadRDTFile(rdtFileFake(fixture[:length]), rdtFileTestPath, &output, 1024)
		if !errors.Is(err, io.ErrUnexpectedEOF) || n != 0 || output.Len() != 0 {
			t.Fatalf("truncation at%d: %d,%v", length, n, err)
		}
	}
}

type rdtFileOutputFailure struct {
	n   int
	err error
}

func (f rdtFileOutputFailure) Write(p []byte) (int, error) { return f.n, f.err }
func TestRDTFileShortWritesAndIOErrors(t *testing.T) {
	failure := errors.New("fixture IO failure")
	stream := rdtFileFake(nil)
	stream.short = true
	if _, err := downloadRDTFile(stream, rdtFileTestPath, io.Discard, 1024); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short request write not rejected")
	}
	stream = rdtFileFake(nil)
	stream.writeErr = failure
	if _, err := downloadRDTFile(stream, rdtFileTestPath, io.Discard, 1024); !errors.Is(err, failure) {
		t.Fatal("request write error not propagated")
	}
	for _, test := range []struct {
		output  rdtFileOutputFailure
		wantN   uint32
		wantErr error
	}{
		{rdtFileOutputFailure{2, nil}, 2, io.ErrShortWrite}, {rdtFileOutputFailure{2, failure}, 2, failure},
		{rdtFileOutputFailure{4, failure}, 4, failure},
	} {
		n, err := downloadRDTFile(rdtFileFake(rdtFileUnhex(rdtFileGoldenHex)), rdtFileTestPath, test.output, 1024)
		if n != test.wantN || !errors.Is(err, test.wantErr) {
			t.Fatalf("output failure=%d,%v", n, err)
		}
	}
	for _, n := range []int{-1, 5} {
		if _, err := downloadRDTFile(rdtFileFake(rdtFileUnhex(rdtFileGoldenHex)), rdtFileTestPath, rdtFileOutputFailure{n, nil}, 1024); err == nil {
			t.Fatal("invalid writer count accepted")
		}
	}
}
func TestRDTFileCorruptLaterBlockKeepsOnlyVerifiedPrefix(t *testing.T) {
	fixture := rdtFileAppendHeader(nil, [5]uint32{1, 0, 0, 5, 0})
	fixture = append(fixture, 2, 2, 0, 0, 0)
	fixture = rdtFileAppendHeader(fixture, [5]uint32{16, 2, 0, 1, 65})
	fixture = append(fixture, 'A')
	fixture = rdtFileAppendHeader(fixture, [5]uint32{16, 2, 1, 1, 65})
	fixture = append(fixture, 'B')
	var output strings.Builder
	n, err := downloadRDTFile(rdtFileFake(fixture), rdtFileTestPath, &output, 1024)
	if n != 1 || err == nil || output.String() != "A" {
		t.Fatalf("corrupt later block=%d,%v,%q", n, err, output.String())
	}
}
