package encoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// FindFFmpeg looks for ffmpeg: the explicit path, next to the app, on PATH,
// then in common install locations.
//
// Chocolatey and Scoop install shims that start the real ffmpeg as a child
// process. Killing the shim leaves ffmpeg running, so we resolve to the real
// binary.
func FindFFmpeg(explicit string) string {
	return resolveShim(findFFmpeg(explicit))
}

// resolveShim returns the ffmpeg a Chocolatey/Scoop shim points to, or p.
func resolveShim(p string) string {
	if p == "" {
		return p
	}
	low := strings.ToLower(filepath.ToSlash(p))
	var cands []string
	switch {
	case strings.Contains(low, "/chocolatey/bin/"):
		root := filepath.Dir(filepath.Dir(p)) // ...\chocolatey
		if m, _ := filepath.Glob(filepath.Join(root, "lib", "ffmpeg*", "tools", "*", "bin", exeName("ffmpeg"))); m != nil {
			cands = append(cands, m...)
		}
		if m, _ := filepath.Glob(filepath.Join(root, "lib", "ffmpeg*", "tools", "bin", exeName("ffmpeg"))); m != nil {
			cands = append(cands, m...)
		}
	case strings.Contains(low, "/scoop/shims/"):
		root := filepath.Dir(filepath.Dir(p)) // ...\scoop
		cands = append(cands,
			filepath.Join(root, "apps", "ffmpeg", "current", "bin", exeName("ffmpeg")),
			filepath.Join(root, "apps", "ffmpeg-full", "current", "bin", exeName("ffmpeg")))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return p
}

func findFFmpeg(explicit string) string {
	cands := []string{}
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(dir, exeName("ffmpeg")),
			filepath.Join(dir, "ffmpeg", exeName("ffmpeg")),
			filepath.Join(dir, "ffmpeg", "bin", exeName("ffmpeg")),
		)
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	for _, c := range []string{
		`C:\ffmpeg\bin\ffmpeg.exe`,
		`C:\ProgramData\chocolatey\bin\ffmpeg.exe`,
		"/opt/homebrew/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/usr/bin/ffmpeg",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func ffprobePath(ffmpeg string) string {
	if ffmpeg != "" {
		p := filepath.Join(filepath.Dir(ffmpeg), exeName("ffprobe"))
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if lp, err := exec.LookPath("ffprobe"); err == nil {
		return lp
	}
	return "ffprobe"
}

type MediaInfo struct {
	Duration float64
	HasAudio bool
	Width    int
	Height   int
}

// Probe returns a file's duration, size and whether it has audio.
func Probe(ffmpeg, path string) MediaInfo {
	out, err := run(ffprobePath(ffmpeg),
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		return MediaInfo{}
	}
	var doc struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &doc) != nil {
		return MediaInfo{}
	}
	mi := MediaInfo{}
	mi.Duration, _ = strconv.ParseFloat(doc.Format.Duration, 64)
	for _, s := range doc.Streams {
		if s.CodecType == "audio" {
			mi.HasAudio = true
		}
		if s.CodecType == "video" && mi.Width == 0 {
			mi.Width, mi.Height = s.Width, s.Height
		}
	}
	return mi
}

// ListEncoders returns the H.264 encoders that work on this machine. The
// default is the first working one from prefer (the detected GPUs' encoders),
// then any other working GPU encoder, then x264. Hardware encoders are tried on
// a few test frames, since appearing in -encoders doesn't mean the driver
// supports them.
func ListEncoders(ffmpeg string, prefer []string) map[string]any {
	out, _ := run(ffmpeg, "-hide_banner", "-encoders")
	avail := []string{"libx264"}
	def := "libx264"
	order := append([]string{}, prefer...)
	for _, enc := range []string{"h264_nvenc", "h264_amf", "h264_qsv", "h264_videotoolbox"} {
		if !slices.Contains(order, enc) {
			order = append(order, enc)
		}
	}
	for _, enc := range order {
		if !bytes.Contains(out, []byte(enc)) {
			continue
		}
		if _, err := run(ffmpeg, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=black:s=256x256:r=30", "-frames:v", "3",
			"-c:v", enc, "-f", "null", "-"); err != nil {
			continue
		}
		avail = append(avail, enc)
		if def == "libx264" {
			def = enc
		}
	}
	return map[string]any{"available": avail, "default": def}
}

var dshowRe = regexp.MustCompile(`"([^"]+)"\s+\((audio|video)\)`)

// ListMics returns the DirectShow audio capture devices (Windows only).
func ListMics(ffmpeg string) []string {
	if runtime.GOOS != "windows" || ffmpeg == "" {
		return []string{}
	}
	out, _ := run(ffmpeg, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	mics := []string{}
	for _, m := range dshowRe.FindAllStringSubmatch(string(out), -1) {
		if m[2] == "audio" {
			mics = append(mics, m[1])
		}
	}
	return mics
}

// defaultFont picks a bold system font for text layers.
func defaultFont() string {
	var cands []string
	switch runtime.GOOS {
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		cands = []string{filepath.Join(win, "Fonts", "segoeuib.ttf"), filepath.Join(win, "Fonts", "arialbd.ttf"), filepath.Join(win, "Fonts", "arial.ttf")}
	case "darwin":
		cands = []string{"/System/Library/Fonts/Supplemental/Arial Bold.ttf", "/System/Library/Fonts/Helvetica.ttc"}
	default:
		cands = []string{"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", "/usr/share/fonts/TTF/DejaVuSans-Bold.ttf"}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func run(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	configureProc(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

// runStdout is like run but returns stdout only, so binary output isn't mixed
// with log lines.
func runStdout(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	configureProc(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return out.Bytes(), nil
}
