package ffsetup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func fakeBuild(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"ffmpeg-9.0-essentials_build/bin/ffmpeg.exe":  "ffmpeg",
		"ffmpeg-9.0-essentials_build/bin/ffprobe.exe": "ffprobe",
		"ffmpeg-9.0-essentials_build/bin/ffplay.exe":  "ffplay",
		"ffmpeg-9.0-essentials_build/README.txt":      "readme",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, zipBody []byte, sum string) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ffmpeg.zip":
			w.Write(zipBody)
		case "/ffmpeg.zip.sha256":
			w.Write([]byte(sum + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := DownloadURL
	DownloadURL = srv.URL + "/ffmpeg.zip"
	t.Cleanup(func() { DownloadURL = old })
}

func wait(t *testing.T, in *Installer) Status {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := in.Status(); st.State == "done" || st.State == "error" {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("install didn't finish")
	return Status{}
}

func TestInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the installer is Windows-only")
	}
	body := fakeBuild(t)
	sum := sha256.Sum256(body)
	serve(t, body, hex.EncodeToString(sum[:]))

	dir := t.TempDir()
	in := New(dir)
	var got string
	in.OnDone = func(p string) { got = p }
	if err := in.Start(); err != nil {
		t.Fatal(err)
	}
	st := wait(t, in)
	if st.State != "done" {
		t.Fatalf("state %q: %s", st.State, st.Error)
	}
	if want := filepath.Join(dir, "ffmpeg", "ffmpeg.exe"); got != want || in.Installed() != want {
		t.Fatalf("installed at %q / %q, want %q", got, in.Installed(), want)
	}
	for name, body := range map[string]string{"ffmpeg.exe": "ffmpeg", "ffprobe.exe": "ffprobe"} {
		b, err := os.ReadFile(filepath.Join(dir, "ffmpeg", name))
		if err != nil || string(b) != body {
			t.Fatalf("%s: %q, %v", name, b, err)
		}
	}
	for _, name := range []string{"ffplay.exe", "download.zip"} {
		if _, err := os.Stat(filepath.Join(dir, "ffmpeg", name)); err == nil {
			t.Fatalf("%s shouldn't be left behind", name)
		}
	}
}

func TestInstallBadChecksum(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the installer is Windows-only")
	}
	serve(t, fakeBuild(t), "0000000000000000000000000000000000000000000000000000000000000000")

	in := New(t.TempDir())
	in.Start()
	st := wait(t, in)
	if st.State != "error" || in.Installed() != "" {
		t.Fatalf("expected a checksum error, got %q (%s)", st.State, st.Error)
	}
	// A failed install can be retried.
	if err := in.Start(); err != nil || in.Status().State == "error" {
		t.Fatalf("retry didn't start: %v %q", err, in.Status().State)
	}
	wait(t, in)
}
