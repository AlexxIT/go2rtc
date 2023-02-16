package mpegts

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTime(t *testing.T) {
	w := NewWriter()
	w.WriteTime(0xFFFFFFFF)
	assert.Equal(t, []byte{0x27, 0xFF, 0xFF, 0xFF, 0xFF}, w.Bytes())
	ts := ParseTime(w.Bytes())
	assert.Equal(t, uint32(0xFFFFFFFF), ts)
}
