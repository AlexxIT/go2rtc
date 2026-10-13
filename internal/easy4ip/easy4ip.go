package easy4ip

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/easy4ip"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("easy4ip")

	// the traces carry the serial and auth data, so only at trace level
	easy4ip.Logf = func(format string, args ...any) { log.Trace().Msgf(format, args...) }

	easy4ip.Warnf = func(format string, args ...any) { log.Warn().Msgf(format, args...) }

	streams.RedirectFunc("easy4ip", redirect)

	api.HandleFunc("api/easy4ip", apiDahua)
}

var log zerolog.Logger

// Shutdown tells every device its session is over. Call it before the process exits: a device
// that is not told keeps the dead session and may fall off the P2P cloud for a while.
func Shutdown() {
	mu.Lock()
	all := make([]*device, 0, len(devices))
	for _, d := range devices {
		all = append(all, d)
	}
	mu.Unlock()

	var wg sync.WaitGroup
	for _, d := range all {
		wg.Add(1)
		go func() { defer wg.Done(); _ = d.client.Close() }()
	}
	wg.Wait()
}

// redirect turns easy4ip://user:pass@SERIAL/path?query into an RTSP URL on a loopback port that tunnels
// into the device's P2P session, so the RTSP source does the rest.
//
//	easy4ip://admin:password@SERIAL/cam/realmonitor?channel=1&subtype=0
//
// Options (removed from the URL the NVR sees): rtsp_port (554), http_port (80, opens that tunnel too),
// client_id (default: stable per host and serial), relay=1 (experimental).
func redirect(rawURL string) (string, error) {
	t, err := parseURL(rawURL)
	if err != nil {
		return "", err
	}

	d := getDevice(t.serial, t.user, t.pass, t.clientID, t.relay)

	local, err := d.listen(t.rtspPort)
	if err != nil {
		return "", err
	}
	if t.httpPort > 0 {
		if _, err = d.listen(t.httpPort); err != nil {
			return "", err
		}
	}

	// A session takes seconds to minutes; the RTSP client behind us would time out before that.
	if !d.client.WaitReady(readyTimeout) {
		return "", errors.New("easy4ip: device not reachable over P2P, check the serial; after a restart wait 1-2 minutes")
	}

	return t.location(local), nil
}

type target struct {
	serial, user, pass, clientID string
	relay                        bool
	rtspPort, httpPort           int
	path, query, fragment        string
}

func parseURL(rawURL string) (*target, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Hostname() == "" || u.User == nil {
		return nil, errors.New("easy4ip: need easy4ip://user:password@SERIAL/path")
	}

	q := u.Query()
	take := func(key string) string {
		v := q.Get(key)
		q.Del(key)
		return v
	}
	t := &target{
		serial:   u.Hostname(),
		user:     u.User.Username(),
		rtspPort: atoi(take("rtsp_port"), 554),
		httpPort: atoi(take("http_port"), 0),
		clientID: take("client_id"),
		relay:    take("relay") == "1",
		path:     u.Path,
		query:    q.Encode(),
		fragment: u.Fragment,
	}
	t.pass, _ = u.User.Password()
	if t.clientID == "" {
		t.clientID = defaultClientID(t.serial)
	}
	return t, nil
}

// location is the RTSP URL of the device's RTSP server as seen through a loopback tunnel port.
func (t *target) location(port int) string {
	u := url.URL{
		Scheme:   "rtsp",
		User:     url.UserPassword(t.user, t.pass),
		Host:     "127.0.0.1:" + strconv.Itoa(port),
		Path:     t.path,
		RawQuery: t.query,
		Fragment: t.fragment,
	}
	return u.String()
}

const readyTimeout = 30 * time.Second

// short is enough to tell devices apart in logs without printing the serial.
func short(serial string) string {
	if len(serial) > 4 {
		return "..." + serial[len(serial)-4:]
	}
	return serial
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// defaultClientID is stable across restarts, so the device takes a reconnect as the same client and
// drops our stale session at once. DMSS sends "<install UUID>:37777".
func defaultClientID(serial string) string {
	host, _ := os.Hostname()
	h := sha1.Sum([]byte("go2rtc:" + host + ":" + serial))
	return fmt.Sprintf("%x-%x-%x-%x-%x:37777", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

var (
	mu      sync.Mutex
	devices = map[string]*device{}
)

type device struct {
	serial string
	sig    string // what the session was built from; a change means a new session
	client *easy4ip.Client

	mu  sync.Mutex
	lns map[int]*listener // device port -> loopback listener
}

type listener struct {
	net.Listener
	port int
}

func getDevice(serial, user, pass, clientID string, relay bool) *device {
	sig := fmt.Sprintf("%s\x00%s\x00%s\x00%t", user, pass, clientID, relay)

	mu.Lock()
	defer mu.Unlock()

	if d := devices[serial]; d != nil {
		if d.sig == sig {
			return d
		}
		d.close() // credentials changed
	}

	d := &device{serial: serial, sig: sig, lns: map[int]*listener{}}
	d.client = easy4ip.NewClient(easy4ip.Config{
		Serial: serial,
		Relay:  relay,
		Cred:   &easy4ip.Cred{User: user, Password: pass, ClientID: clientID},
		OnState: func(up bool) {
			if up {
				log.Info().Str("device", short(serial)).Msg("[easy4ip] P2P session established")
			} else {
				log.Warn().Str("device", short(serial)).Msg("[easy4ip] P2P session lost, reconnecting")
			}
		},
	})
	devices[serial] = d
	return d
}

func (d *device) close() {
	_ = d.client.Close()
	d.mu.Lock()
	for _, l := range d.lns {
		_ = l.Close()
	}
	d.lns = map[int]*listener{}
	d.mu.Unlock()
}

// listen returns the loopback port that tunnels to a device port, opening it on first use.
func (d *device) listen(devPort int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l := d.lns[devPort]; l != nil {
		return l.port, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l := &listener{Listener: ln, port: ln.Addr().(*net.TCPAddr).Port}
	d.lns[devPort] = l
	go d.accept(l, devPort)
	return l.port, nil
}

func (d *device) accept(l *listener, devPort int) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go d.proxy(conn, devPort)
	}
}

func (d *device) proxy(conn net.Conn, devPort int) {
	defer conn.Close()

	s, err := d.client.Dial(devPort)
	if err != nil {
		log.Debug().Err(err).Str("device", short(d.serial)).Int("port", devPort).Msg("[easy4ip] dial")
		return
	}
	defer s.Close()

	go func() {
		_, _ = io.Copy(s, conn)
		_ = s.Close() // client done: ends the other direction too
	}()
	_, _ = io.Copy(conn, s)
}

// apiDahua: GET /api/easy4ip -> [{"serial":"..","up":true,"ports":{"554":41234},"stats":{...}}]
func apiDahua(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Serial string         `json:"serial"`
		Up     bool           `json:"up"`
		Ports  map[string]int `json:"ports"`
		Stats  easy4ip.Stats  `json:"stats"`
	}
	items := []item{}

	mu.Lock()
	for _, d := range devices {
		it := item{Serial: d.serial, Up: d.client.Ready(), Ports: map[string]int{}, Stats: d.client.Stats()}
		d.mu.Lock()
		for dev, l := range d.lns {
			it.Ports[strconv.Itoa(dev)] = l.port
		}
		d.mu.Unlock()
		items = append(items, it)
	}
	mu.Unlock()

	api.ResponseJSON(w, items)
}
