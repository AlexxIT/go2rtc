package app

import (
	"io"
	"os"

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
