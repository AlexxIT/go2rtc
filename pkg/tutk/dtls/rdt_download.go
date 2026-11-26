package dtls

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"
)

// DownloadRDTFile opens and closes one separate authenticated channel-10
// connection. The caller enables/disables the ephemeral camera RDT server.
func (c *DTLSConn) DownloadRDTFile(psk []byte, path string, output io.Writer, max uint32) (count uint32, result error) {
	if err := ValidateRDTFileRequest(path, max); err != nil {
		return 0, err
	}
	if c == nil || output == nil {
		return 0, errors.New("RDT transport and output required")
	}
	c.rdtMu.Lock()
	defer c.rdtMu.Unlock()
	defer c.closeRDTHandshakeLocked()
	if err := c.openRDTChannelLocked(psk); err != nil {
		return 0, err
	}
	c.mu.RLock()
	conn := c.rdtConn
	c.mu.RUnlock()
	if conn == nil {
		return 0, errors.New("RDT secure channel is unavailable")
	}
	stream, err := newRDTSession(conn, 1)
	if err != nil {
		return 0, errors.New("RDT file transfer failed (stage=hello)")
	}
	defer func() {
		if err := stream.Close(); err != nil && result == nil {
			cause := "transport"
			if errors.Is(err, io.EOF) {
				cause = "peer_closed"
			}
			if errors.Is(err, errRDTRetry) {
				cause = "ack_timeout"
			}
			result = errors.New("RDT file transfer failed (stage=shutdown, cause=" + cause + ")")
		}
	}()
	deadline := time.Now().Add(45 * time.Second)
	_ = stream.SetReadDeadline(deadline)
	_ = stream.SetWriteDeadline(deadline)
	count, err = downloadRDTFile(stream, path, output, max)
	if err != nil {
		return count, safeRDTFileFailure(err)
	}
	return count, nil
}

// Match only local helper messages. Never interpolate an upstream error: reads
// and output writers may contain arbitrary text, including private information.
func safeRDTFileFailure(err error) error {
	message := err.Error()
	if strings.HasPrefix(message, "RDT recording unavailable or empty: ") && errors.Is(err, fs.ErrNotExist) {
		return fs.ErrNotExist
	}
	stage := "unknown"
	switch {
	case strings.HasPrefix(message, "RDT file request: "):
		stage = "request_write"
	case strings.HasPrefix(message, "RDT file sync header: "):
		stage = "sync_header_read"
	case message == "invalid RDT file sync header":
		stage = "sync_header_unexpected"
	case strings.HasPrefix(message, "RDT file sync payload: "):
		stage = "sync_payload_read"
	case message == "RDT file has indeterminate zero size":
		stage = "size_zero"
	case message == "RDT file start was rejected":
		stage = "start_rejected"
	case message == "RDT file exceeds download limit":
		stage = "size_limit"
	case strings.HasPrefix(message, "RDT file data header: "):
		stage = "data_header_read"
	case message == "inconsistent RDT file data header":
		stage = "data_header_unexpected"
	case strings.HasPrefix(message, "RDT file data payload: "):
		stage = "data_payload_read"
	case message == "RDT file checksum mismatch":
		stage = "checksum"
	case message == "invalid RDT output write count":
		stage = "output_write_count"
	case strings.HasPrefix(message, "RDT file output: "):
		stage = "output_write"
	case errors.Is(err, io.ErrShortWrite):
		stage = "output_short_write"
	}
	if errors.Is(err, errRDTRetry) {
		return errors.New("RDT file transfer failed (stage=" + stage + ", cause=ack_timeout)")
	}
	return errors.New("RDT file transfer failed (stage=" + stage + ")")
}
