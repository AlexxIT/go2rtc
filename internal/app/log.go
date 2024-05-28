package app

import (
	"io"
	"os"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var MemoryLog = newBuffer(16)

func NewLogger(config map[string]string) zerolog.Logger {
	var writer io.Writer

	// support output only to memory
	switch config["output"] {
	case "stderr":
		writer = os.Stderr
	case "stdout":
		writer = os.Stdout
	}

	timeFormat := config["time"]

	if writer != nil {
		switch format := config["format"]; format {
		case "color", "text":
			if timeFormat != "" {
				writer = &zerolog.ConsoleWriter{
					Out:        writer,
					NoColor:    format == "text" || !shell.IsInteractive(os.Stdout.Fd()),
					TimeFormat: "15:04:05.000",
				}
			} else {
				writer = &zerolog.ConsoleWriter{
					Out:     writer,
					NoColor: format == "text" || !shell.IsInteractive(os.Stdout.Fd()),
					PartsOrder: []string{
						zerolog.LevelFieldName,
						zerolog.CallerFieldName,
						zerolog.MessageFieldName,
					},
				}
			}
		case "json": // none
		}

		writer = zerolog.MultiLevelWriter(writer, MemoryLog)
	} else {
		writer = MemoryLog
	}

	logger := zerolog.New(writer)

	if timeFormat != "" {
		zerolog.TimeFieldFormat = timeFormat
		logger = logger.With().Timestamp().Logger()
	}

	lvl, _ := zerolog.ParseLevel(config["level"])
	return logger.Level(lvl)
}

func GetLogger(module string) zerolog.Logger {
	if s, ok := modules[module]; ok {
		lvl, err := zerolog.ParseLevel(s)
		if err == nil {
			return log.Level(lvl)
		}
		log.Warn().Err(err).Caller().Send()
	}

	return log.Logger
}

// modules log levels
var modules map[string]string

const chunkSize = 1 << 16

type circularBuffer struct {
	chunks [][]byte
	r, w   int
}

func newBuffer(chunks int) *circularBuffer {
	b := &circularBuffer{chunks: make([][]byte, 0, chunks)}
	// create first chunk
	b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
	return b
}

func (b *circularBuffer) Write(p []byte) (n int, err error) {
	n = len(p)

	// check if chunk has size
	if len(b.chunks[b.w])+n > chunkSize {
		// increase write chunk index
		if b.w++; b.w == cap(b.chunks) {
			b.w = 0
		}
		// check overflow
		if b.r == b.w {
			// increase read chunk index
			if b.r++; b.r == cap(b.chunks) {
				b.r = 0
			}
		}
		// check if current chunk exists
		if b.w == len(b.chunks) {
			// allocate new chunk
			b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
		} else {
			// reset len of current chunk
			b.chunks[b.w] = b.chunks[b.w][:0]
		}
	}

	b.chunks[b.w] = append(b.chunks[b.w], p...)
	return
}

func (b *circularBuffer) WriteTo(w io.Writer) (n int64, err error) {
	for i := b.r; ; {
		var nn int
		if nn, err = w.Write(b.chunks[i]); err != nil {
			return
		}
		n += int64(nn)

		if i == b.w {
			break
		}
		if i++; i == cap(b.chunks) {
			i = 0
		}
	}
	return
}

func (b *circularBuffer) Reset() {
	b.chunks[0] = b.chunks[0][:0]
	b.r = 0
	b.w = 0
}
