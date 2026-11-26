//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func testMetadata(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "camera.json")
	value := metadata{"10.0.0.1", strings.Repeat("u", 20), strings.Repeat("e", 16), "aabbccddeeff", "test-model"}
	data, err := json.Marshal(value)
	if err != nil || os.WriteFile(path, data, 0600) != nil {
		t.Fatal("creating metadata fixture")
	}
	return path
}

func testOptions(t *testing.T) options {
	t.Helper()
	dir := testDirectory(t)
	return options{live: true, credentials: testMetadata(t, dir),
		path: "/media/mmc/record/20260101/12/34.mp4", mediaPath: filepath.Join(dir, "clip.mp4"),
		resultPath: filepath.Join(dir, "result.json"), max: 16 * 1024 * 1024}
}

func TestPrivateInputRequiresPrivateRegularFile(t *testing.T) {
	dir := testDirectory(t)
	path := testMetadata(t, dir)
	if _, ok := privateInput(path); !ok {
		t.Fatal("valid private metadata rejected")
	}
	alias := filepath.Join(dir, "alias.json")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, ok := privateInput(alias); ok {
		t.Fatal("metadata symlink followed")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := privateInput(path); ok {
		t.Fatal("readable-by-others metadata accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, ok := privateInput(path); ok {
		t.Fatal("metadata in non-private directory accepted")
	}
}

func TestPrivateOutputsRejectSymlinksExistingFilesAndPublicParents(t *testing.T) {
	for _, suffix := range []string{".mp4", ".json"} {
		t.Run(suffix, func(t *testing.T) {
			dir := testDirectory(t)
			existing := filepath.Join(dir, "existing"+suffix)
			if err := os.WriteFile(existing, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(dir, "alias"+suffix)
			if err := os.Symlink(existing, alias); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{existing, alias} {
				if f, err := privateOutput(path, suffix); err == nil || f != nil {
					t.Fatal("existing output or symlink accepted")
				}
			}
			if got, _ := os.ReadFile(existing); string(got) != "preserve" {
				t.Fatal("existing file modified")
			}
			if err := os.Chmod(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if f, err := privateOutput(filepath.Join(dir, "new"+suffix), suffix); err == nil || f != nil {
				t.Fatal("output in non-private directory accepted")
			}
		})
	}
}

type testCamera struct {
	downloadErr, closeErr error
	closed                bool
}

func (c *testCamera) DownloadSDFile(_ string, output io.Writer, _ uint32) (uint32, error) {
	n, err := output.Write([]byte("test"))
	if c.downloadErr != nil {
		err = c.downloadErr
	}
	return uint32(n), err
}

func (c *testCamera) Close() error { c.closed = true; return c.closeErr }

func TestExecutePreflightsAllPathsBeforeDial(t *testing.T) {
	for _, name := range []string{"result collision", "media collision", "result symlink", "public result parent", "public credential parent", "same result and credentials", "same media and result"} {
		t.Run(name, func(t *testing.T) {
			o := testOptions(t)
			switch name {
			case "result collision":
				if err := os.WriteFile(o.resultPath, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "media collision":
				if err := os.WriteFile(o.mediaPath, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "result symlink":
				if err := os.Symlink(o.credentials, o.resultPath); err != nil {
					t.Fatal(err)
				}
			case "public result parent":
				if err := os.Chmod(filepath.Dir(o.resultPath), 0755); err != nil {
					t.Fatal(err)
				}
			case "public credential parent":
				other := testDirectory(t)
				o.credentials = testMetadata(t, other)
				if err := os.Chmod(other, 0755); err != nil {
					t.Fatal(err)
				}
			case "same result and credentials":
				o.resultPath = o.credentials
			case "same media and result":
				o.mediaPath = o.resultPath
			}
			called := false
			code := execute(o, func(string) (camera, error) { called = true; return &testCamera{}, nil })
			if code == 0 || called {
				t.Fatal("invalid local paths reached camera authentication")
			}
			if _, ok := privateInput(o.credentials); !ok && name != "public result parent" && name != "public credential parent" {
				t.Fatal("pairing input was modified by output preflight")
			}
			if name == "media collision" {
				if data, _ := os.ReadFile(o.mediaPath); string(data) != "preserve" {
					t.Fatal("unowned media collision was removed or changed")
				}
			}
		})
	}
}

func TestExecuteSuccessAndFailureCleanup(t *testing.T) {
	for _, name := range []string{"success", "transfer failure", "session close failure"} {
		t.Run(name, func(t *testing.T) {
			o := testOptions(t)
			client := &testCamera{}
			if name == "transfer failure" {
				client.downloadErr = errors.New("fixed fixture failure")
			}
			if name == "session close failure" {
				client.closeErr = errors.New("fixture close failure")
			}
			code := execute(o, func(string) (camera, error) {
				for _, path := range []string{o.mediaPath, o.resultPath} {
					info, err := os.Stat(path)
					if err != nil || info.Mode().Perm() != 0600 {
						t.Fatal("outputs not privately reserved before Dial")
					}
				}
				return client, nil
			})
			if !client.closed {
				t.Fatal("camera session left open")
			}
			data, err := os.ReadFile(o.resultPath)
			var r result
			if err != nil || json.Unmarshal(data, &r) != nil || r.Operation != "download_sd_file" {
				t.Fatal("missing integration result contract")
			}
			if name == "success" {
				media, err := os.ReadFile(o.mediaPath)
				if code != 0 || r.Phase != "download_complete" || r.Bytes != 4 || err != nil || string(media) != "test" {
					t.Fatalf("success result invalid: code=%d result=%+v", code, r)
				}
			} else {
				if code == 0 || r.Error == "" || r.Phase == "download_complete" {
					t.Fatal("failure was reported as success")
				}
				if _, err := os.Stat(o.mediaPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed transfer retained media")
				}
			}
		})
	}
}

type faultWriter struct {
	bytes.Buffer
	syncErr, closeErr, writeErr error
	shortWrite                  bool
	synced, closed              bool
}

func (w *faultWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.shortWrite {
		return w.Buffer.Write(data[:len(data)-1])
	}
	return w.Buffer.Write(data)
}
func (w *faultWriter) Sync() error  { w.synced = true; return w.syncErr }
func (w *faultWriter) Close() error { w.closed = true; return w.closeErr }

func TestFinalizeFilesChecksEveryDurabilityFailure(t *testing.T) {
	failure := errors.New("fixture IO failure")
	for _, name := range []string{"media sync", "media close", "report encode", "report short write", "report sync", "report close"} {
		t.Run(name, func(t *testing.T) {
			media, report := &faultWriter{}, &faultWriter{}
			switch name {
			case "media sync":
				media.syncErr = failure
			case "media close":
				media.closeErr = failure
			case "report encode":
				report.writeErr = failure
			case "report short write":
				report.shortWrite = true
			case "report sync":
				report.syncErr = failure
			case "report close":
				report.closeErr = failure
			}
			r := result{Operation: "download_sd_file", Phase: "download_sd_file"}
			success, reportErr := finalizeFiles(media, report, &r, true)
			if success || !media.synced || !media.closed || !report.closed {
				t.Fatal("finalization failure ignored or a writer left open")
			}
			if strings.HasPrefix(name, "report") && reportErr == nil {
				t.Fatal("report failure not distinguished for cleanup")
			}
			if strings.HasPrefix(name, "media") && (r.Error == "" || r.Phase == "download_complete") {
				t.Fatal("media finalization failure reported completion")
			}
		})
	}
}

type syncFailingFile struct{ durableWriter }

func (f syncFailingFile) Sync() error { return errors.New("fixture sync failure") }

func TestResultFinalizationFailureRemovesOwnedMedia(t *testing.T) {
	dir := testDirectory(t)
	mediaPath, reportPath := filepath.Join(dir, "clip.mp4"), filepath.Join(dir, "result.json")
	media, err := privateOutput(mediaPath, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	report, err := privateOutput(reportPath, ".json")
	if err != nil {
		t.Fatal(err)
	}
	report.file = syncFailingFile{report.file}
	r := result{Operation: "download_sd_file", Phase: "download_sd_file"}
	if code := finishOutputs(media, report, &r, 0); code == 0 {
		t.Fatal("result sync failure retained success exit status")
	}
	for _, path := range []string{mediaPath, reportPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("result finalization failure retained an owned output")
		}
	}
}

func TestCleanupUsesPinnedDirectoryAfterPathReplacement(t *testing.T) {
	dir := testDirectory(t)
	original, moved := filepath.Join(dir, "original"), filepath.Join(dir, "moved")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	media, err := privateOutput(filepath.Join(original, "clip.mp4"), ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	report, err := privateOutput(filepath.Join(dir, "result.json"), ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(original, "clip.mp4")
	if err := os.WriteFile(replacement, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	r := result{Phase: "download_sd_file", Error: "fixed failure"}
	if finishOutputs(media, report, &r, 1) != 1 {
		t.Fatal("failed download became successful")
	}
	if _, err := os.Stat(filepath.Join(moved, "clip.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original owned media was not removed")
	}
	if data, _ := os.ReadFile(replacement); string(data) != "preserve" {
		t.Fatal("cleanup followed the replaced directory pathname")
	}
}
