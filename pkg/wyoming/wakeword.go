package wyoming

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type WakeWord struct {
	*API
	names []string
	send  int

	Detection string
}

func DialWakeWord(rawURL string) (*WakeWord, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	api, err := DialAPI(u.Host)
	if err != nil {
		return nil, err
	}

	names := u.Query()["name"]
	if len(names) == 0 {
		names = []string{"ok_nabu_v0.1"}
	}

	wake := &WakeWord{API: api, names: names}
	if err = wake.Start(); err != nil {
		_ = wake.Close()
		return nil, err
	}

	go wake.handle()
	return wake, nil
}

func (w *WakeWord) handle() {
	defer w.Close()

	for {
		evt, err := w.ReadEvent()
		if err != nil {
			return
		}

		if evt.Type == "detection" {
			var data struct {
				Name string `json:"name"`
			}
			if err = json.Unmarshal([]byte(evt.Data), &data); err != nil {
				return
			}
			w.Detection = data.Name
		}
	}
}

//func (w *WakeWord) Describe() error {
//	if err := w.WriteEvent(&Event{Type: "describe"}); err != nil {
//		return err
//	}
//
//	evt, err := w.ReadEvent()
//	if err != nil {
//		return err
//	}
//
//	var info struct {
//		Wake []struct {
//			Models []struct {
//				Name string `json:"name"`
//			} `json:"models"`
//		} `json:"wake"`
//	}
//	if err = json.Unmarshal(evt.Data, &info); err != nil {
//		return err
//	}
//
//	return nil
//}

func (w *WakeWord) Start() error {
	msg := struct {
		Names []string `json:"names"`
	}{
		Names: w.names,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	evt := &Event{Type: "detect", Data: string(data)}
	if err := w.WriteEvent(evt); err != nil {
		return err
	}

	evt = &Event{Type: "audio-start", Data: audioData(0)}
	return w.WriteEvent(evt)
}

func (w *WakeWord) Close() error {
	return w.conn.Close()
}

func (w *WakeWord) WriteChunk(payload []byte) error {
	evt := &Event{Type: "audio-chunk", Data: audioData(w.send), Payload: payload}
	w.send += len(payload)
	return w.WriteEvent(evt)
}

func audioData(send int) string {
	// timestamp in ms = send / 2 * 1000 / 16000 = send / 32
	return fmt.Sprintf(`{"rate":16000,"width":2,"channels":1,"timestamp":%d}`, send/32)
}
