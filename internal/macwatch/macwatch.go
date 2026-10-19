// Package macwatch keeps the camera IP addresses in the go2rtc config up to
// date.
//
// Cameras usually get their address from a DHCP server and it can change
// after the camera or the router reboot. A stream link like
// "rtsp://user:pass@192.168.1.64/Streaming/Channels/101" is pinned to the old
// address and stops working.
//
// The module remembers MAC address of every camera, finds its actual IP
// address in the OS ARP table and rewrites the config file when the address
// has changed.
package macwatch

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Interval      int               `yaml:"interval"`
			Sweep         *bool             `yaml:"sweep"`
			SweepInterval int               `yaml:"sweep_interval"`
			Verify        *bool             `yaml:"verify"`
			DryRun        bool              `yaml:"dry_run"`
			Cameras       map[string]string `yaml:"cameras"`
		} `yaml:"macwatch"`
	}

	// default config
	cfg.Mod.Interval = 60
	cfg.Mod.SweepInterval = 300

	app.LoadConfig(&cfg)

	log = app.GetLogger("macwatch")

	// API is always available, so the discover endpoint can be used for
	// creating the config section
	api.HandleFunc("api/macwatch", apiMacwatch)
	api.HandleFunc("api/macwatch/discover", apiDiscover)

	sweepEnabled = cfg.Mod.Sweep == nil || *cfg.Mod.Sweep
	verifyEnabled = cfg.Mod.Verify == nil || *cfg.Mod.Verify
	dryRun = cfg.Mod.DryRun
	sweepInterval = time.Duration(cfg.Mod.SweepInterval) * time.Second
	interval = time.Duration(cfg.Mod.Interval) * time.Second

	var list []*target

	for name, raw := range cfg.Mod.Cameras {
		mac := NormalizeMAC(raw)
		if mac == "" {
			log.Warn().Str("stream", name).Str("mac", raw).Msg("[macwatch] invalid MAC address")
			continue
		}
		list = append(list, &target{Stream: name, MAC: mac})
	}

	if len(list) == 0 {
		log.Debug().Msg("[macwatch] no cameras in the config")
		return
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Stream < list[j].Stream
	})

	targetsMu.Lock()
	targets = list
	targetsMu.Unlock()

	go worker()

	log.Info().Int("cameras", len(list)).Msg("[macwatch] init")
}

// target is a camera from the config file.
type target struct {
	Stream string
	MAC    string
}

