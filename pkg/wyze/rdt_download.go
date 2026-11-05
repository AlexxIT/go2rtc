package wyze

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/AlexxIT/go2rtc/pkg/tutk/dtls"
)

// DownloadSDFile retrieves one strictly validated existing SD file using the
// authenticated LAN session. The lower-level RDT transfer enforces path and size
// limits. The caller owns the local output writer and any partial-file cleanup.
// No resolution, video/audio channel, cloud request or camera storage write is used.
func (c *Client) DownloadSDFile(path string, output io.Writer, max uint32) (count uint32, result error) {
	if c == nil {
		return 0, errors.New("authenticated connection required")
	}
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed || c.conn == nil || c.enr == "" {
		return 0, errors.New("authenticated connection required")
	}
	if output == nil || max == 0 || max > 64*1024*1024 {
		return 0, errors.New("file download requires an output writer and a size limit within 64 MiB")
	}
	if err := dtls.ValidateRDTFileRequest(path, max); err != nil {
		return 0, err
	}
	// All path and size validation finishes before the first control command.
	// Always attempt cleanup even when enable acknowledgement or transfer is lost.
	defer func() {
		if err := c.setRDTEnabled(false); err != nil {
			result = errors.Join(result, fmt.Errorf("RDT disable failed: %w", err))
		}
	}()
	if err := c.setRDTEnabled(true); err != nil {
		return 0, fmt.Errorf("RDT enable failed: %w", err)
	}
	psk := sha256.Sum256([]byte(c.enr))
	count, err := c.conn.DownloadRDTFile(psk[:], path, output, max)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return count, fs.ErrNotExist
		}
		// DownloadRDTFile maps every transfer failure to a fixed local stage.
		// It never returns peer text or arbitrary output writer error details.
		return count, err
	}
	return count, nil
}
