package wyoming

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/expr"
	"github.com/AlexxIT/go2rtc/pkg/wav"
)

type env struct {
	*satellite
	Type string
	Data string
}

func (s *satellite) handleEvent(evt *Event) {
	switch evt.Type {
	case "describe":
		// {"asr": [], "tts": [], "handle": [], "intent": [], "wake": [], "satellite": {"name": "my satellite", "attribution": {"name": "", "url": ""}, "installed": true, "description": "my satellite", "version": "1.4.1", "area": null, "snd_format": null}}
		data := fmt.Sprintf(`{"satellite":{"name":%q,"attribution":{"name":"go2rtc","url":"https://github.com/AlexxIT/go2rtc"},"installed":true}}`, s.srv.Name)
		s.WriteEvent("info", data)
	case "run-satellite":
		s.Detect()
	case "pause-satellite":
		s.Stop()
	case "detect": // WAKE_WORD_START {"names": null}
	case "detection": // WAKE_WORD_END {"name": "ok_nabu_v0.1", "timestamp": 17580, "speaker": null}
	case "transcribe": // STT_START {"language": "en"}
	case "voice-started": // STT_VAD_START {"timestamp": 1160}
	case "voice-stopped": // STT_VAD_END {"timestamp": 2470}
		s.Pause()
	case "transcript": // STT_END {"text": "how are you"}
	case "synthesize": // TTS_START {"text": "Sorry, I couldn't understand that", "voice": {"language": "en"}}
	case "audio-start": // TTS_END {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
	case "audio-chunk": // {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
	case "audio-stop": // {"timestamp": 2.880000000000002}
		// run async because PlayAudio takes some time
		go func() {
			s.PlayAudio()
			s.WriteEvent("played")
			s.Detect()
		}()
	case "error":
		s.Detect()
	case "internal-run":
		s.WriteEvent("run-pipeline", `{"start_stage":"wake","end_stage":"tts"}`)
		s.Stream()
	case "internal-detection":
		s.WriteEvent("run-pipeline", `{"start_stage":"asr","end_stage":"tts"}`)
		s.Stream()
	}
}

func (s *satellite) handleScript(evt *Event) {
	var script string
	if s.srv.Event != nil {
		script = s.srv.Event[evt.Type]
	}

	s.srv.Trace("event=%s data=%s payload size=%d", evt.Type, evt.Data, len(evt.Payload))

	if script == "" {
		s.handleEvent(evt)
		return
	}

	// run async because script can have sleeps
	go func() {
		e := &env{satellite: s, Type: evt.Type, Data: evt.Data}
		if res, err := expr.Eval(script, e); err != nil {
			s.srv.Trace("event=%s expr error=%s", evt.Type, err)
			s.handleEvent(evt)
		} else {
			s.srv.Trace("event=%s expr result=%v", evt.Type, res)
		}
	}()
}

func (s *satellite) Detect() bool {
	return s.setMicState(stateWaitVAD)
}

func (s *satellite) Stream() bool {
	return s.setMicState(stateActive)
}

func (s *satellite) Pause() bool {
	return s.setMicState(stateIdle)
}

func (s *satellite) Stop() bool {
	s.micStop()
	return true
}

func (s *satellite) WriteEvent(args ...string) bool {
	if len(args) == 0 {
		return false
	}
	evt := &Event{Type: args[0]}
	if len(args) > 1 {
		evt.Data = args[1]
	}
	if err := s.api.WriteEvent(evt); err != nil {
		return false
	}
	return true
}

func (s *satellite) PlayAudio() bool {
	return s.playAudio(sndCodec, bytes.NewReader(s.sndAudio))
}

func (s *satellite) PlayFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}

	codec, err := wav.ReadHeader(f)
	if err != nil {
		return false
	}

	return s.playAudio(codec, f)
}

func (e *env) Sleep(s string) bool {
	d, err := time.ParseDuration(s)
	if err != nil {
		return false
	}
	time.Sleep(d)
	return true
}
