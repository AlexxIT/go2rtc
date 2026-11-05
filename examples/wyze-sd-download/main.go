//go:build linux

// wyze-sd-download demonstrates one local recording download using cached
// pairing metadata. It performs no cloud discovery and runs no HTTP service.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk/dtls"
	"github.com/AlexxIT/go2rtc/pkg/wyze"
)

type metadata struct {
	IP    string `json:"ip"`
	UID   string `json:"uid"`
	ENR   string `json:"enr"`
	MAC   string `json:"mac"`
	Model string `json:"model"`
}

type result struct {
	Operation     string  `json:"operation"`
	Phase         string  `json:"phase"`
	RecordingPath string  `json:"recording_path"`
	Bytes         uint32  `json:"downloaded_bytes"`
	Elapsed       float64 `json:"elapsed_seconds"`
	Error         string  `json:"error,omitempty"`
}

// Keep directory descriptors open for the complete operation. Path replacement
// outside these private directories cannot redirect later opens or cleanup.
func privateParent(path string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", os.ErrInvalid
	}
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return nil, "", os.ErrInvalid
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "", os.ErrPermission
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || !ownedPrivate(info) {
		_ = root.Close()
		return nil, "", os.ErrPermission
	}
	return root, name, nil
}

func ownedPrivate(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.Mode().Perm()&0077 == 0
}

func privateInput(path string) (metadata, bool) {
	var value metadata
	root, name, err := privateParent(path)
	if err != nil {
		return value, false
	}
	defer root.Close()
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return value, false
	}
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() || !ownedPrivate(info) || info.Size() > 16384 {
		_ = f.Close()
		return value, false
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 16385))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(data) > 16384 {
		return value, false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil {
		return value, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return value, false
	}
	ip, err := netip.ParseAddr(value.IP)
	return value, err == nil && ip.Is4() && ip.IsPrivate() && len(value.UID) == 20 && len(value.ENR) >= 16 && value.MAC != "" && value.Model != ""
}

type privateFile struct {
	root *os.Root
	name string
	file durableWriter
}

func privateOutput(path, suffix string) (*privateFile, error) {
	if !strings.HasSuffix(path, suffix) {
		return nil, os.ErrInvalid
	}
	root, name, err := privateParent(path)
	if err != nil {
		return nil, err
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		_ = root.Close()
		return nil, os.ErrPermission
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !ownedPrivate(info) {
		_ = f.Close()
		_ = root.Remove(name)
		_ = root.Close()
		return nil, os.ErrPermission
	}
	return &privateFile{root, name, f}, nil
}

type durableWriter interface {
	io.Writer
	Sync() error
	Close() error
}

type checkedWriter struct{ io.Writer }

func (w checkedWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

// Finalize media before reporting success. Return a report error separately so
// callers discard a partially written result as well as any owned media file.
func finalizeFiles(media, report durableWriter, r *result, success bool) (bool, error) {
	if media != nil {
		if success && media.Sync() != nil {
			r.Error, success = "local media sync failed", false
		}
		if err := media.Close(); err != nil {
			if success {
				r.Error = "local media close failed"
			}
			success = false
		}
		if success {
			r.Phase = "download_complete"
		}
	}
	var reportErr error
	if json.NewEncoder(checkedWriter{report}).Encode(r) != nil {
		reportErr = errors.New("local result encode failed")
	} else if report.Sync() != nil {
		reportErr = errors.New("local result sync failed")
	}
	if err := report.Close(); err != nil && reportErr == nil {
		reportErr = errors.New("local result close failed")
	}
	return success && reportErr == nil, reportErr
}

func finishOutputs(media, report *privateFile, r *result, code int) int {
	var output durableWriter
	if media != nil {
		output = media.file
	}
	success, reportErr := finalizeFiles(output, report.file, r, code == 0)
	if !success {
		code = 1
		if media != nil {
			_ = media.root.Remove(media.name)
		}
	}
	if reportErr != nil {
		_ = report.root.Remove(report.name)
	}
	if media != nil {
		_ = media.root.Close()
	}
	_ = report.root.Close()
	return code
}

type options struct {
	live                                     bool
	credentials, path, mediaPath, resultPath string
	max                                      uint
}

type camera interface {
	DownloadSDFile(string, io.Writer, uint32) (uint32, error)
	Close() error
}

func execute(o options, dial func(string) (camera, error)) (code int) {
	r := result{Operation: "download_sd_file", Phase: "prepare", RecordingPath: o.path}
	// Reserve the result before any network operation. O_EXCL and the pinned
	// directory descriptor protect existing files, including pairing metadata.
	if o.resultPath == o.mediaPath || o.resultPath == o.credentials || (o.mediaPath != "" && o.mediaPath == o.credentials) {
		return 1
	}
	report, err := privateOutput(o.resultPath, ".json")
	if err != nil {
		return 1
	}
	var media *privateFile
	defer func() {
		code = finishOutputs(media, report, &r, code)
	}()
	if o.max == 0 || o.max > 64*1024*1024 || dtls.ValidateRDTFileRequest(o.path, uint32(o.max)) != nil {
		r.Error = "invalid recording request or size limit"
		return 1
	}
	if !o.live {
		r.Phase = "offline_download_prepared"
		return 0
	}
	v, ok := privateInput(o.credentials)
	if !ok {
		r.Error = "private pairing metadata is missing or invalid"
		return 1
	}
	media, err = privateOutput(o.mediaPath, ".mp4")
	if err != nil {
		r.Error = "private media output is unavailable"
		return 1
	}
	q := url.Values{"uid": {v.UID}, "enr": {v.ENR}, "mac": {v.MAC}, "model": {v.Model}, "dtls": {"true"}}
	endpoint := (&url.URL{Scheme: "wyze", Host: v.IP, RawQuery: q.Encode()}).String()
	r.Phase = "authenticate"
	client, err := dial(endpoint)
	if err != nil {
		r.Error = "LAN authentication failed"
		return 1
	}
	defer func() {
		if client.Close() != nil && code == 0 {
			r.Error, code = "LAN session close failed", 1
		}
	}()
	r.Phase = "download_sd_file"
	start := time.Now()
	r.Bytes, err = client.DownloadSDFile(o.path, media.file, uint32(o.max))
	r.Elapsed = time.Since(start).Seconds()
	if err != nil {
		// The Wyze download API returns fixed local diagnostics only.
		r.Error = err.Error()
		return 1
	}
	return 0
}

func run() int {
	o := options{}
	flag.BoolVar(&o.live, "live", false, "Permit one authenticated LAN download")
	flag.StringVar(&o.credentials, "credentials", "", "Private Linux JSON file: ip, uid, enr, mac, model")
	flag.StringVar(&o.path, "download", "", "Camera-local /media/mmc/record/YYYYMMDD/HH/MM.mp4 path")
	flag.StringVar(&o.mediaPath, "media-output", "", "New .mp4 in a private Linux directory")
	flag.StringVar(&o.resultPath, "output", "", "New JSON result in a private Linux directory")
	flag.UintVar(&o.max, "max-bytes", 16*1024*1024, "Maximum file bytes, at most 64 MiB")
	flag.Parse()
	return execute(o, func(endpoint string) (camera, error) { return wyze.Dial(endpoint) })
}

func main() { log.SetOutput(io.Discard); os.Exit(run()) }
