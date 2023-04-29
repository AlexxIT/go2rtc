package homekit

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/AlexxIT/go2rtc/cmd/app/store"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/mdns"
	"github.com/gorilla/websocket"

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

var activeConnections int
var activeConnectionsMutex sync.Mutex

func apiHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			_, err = w.Write([]byte(err.Error()))
			return
		}
		activeConnectionsMutex.Lock()
		activeConnections++
		activeConnectionsMutex.Unlock()

		done := make(chan struct{})
		go hkDiscoverDevices(conn, done)

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				log.Debug().Err(err).Caller().Send()
				_, err = w.Write([]byte(err.Error()))
				break
			}
		}

		close(done)
		activeConnectionsMutex.Lock()
		activeConnections--
		activeConnectionsMutex.Unlock()
		conn.Close()

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

func hkDiscoverDevices(conn *websocket.Conn, done chan struct{}) {
	queryCounter := 0
	for {
		select {
		case <-done:
			return
		default:
			log.Trace().Int("active connections: ", activeConnections).Msg("[homekit] ")
			activeConnectionsMutex.Lock()
			if activeConnections <= 0 {
				activeConnectionsMutex.Unlock()
				return
			}
			activeConnectionsMutex.Unlock()

			queryCounter++
			timeout := time.Second
			if queryCounter%10 == 0 {
				timeout = 5 * time.Second
			}

			entries := mdns.GetAll(timeout)

			for name, src := range store.GetDict("streams") {
				if src := src.(string); strings.HasPrefix(src, "homekit") {
					u, err := url.Parse(src)
					if err != nil {
						continue
					}
					device := Device{
						Name:   name,
						Addr:   u.Host,
						Paired: true,
					}
					err = conn.WriteJSON(device)
					if err != nil {
						log.Error().Err(err).Caller().Send()

						return
					}
				}
			}

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
					log.Debug().Err(err).Caller().Send()

					return
				}
			}

			time.Sleep(timeout)
		}
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
