package dtls

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"time"
)

const rdtFileMaxBytes uint32 = 64 * 1024 * 1024
const rdtFileMaxChunk uint32 = 4096

var rdtRecordingPath = regexp.MustCompile(`^/media/mmc/record/([0-9]{8})/([0-9]{2})/([0-9]{2})(_new)?\.mp4$`)

// These are the application header words INSIDE an RDT byte stream, not the
// magic-prefixed RDT transport header. Evidence: firmware 4.36.16.6114,
// FUN_00451650 (command0/SYNC), FUN_004512cc (DATA), and Android manager3's
// command0 path payload; see RDT_EXPERIMENTAL.md.
type rdtFileHeader [5]uint32

// downloadRDTFile downloads one existing, strictly named normal SD recording.
// It does not open a connection, own/close stream or output, or set deadlines.
// Caller must impose operation deadlines and discard/close the stream after any
// error: a partially consumed protocol stream cannot be reused safely.
// It may leave a verified prefix in output on failure; returned count is the
// number of bytes output accepted. There is no resume or filesystem path access here.
func downloadRDTFile(stream io.ReadWriter, path string, output io.Writer, maxBytes uint32) (uint32, error) {
	if stream == nil || output == nil {
		return 0, errors.New("RDT file stream and output are required")
	}
	if err := ValidateRDTFileRequest(path, maxBytes); err != nil {
		return 0, err
	}
	return transferRDTFile(stream, path, output, maxBytes)
}

// ValidateRDTFileRequest supports preflight before changing channel state.
func ValidateRDTFileRequest(path string, maxBytes uint32) error {
	if maxBytes == 0 || maxBytes > rdtFileMaxBytes {
		return errors.New("RDT file limit must be between 1 byte and 64 MiB")
	}
	match := rdtRecordingPath.FindStringSubmatch(path)
	if match == nil {
		return errors.New("RDT path is not a normal SD recording")
	}
	timestamp, err := time.Parse("20060102/15/04", match[1]+"/"+match[2]+"/"+match[3])
	if err != nil || timestamp.Year() == 0 {
		return errors.New("RDT recording path has invalid calendar time")
	}
	// Paths are ASCII and short enough for the native signed-char length field.
	if len(path) > 127 {
		return errors.New("RDT recording path is too long")
	}
	return nil
}

func transferRDTFile(stream io.ReadWriter, path string, output io.Writer, maxBytes uint32) (uint32, error) {
	payload := make([]byte, 1+len(path)+8)
	payload[0] = byte(len(path))
	copy(payload[1:], path)
	// Final eight bytes are zero resume offset and zero rolling checksum, exactly
	// as manager3 emits. Firmware consumes offset from immediately after the path.
	request := make([]byte, 20+len(payload))
	binary.LittleEndian.PutUint32(request[12:], uint32(len(payload)))
	copy(request[20:], payload)
	if err := writeRDTFileBytes(stream, request); err != nil {
		return 0, fmt.Errorf("RDT file request: %w", err)
	}

	// Firmware starts the file reader before queueing SYNC. Its DATA writes can
	// therefore arrive first, even though RDT itself preserves byte ordering.
	// Stream verified blocks immediately using their advertised total, then require
	// exactly one successful SYNC with that same total before claiming completion.
	// A failed or absent SYNC may leave a verified prefix, as with any later error.
	var size, downloaded uint32
	var synced bool
	block := make([]byte, rdtFileMaxChunk)
	for !synced || downloaded < size {
		header, err := readRDTFileHeader(stream)
		if err != nil {
			if !synced {
				return downloaded, fmt.Errorf("RDT file sync header: %w", err)
			}
			return downloaded, fmt.Errorf("RDT file data header: %w", err)
		}
		if header[0] == 1 {
			if synced || header != (rdtFileHeader{1, 0, 0, 5, 0}) {
				return downloaded, errors.New("invalid RDT file sync header")
			}
			var syncPayload [5]byte
			if _, err = io.ReadFull(stream, syncPayload[:]); err != nil {
				return downloaded, fmt.Errorf("RDT file sync payload: %w", rdtFileReadError(err))
			}
			syncSize := binary.LittleEndian.Uint32(syncPayload[1:])
			if size != 0 && size != syncSize {
				return downloaded, errors.New("invalid RDT file sync header")
			}
			switch syncPayload[0] {
			case 2:
				if syncSize == 0 {
					return downloaded, errors.New("RDT file has indeterminate zero size")
				}
			case 0, 3:
				// Firmware initial fopen failure yields size0 then offset0==size0
				// status3. This cannot establish a real empty export.
				return downloaded, fmt.Errorf("RDT recording unavailable or empty: %w", fs.ErrNotExist)
			default:
				return downloaded, errors.New("RDT file start was rejected")
			}
			if syncSize > maxBytes {
				return downloaded, errors.New("RDT file exceeds download limit")
			}
			size, synced = syncSize, true
			continue
		}
		if size == 0 {
			if header[0] != 16 || header[1] == 0 {
				return downloaded, errors.New("inconsistent RDT file data header")
			}
			if header[1] > maxBytes {
				return downloaded, errors.New("RDT file exceeds download limit")
			}
			size = header[1]
		}
		length := header[3]
		// Firmware fills index with the current offset AFTER fread but BEFORE it
		// advances offset by fread length. Therefore index is the block's start.
		if header[0] != 16 || header[1] != size || header[2] != downloaded || length == 0 || length > rdtFileMaxChunk || length > size-downloaded {
			return downloaded, errors.New("inconsistent RDT file data header")
		}
		data := block[:int(length)]
		if _, err = io.ReadFull(stream, data); err != nil {
			return downloaded, fmt.Errorf("RDT file data payload: %w", rdtFileReadError(err))
		}
		if signedRDTFileChecksum(data) != header[4] {
			return downloaded, errors.New("RDT file checksum mismatch")
		}
		n, err := output.Write(data)
		if n < 0 || n > len(data) {
			return downloaded, errors.New("invalid RDT output write count")
		}
		downloaded += uint32(n)
		if err != nil {
			return downloaded, fmt.Errorf("RDT file output: %w", err)
		}
		if n != len(data) {
			return downloaded, io.ErrShortWrite
		}
	}
	// Completion requires both the exact advertised size and a matching SYNC,
	// not EOF. Do not consume a possible subsequent application message.
	return downloaded, nil
}

func signedRDTFileChecksum(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(int32(int8(b)))
	}
	return sum
}
func readRDTFileHeader(reader io.Reader) (rdtFileHeader, error) {
	var header rdtFileHeader
	var raw [20]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return header, rdtFileReadError(err)
	}
	for i := range header {
		header[i] = binary.LittleEndian.Uint32(raw[i*4:])
	}
	return header, nil
}
func rdtFileReadError(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}
func writeRDTFileBytes(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
