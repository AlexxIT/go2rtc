package dtls

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"time"
)

// msgSessionCloseCC51 follows the tested firmware's iotcRecordSendSessionClose
// and record encoder: client-local record SID 0, direct path 0, zero relay
// ticket, Unix seconds, full connection RID, then HMAC-SHA1 over all 28 bytes.
// The receiver locates and authenticates the session by the full eight-byte RID.
// No legacy-IOTC close packet is inferred from this CC51 evidence.
func (c *DTLSConn) msgSessionCloseCC51(now time.Time) []byte {
	b := make([]byte, 48)
	copy(b, magicCC51)
	binary.LittleEndian.PutUint16(b[4:], 0x1302)
	binary.LittleEndian.PutUint16(b[6:], 36)
	binary.LittleEndian.PutUint32(b[16:], uint32(now.Unix()))
	copy(b[20:28], c.sid)
	h := hmac.New(sha1.New, append([]byte(c.uid), c.authKey...))
	_, _ = h.Write(b[:28])
	copy(b[28:], h.Sum(nil))
	return b
}
