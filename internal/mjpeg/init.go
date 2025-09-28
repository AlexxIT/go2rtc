package mjpeg

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/ascii"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpjpeg"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			SnapshotCache                bool `yaml:"snapshot_cache"`
			SnapshotCacheTimeout         int  `yaml:"snapshot_cache_timeout"`
			SnapshotServeCachedByDefault bool `yaml:"snapshot_serve_cached_by_default"`
		} `yaml:"mjpeg"`
	}

	// Defaults
	cfg.Mod.SnapshotCache = true
	cfg.Mod.SnapshotCacheTimeout = 600
	cfg.Mod.SnapshotServeCachedByDefault = false

	app.LoadConfig(&cfg)

	// Store global config
	snapshotCacheEnabled = cfg.Mod.SnapshotCache
	snapshotCacheTimeout = time.Duration(cfg.Mod.SnapshotCacheTimeout) * time.Second
	snapshotServeCachedByDefault = cfg.Mod.SnapshotServeCachedByDefault

	// Handle special values
	if cfg.Mod.SnapshotCacheTimeout < 0 {
		snapshotCacheEnabled = false
	}

	api.HandleFunc("api/frame.jpeg", handlerKeyframe)
	api.HandleFunc("api/stream.mjpeg", handlerStream)
	api.HandleFunc("api/stream.ascii", handlerStream)
	api.HandleFunc("api/stream.y4m", apiStreamY4M)

	ws.HandleFunc("mjpeg", handlerWS)

	log = app.GetLogger("mjpeg")
}

var log zerolog.Logger

var (
	snapshotCacheEnabled         bool
	snapshotCacheTimeout         time.Duration
	snapshotServeCachedByDefault bool
)

func getSnapshotCacheTimeout() time.Duration {
	if !snapshotCacheEnabled {
		return -1 // disabled
	}
	return snapshotCacheTimeout
}

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	stream, _ := streams.GetOrPatch(r.URL.Query())
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	query := r.URL.Query()

	// Determine if client wants cached snapshot
	// Priority: query param > global config
	allowCached := snapshotServeCachedByDefault
	if query.Has("cached") {
		allowCached = query.Get("cached") != "false" && query.Get("cached") != "0"
	}

	// Start/reset snapshot cache (if enabled)
	stream.TouchSnapshotCache(getSnapshotCacheTimeout(), transcodeToJPEG)

	// Try to serve from cache if allowed
	if allowCached {
		if b, timestamp, exists := stream.GetCachedSnapshot(); exists {
			age := time.Since(timestamp)

			log.Trace().
				Dur("age_ms", age).
				Int("size", len(b)).
				Msg("[mjpeg] serving cached snapshot")

			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.Header().Set("X-Snapshot-Age-Ms", strconv.Itoa(int(age.Milliseconds())))
			w.Header().Set("X-Snapshot-Timestamp", timestamp.Format(time.RFC3339Nano))
			w.Header().Set("X-Snapshot-Cached", "true")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "close")
			w.Header().Set("Pragma", "no-cache")

			if _, err := w.Write(b); err != nil {
				log.Error().Err(err).Caller().Send()
			}
			return
		}
	}

	// Client wants fresh snapshot OR no cache available yet
	// Use traditional blocking approach
	log.Debug().Bool("allow_cached", allowCached).Msg("[mjpeg] fetching fresh snapshot")

	cons := magic.NewKeyframe()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	once := &core.OnceBuffer{}
	_, _ = cons.WriteTo(once)
	b := once.Buffer()

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		ts := time.Now()
		var err error
		if b, err = ffmpeg.JPEGWithQuery(b, query); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Msgf("[mjpeg] transcoding time=%s", time.Since(ts))
	case core.CodecJPEG:
		b = mjpeg.FixJPEG(b)
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("X-Snapshot-Age-Ms", "0")
	w.Header().Set("X-Snapshot-Timestamp", time.Now().Format(time.RFC3339Nano))
	w.Header().Set("X-Snapshot-Cached", "false")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")
	w.Header().Set("Pragma", "no-cache")

	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		outputMjpeg(w, r)
	} else {
		inputMjpeg(w, r)
	}
}

func outputMjpeg(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := mjpeg.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mjpeg] add consumer")
		return
	}

	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	if strings.HasSuffix(r.URL.Path, "mjpeg") {
		wr := mjpeg.NewWriter(w)
		_, _ = cons.WriteTo(wr)
	} else {
		cons.FormatName = "ascii"

		query := r.URL.Query()
		wr := ascii.NewWriter(w, query.Get("color"), query.Get("back"), query.Get("text"))
		_, _ = cons.WriteTo(wr)
	}

	stream.RemoveConsumer(cons)
}

func inputMjpeg(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	prod, _ := mpjpeg.Open(r.Body)
	prod.WithRequest(r)

	stream.AddProducer(prod)

	if err := prod.Start(); err != nil && err != io.EOF {
		log.Warn().Err(err).Caller().Send()
	}

	stream.RemoveProducer(prod)
}

func handlerWS(tr *ws.Transport, _ *ws.Message) error {
	stream, _ := streams.GetOrPatch(tr.Request.URL.Query())
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := mjpeg.NewConsumer()
	cons.WithRequest(tr.Request)

	if err := stream.AddConsumer(cons); err != nil {
		log.Debug().Err(err).Msg("[mjpeg] add consumer")
		return err
	}

	tr.Write(&ws.Message{Type: "mjpeg"})

	go cons.WriteTo(tr.Writer())

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}

func apiStreamY4M(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := y4m.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)
}

// transcodeToJPEG is injected into snapshot cache to avoid import cycles
func transcodeToJPEG(b []byte, codecName string) ([]byte, error) {
	switch codecName {
	case core.CodecH264, core.CodecH265:
		// Transcode via FFmpeg (no query params for cached version)
		return ffmpeg.JPEGWithScale(b, -1, -1)

	case core.CodecJPEG:
		// Fix JPEG headers if needed
		return mjpeg.FixJPEG(b), nil

	case core.CodecRAW:
		// Should already be encoded by Encoder(), skip
		return nil, errors.New("raw codec not supported in cache")

	default:
		// Unsupported codec
		return nil, errors.New("unsupported codec: " + codecName)
	}
}
