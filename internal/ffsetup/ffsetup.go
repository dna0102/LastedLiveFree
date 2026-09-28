// Package ffsetup downloads ffmpeg on Windows when it isn't installed. It uses
// the "essentials" build from gyan.dev (linked from ffmpeg.org), checks it
// against the published SHA-256 and keeps only ffmpeg.exe and ffprobe.exe.
package ffsetup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DownloadURL can be overridden with LASTEDLIVE_FFMPEG_URL, e.g. to use a
// mirror. A "<url>.sha256" file must sit next to it.
var DownloadURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

func init() {
	if u := os.Getenv("LASTEDLIVE_FFMPEG_URL"); u != "" {
		DownloadURL = u
	}
}

// Status is what the UI shows while installing.
type Status struct {
	State string `json:"state"` // idle | downloading | verifying | extracting | done | error
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
	Path  string `json:"path,omitempty"`
}

// Installer puts ffmpeg in <dir>/ffmpeg. OnDone is called with the path to
// ffmpeg.exe once it's ready.
type Installer struct {
	dir    string
	OnDone func(path string)

	mu sync.Mutex
	st Status
}

func New(dataDir string) *Installer {
	return &Installer{dir: filepath.Join(dataDir, "ffmpeg"), st: Status{State: "idle"}}
}

// Installed returns the path of a previously installed ffmpeg, or "".
func (in *Installer) Installed() string {
	p := filepath.Join(in.dir, "ffmpeg.exe")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func (in *Installer) Status() Status {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.st
}

// Start begins the install in the background. It does nothing if one is
// already running or finished.
func (in *Installer) Start() error {
	if runtime.GOOS != "windows" {
		return errors.New("install ffmpeg with your package manager (brew install ffmpeg, apt install ffmpeg, ...)")
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	switch in.st.State {
	case "downloading", "verifying", "extracting", "done":
		return nil
	}
	in.st = Status{State: "downloading"}
	go in.run()
	return nil
}

func (in *Installer) set(f func(*Status)) {
	in.mu.Lock()
	f(&in.st)
	in.mu.Unlock()
}

func (in *Installer) run() {
	path, err := in.install()
	if err != nil {
		in.set(func(s *Status) { s.State, s.Error = "error", err.Error() })
		return
	}
	in.set(func(s *Status) { s.State, s.Path = "done", path })
	if in.OnDone != nil {
		in.OnDone(path)
	}
}

func (in *Installer) install() (string, error) {
	if err := os.MkdirAll(in.dir, 0o755); err != nil {
		return "", err
	}
	want, err := fetchChecksum(DownloadURL + ".sha256")
	if err != nil {
		return "", fmt.Errorf("couldn't get the ffmpeg checksum: %w", err)
	}

	zipPath := filepath.Join(in.dir, "download.zip")
	defer os.Remove(zipPath)
	got, err := in.download(zipPath)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	in.set(func(s *Status) { s.State = "verifying" })
	if !strings.EqualFold(got, want) {
		return "", errors.New("the download is corrupted (checksum mismatch), try again")
	}

	in.set(func(s *Status) { s.State = "extracting" })
	if err := extract(zipPath, in.dir, "ffmpeg.exe", "ffprobe.exe"); err != nil {
		return "", fmt.Errorf("couldn't unpack ffmpeg: %w", err)
	}
	return filepath.Join(in.dir, "ffmpeg.exe"), nil
}

func fetchChecksum(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	sum := strings.Fields(string(b))
	if len(sum) == 0 || len(sum[0]) != 64 {
		return "", errors.New("unexpected checksum file")
	}
	return sum[0], nil
}

// download saves the zip to dst and returns its SHA-256, updating the progress
// as it goes.
func (in *Installer) download(dst string) (string, error) {
	resp, err := http.Get(DownloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	in.set(func(s *Status) { s.Total = resp.ContentLength })

	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, 256<<10)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			in.set(func(s *Status) { s.Done = done })
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extract copies the named files out of the zip's bin folder into dir. Each is
// written to a temp name first so a failed unpack never leaves half a binary.
func extract(zipPath, dir string, names ...string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	found := 0
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if !contains(names, base) || !strings.HasSuffix(filepath.ToSlash(filepath.Dir(f.Name)), "bin") {
			continue
		}
		if err := extractFile(f, filepath.Join(dir, base)); err != nil {
			return err
		}
		found++
	}
	if found != len(names) {
		return fmt.Errorf("the zip is missing %s", strings.Join(names, " or "))
	}
	return nil
}

func extractFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
