package app

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
)

var MemoryLog = newBuffer(16)

func GetLogger(module string) zerolog.Logger {
	if s, ok := modules[module]; ok {
		lvl, err := zerolog.ParseLevel(s)
		if err == nil {
			return Logger.Level(lvl)
		}
		Logger.Warn().Err(err).Caller().Send()
	}

	return Logger
}

// initLogger support:
// - output: empty (only to memory), stderr, stdout
// - format: empty (autodetect color support), color, json, text
// - time:   empty (disable timestamp), UNIXMS, UNIXMICRO, UNIXNANO
// - level:  disabled, trace, debug, info, warn, error...
func initLogger() {
	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	cfg.Mod = modules // defaults

	LoadConfig(&cfg)

	var writer io.Writer

	switch modules["output"] {
	case "stderr":
		writer = os.Stderr
	case "stdout":
		writer = os.Stdout
	}

	timeFormat := modules["time"]

	if writer != nil {
		if format := modules["format"]; format != "json" {
			console := &zerolog.ConsoleWriter{Out: writer}

			switch format {
			case "text":
				console.NoColor = true
			case "color":
				console.NoColor = false // useless, but anyway
			default:
				// autodetection if output support color
				// go-isatty - dependency for go-colorable - dependency for ConsoleWriter
				console.NoColor = !isatty.IsTerminal(writer.(*os.File).Fd())
			}

			if timeFormat != "" {
				console.TimeFormat = "15:04:05.000"
			} else {
				console.PartsOrder = []string{
					zerolog.LevelFieldName,
					zerolog.CallerFieldName,
					zerolog.MessageFieldName,
				}
			}

			writer = console
		}

		writer = zerolog.MultiLevelWriter(writer, MemoryLog)
	} else {
		writer = MemoryLog
	}

	lvl, _ := zerolog.ParseLevel(modules["level"])
	Logger = zerolog.New(writer).Level(lvl)

	if timeFormat != "" {
		zerolog.TimeFieldFormat = timeFormat
		Logger = Logger.With().Timestamp().Logger()
	}
}

var Logger zerolog.Logger

// modules log levels
var modules = map[string]string{
	"format": "", // useless, but anyway
	"level":  "info",
	"output": "stdout", // TODO: change to stderr someday
	"time":   zerolog.TimeFormatUnixMs,
}

const chunkSize = 1 << 16

type circularBuffer struct {
	chunks [][]byte
	r, w   int
	mu     sync.Mutex
}

func newBuffer(chunks int) *circularBuffer {
	b := &circularBuffer{chunks: make([][]byte, 0, chunks)}
	// create first chunk
	b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
	return b
}

func (b *circularBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n = len(p)
	if n == 0 {
		return 0, nil
	}

	if len(b.chunks) == 0 {
		b.chunks = append(b.chunks, make([]byte, 0, chunkSize))
	}

	// check if chunk has size
	if len(b.chunks[b.w])+n > chunkSize {
		// increase write chunk index
		b.w++
		if b.w == cap(b.chunks) {
			b.w = 0
		}
		// check overflow
		if b.r == b.w {
			return 0, errors.New("circularBuffer overflow, cannot write without overwriting unread data")
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
	return n, nil
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

func (b *circularBuffer) Reset() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.chunks {
		b.chunks[i] = b.chunks[i][:0]
	}

	b.r = 0
	b.w = 0

	return nil
}

// Bytes concatenates all chunks into a single byte slice.
func (b *circularBuffer) Bytes() []byte {
	var totalLen int
	for _, chunk := range b.chunks {
		totalLen += len(chunk)
	}
	result := make([]byte, 0, totalLen)

	for i := b.r; ; {
		result = append(result, b.chunks[i]...)

		if i == b.w {
			break
		}

		i++
		if i == len(b.chunks) {
			i = 0
		}
	}
	return result
}