// Camera is a current state of the camera, can be serialized to JSON.
type Camera struct {
	Stream  string `json:"stream"`
	MAC     string `json:"mac"`
	Source  string `json:"source,omitempty"`
	IP      string `json:"ip,omitempty"`
	Found   string `json:"found,omitempty"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
	Error   string `json:"error,omitempty"`
	Updated string `json:"updated,omitempty"`

	port string
}

type response struct {
	Enabled       bool      `json:"enabled"`
	ConfigPath    string    `json:"config_path,omitempty"`
	Interval      int       `json:"interval"`
	Sweep         bool      `json:"sweep"`
	SweepInterval int       `json:"sweep_interval"`
	Verify        bool      `json:"verify"`
	DryRun        bool      `json:"dry_run"`
	Updated       string    `json:"updated"`
	Cameras       []*Camera `json:"cameras"`
}

const (
	statusOK          = "ok"          // MAC matches the IP from the config
	statusUpdated     = "updated"     // IP address has been changed in the config
	statusDryRun      = "dry_run"     // IP address could be changed
	statusNotFound    = "not_found"   // MAC is absent in the ARP table
	statusUnreachable = "unreachable" // MAC is found, but the host is not alive
	statusStatic      = "static"      // source uses a hostname (or a stream name)
	statusNoStream    = "no_stream"   // stream is absent in the go2rtc
	statusError       = "error"       // internal error
)

const probeTimeout = time.Second * 2

var (
	log zerolog.Logger

	targets   []*target
	targetsMu sync.RWMutex // guards targets (filled in Init, read by the API)

	cameras []*Camera

	checkMu sync.Mutex // serialize checks
	stateMu sync.Mutex // guards cameras

	interval      time.Duration
	sweepEnabled  bool
	sweepInterval time.Duration
	verifyEnabled bool
	dryRun        bool
	lastSweep     time.Time
	lastUpdated   string
)

func worker() {
	// cameras can be offline while the go2rtc starts
	time.Sleep(time.Second * 5)
	check(false)

	if interval <= 0 {
		return
	}

	for range time.Tick(interval) {
		check(false)
	}
}

func check(force bool) {
	checkMu.Lock()
	defer checkMu.Unlock()

	neighbors, err := Neighbors()
	if err != nil {
		log.Warn().Err(err).Msg("[macwatch] can't read ARP table")
	}

	table := newTable(neighbors)

	targetsMu.RLock()
	list := targets
	targetsMu.RUnlock()

	results := make([]*Camera, 0, len(list))
	var pending []*Camera

	for _, t := range list {
		cam := &Camera{Stream: t.Stream, MAC: t.MAC}

		info, ok := sourceOf(t.Stream)

		switch {
		case !ok:
			cam.Status = statusNoStream
		case info.host == "":
			// hostname or link to another stream - nothing to update
			cam.Source = info.raw
			cam.Status = statusStatic
		default:
			cam.Source, cam.IP, cam.port = info.raw, info.host, info.port

			if table.ipToMAC[cam.IP] == cam.MAC {
				cam.Found = cam.IP
				cam.Status = statusOK
			} else {
				pending = append(pending, cam)
			}
		}

		results = append(results, cam)
	}

	if len(pending) > 0 && sweepEnabled && (force || time.Since(lastSweep) >= sweepInterval) {
		lastSweep = time.Now()

		for _, subnet := range subnets(pending) {
			log.Debug().Str("subnet", subnet.String()).Msg("[macwatch] sweep")
			Sweep(subnet)
		}

		table = waitTable(pending, table)
	}

	for _, cam := range pending {
		resolve(cam, table)
	}

	stateMu.Lock()
	for _, cam := range results {
		if cam.Updated == "" {
			if old := findCamera(cameras, cam.Stream); old != nil {
				cam.Updated = old.Updated // keep the time of the last change
			}
		}
	}
	cameras = results
	lastUpdated = time.Now().Format(time.RFC3339)
	stateMu.Unlock()
}

// waitTable waits until the ARP table will contain MAC addresses for all
// pending cameras. Some devices answer ARP requests with a big delay.
func waitTable(pending []*Camera, table *table) *table {
	for i := 0; i < 5; i++ {
		missing := false
		for _, cam := range pending {
			if len(table.macToIP[cam.MAC]) == 0 {
				missing = true
				break
			}
		}

		if !missing {
			return table
		}

		time.Sleep(time.Millisecond * 500)

		neighbors, err := Neighbors()
		if err != nil {
			continue
		}
		table = newTable(neighbors)
	}

	return table
}

func resolve(cam *Camera, table *table) {
	ips := table.macToIP[cam.MAC]
	if len(ips) == 0 {
		cam.Status = statusNotFound
		log.Debug().Str("stream", cam.Stream).Str("mac", cam.MAC).Msg("[macwatch] MAC not found")
		return
	}

	sortBySubnet(ips, cam.IP)

	var newIP string

	for _, ip := range ips {
		if ip == cam.IP {
			newIP = ip
			break
		}
		if verifyEnabled && !probePort(ip, cam.port, probeTimeout) {
			continue
		}
		newIP = ip
		break
	}

	if newIP == "" {
		cam.Status = statusUnreachable
		cam.Note = "found " + strings.Join(ips, ", ")
		return
	}

	cam.Found = newIP

	if newIP == cam.IP {
		cam.Status = statusOK
		return
	}

	// the DHCP lease has changed
	newSource, ok := replaceIPHost(cam.Source, cam.IP, newIP)
	if !ok {
		cam.Status = statusError
		cam.Error = "can't replace host in " + cam.Source
		return
	}

	if dryRun {
		cam.Status = statusDryRun
		cam.Note = newSource
		return
	}

	if err := saveConfig(cam.Stream, cam.IP, newIP); err != nil {
		cam.Status = statusError
		cam.Error = err.Error()
		return
	}

	if stream := streams.Get(cam.Stream); stream != nil {
		stream.ReplaceSource(cam.Source, newSource)
	}

	log.Info().
		Str("stream", cam.Stream).
		Str("mac", cam.MAC).
		Str("old", cam.IP).
		Str("new", newIP).
		Msg("[macwatch] update IP address")

	cam.IP = newIP
	cam.Source = newSource
	cam.Status = statusUpdated
	cam.Updated = time.Now().Format(time.RFC3339)
}

// saveConfig rewrites the IP address of the stream source in the config file.
func saveConfig(name, oldIP, newIP string) error {
	if app.ConfigPath == "" {
		return errors.New("config file disabled")
	}

	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return err
	}

	var cfg struct {
		Streams map[string]any `yaml:"streams"`
	}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	value, ok := cfg.Streams[name]
	if !ok {
		// ex. config from the command line or multiple config files
		return fmt.Errorf("stream %s not found in %s", name, app.ConfigPath)
	}

	switch value := value.(type) {
	case string:
		newValue, ok := replaceIPHost(value, oldIP, newIP)
		if !ok {
			return fmt.Errorf("can't find %s in the source", oldIP)
		}
		return app.PatchConfig([]string{"streams", name}, newValue)

	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				return errors.New("unsupported source")
			}
			if newItem, ok := replaceIPHost(s, oldIP, newIP); ok {
				s = newItem
			}
			items = append(items, s)
		}
		return app.PatchConfig([]string{"streams", name}, items)

	case map[string]any:
		s, ok := value["url"].(string)
		if !ok {
			return errors.New("unsupported source")
		}
		newSource, ok := replaceIPHost(s, oldIP, newIP)
		if !ok {
			return fmt.Errorf("can't find %s in the source", oldIP)
		}
		return app.PatchConfig([]string{"streams", name, "url"}, newSource)

	default:
		return errors.New("unsupported source")
	}
}

// replaceIPHost replaces the IP address of the host in the raw URL with
// a new one. Returns false if the URL has no host or the host is not oldIP.
func replaceIPHost(raw, oldIP, newIP string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != oldIP {
		return "", false
	}

	i := strings.Index(raw, "://")
	if i < 0 {
		return "", false
	}

	start := i + 3

	// skip user info, but not the "@" from the path
	if j := strings.LastIndexByte(raw[start:], '@'); j >= 0 {
		if k := strings.IndexAny(raw[start:], "/?#"); k < 0 || j < k {
			start += j + 1
		}
	}

	rest := raw[start:]
	end := len(rest)
	if k := strings.IndexAny(rest, "/?#"); k >= 0 {
		end = k
	}

	// host can contain the port
	if host := rest[:end]; strings.HasPrefix(host, oldIP) {
		return raw[:start] + newIP + host[len(oldIP):] + rest[end:], true
	}

	return "", false
}

type srcInfo struct {
	raw  string
	host string // IP address of the source or empty
	port string
}

func sourceOf(name string) (srcInfo, bool) {
	stream := streams.Get(name)
	if stream == nil {
		return srcInfo{}, false
	}

	sources := stream.Sources()

	for _, source := range sources {
		u, err := url.Parse(source)
		if err != nil {
			continue
		}

		host := u.Hostname()
		if net.ParseIP(host) == nil {
			continue
		}

		port := u.Port()
		if port == "" {
			port = defaultPort(u.Scheme)
		}
		if !verifiableScheme(u.Scheme) {
			port = "" // no TCP port to check
		}

		return srcInfo{raw: source, host: host, port: port}, true
	}

	if len(sources) > 0 {
		return srcInfo{raw: sources[0]}, true
	}

	return srcInfo{}, true
}

func defaultPort(scheme string) string {
	switch scheme {
	case "rtsp":
		return "554"
	case "rtsps":
		return "322"
	case "http":
		return "80"
	case "https":
		return "443"
	case "onvif":
		return "80"
	case "rtmp":
		return "1935"
	default:
		return ""
	}
}

// verifiableScheme returns false for schemes with a dynamic port (ex. HomeKit
// uses a new port after every pairing), so the port can't be used for the
// check.
func verifiableScheme(scheme string) bool {
	return scheme != "homekit"
}

// table is a fast lookup for the ARP table.
type table struct {
	ipToMAC map[string]string
	macToIP map[string][]string
}

func newTable(neighbors []Neighbor) *table {
	t := &table{
		ipToMAC: make(map[string]string, len(neighbors)),
		macToIP: make(map[string][]string, len(neighbors)),
	}

	for _, n := range neighbors {
		t.ipToMAC[n.IP] = n.MAC

		if slices.Contains(t.macToIP[n.MAC], n.IP) {
			continue // skip duplicates
		}

		t.macToIP[n.MAC] = append(t.macToIP[n.MAC], n.IP)
	}

	return t
}

// subnets returns unique local subnets for the pending cameras.
func subnets(items []*Camera) []*net.IPNet {
	var result []*net.IPNet
	var seen []string

	for _, cam := range items {
		n := subnetOf(cam.IP)
		if n == nil {
			log.Debug().Str("stream", cam.Stream).Str("ip", cam.IP).
				Msg("[macwatch] IP is not from a local subnet")
			continue
		}

		s := n.String()
		for _, x := range seen {
			if x == s {
				s = ""
				break
			}
		}

		if s != "" {
			seen = append(seen, s)
			result = append(result, n)
		}
	}

	return result
}

// sortBySubnet moves IP addresses from the same subnet as curIP to the top.
func sortBySubnet(ips []string, curIP string) {
	n := subnetOf(curIP)

	sort.SliceStable(ips, func(i, j int) bool {
		if n != nil {
			a := n.Contains(net.ParseIP(ips[i]))
			b := n.Contains(net.ParseIP(ips[j]))
			if a != b {
				return a
			}
		}
		return bytes.Compare(net.ParseIP(ips[i]).To4(), net.ParseIP(ips[j]).To4()) < 0
	})
}

func findCamera(cameras []*Camera, name string) *Camera {
	for _, cam := range cameras {
		if cam.Stream == name {
			return cam
		}
	}
	return nil
}

func snapshot() *response {
	targetsMu.RLock()
	enabled := len(targets) > 0
	targetsMu.RUnlock()

	stateMu.Lock()
	items := make([]*Camera, len(cameras))
	copy(items, cameras)
	updated := lastUpdated
	stateMu.Unlock()

	return &response{
		Enabled:       enabled,
		ConfigPath:    app.ConfigPath,
		Interval:      int(interval / time.Second),
		Sweep:         sweepEnabled,
		SweepInterval: int(sweepInterval / time.Second),
		Verify:        verifyEnabled,
		DryRun:        dryRun,
		Updated:       updated,
		Cameras:       items,
	}
}

func apiMacwatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// return the last state
	case "POST", "PUT":
		check(true) // rescan
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		return
	}

	api.ResponseJSON(w, snapshot())
}

// discover returns the config section with the MAC address for every stream,
// that has an IP address in the source URL. It allows to create the config
// without searching MAC addresses manually.
func apiDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		return
	}

	neighbors, err := Neighbors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	table := newTable(neighbors)
	cameras := discoverCameras(table)

	section, err := yaml.Encode(map[string]any{
		"macwatch": map[string]any{"cameras": cameras},
	}, 2)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	api.ResponseJSON(w, struct {
		Cameras map[string]string `json:"cameras"`
		Missing map[string]string `json:"missing,omitempty"`
		YAML    string            `json:"yaml"`
	}{
		Cameras: cameras,
		Missing: missingCameras(cameras),
		YAML:    string(section),
	})
}

// missingCameras returns streams with a known MAC address, that are not listed
// in the config yet.
func missingCameras(cameras map[string]string) map[string]string {
	targetsMu.RLock()
	defer targetsMu.RUnlock()

	missing := make(map[string]string)

streams:
	for name, mac := range cameras {
		for _, target := range targets {
			if target.Stream == name {
				continue streams
			}
		}

		missing[name] = mac
	}

	return missing
}

// discoverCameras returns MAC address for every stream that has an IP address
// in the source URL. Streams with a hostname, with a link to another stream
// and streams with an unknown MAC address will be skipped.
func discoverCameras(table *table) map[string]string {
	sources := streams.GetAllSources()

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	cameras := make(map[string]string, len(names))

	for _, name := range names {
		for _, source := range sources[name] {
			u, err := url.Parse(source)
			if err != nil {
				continue
			}

			host := u.Hostname()
			if net.ParseIP(host) == nil {
				continue
			}

			// source can be an alias for another stream
			if mac := table.ipToMAC[host]; mac != "" {
				cameras[name] = mac
			}
			break
		}
	}

	return cameras
}
