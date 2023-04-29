package homekit

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/app/store"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/mdns"
	"github.com/gorilla/websocket"
	"net/http"

	"strings"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			_, err = w.Write([]byte(err.Error()))
			return
		}
		defer conn.Close()

		hkDiscoverDevices(conn)

	case "POST":
		// TODO: post params...

		id := r.URL.Query().Get("id")
		pin := r.URL.Query().Get("pin")
		name := r.URL.Query().Get("name")
		if err := hkPair(id, pin, name); err != nil {
			log.Error().Err(err).Caller().Send()
			_, err = w.Write([]byte(err.Error()))
		}

	case "DELETE":
		src := r.URL.Query().Get("src")
		if err := hkDelete(src); err != nil {
			log.Error().Err(err).Caller().Send()
			_, err = w.Write([]byte(err.Error()))
		}
	}
}

func hkDiscoverDevices(conn *websocket.Conn) {
	for {
		entries := mdns.GetAll()

		for entry := range entries {
			if !strings.HasSuffix(entry.Name, mdns.Suffix) {
				continue
			}

			name := entry.Name[:len(entry.Name)-len(mdns.Suffix)]
			device := Device{
				Name: strings.ReplaceAll(name, "\\", ""),
				Addr: fmt.Sprintf("%s:%d", entry.AddrV4, entry.Port),
			}
			for _, field := range entry.InfoFields {
				switch field[:2] {
				case "id":
					device.ID = field[3:]
				case "md":
					device.Model = field[3:]
				case "sf":
					device.Paired = field[3] == '0'
				}
			}

			err := conn.WriteJSON(device)
			if err != nil {
				log.Error().Err(err).Caller().Send()

				return
			}
		}

		time.Sleep(1 * time.Second)
	}
}

func hkPair(deviceID, pin, name string) (err error) {
	var conn *hap.Conn

	if conn, err = hap.Pair(deviceID, pin); err != nil {
		return
	}

	streams.New(name, conn.URL())

	dict := store.GetDict("streams")
	dict[name] = conn.URL()

	return store.Set("streams", dict)
}

func hkDelete(name string) (err error) {
	dict := store.GetDict("streams")
	for key, rawURL := range dict {
		if key != name {
			continue
		}

		var conn *hap.Conn

		if conn, err = hap.NewConn(rawURL.(string)); err != nil {
			return
		}

		if err = conn.Dial(); err != nil {
			return
		}

		go func() {
			if err = conn.Handle(); err != nil {
				log.Warn().Err(err).Caller().Send()
			}
		}()

		if err = conn.ListPairings(); err != nil {
			return
		}

		if err = conn.DeletePairing(conn.ClientID); err != nil {
			log.Error().Err(err).Caller().Send()
		}

		delete(dict, name)

		return store.Set("streams", dict)
	}

	return nil
}

type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	Model  string `json:"model"`
	Paired bool   `json:"paired"`
	//Type    string `json:"type"`
}
