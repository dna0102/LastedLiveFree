// Package encoder runs ffmpeg. It composites a scene (color, image, video,
// screen and text layers), pushes it over RTMP, and from the same process
// writes an MJPEG preview of the output to stdout. On Windows every ffmpeg is
// put in a Job Object so none outlive the app.
package encoder

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lastedlive/internal/capture"
)

type Source struct {
	Kind     string `json:"kind"` // color | image | video | screen | text
	Name     string `json:"name,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	Path     string `json:"path,omitempty"`
	Color    string `json:"color,omitempty"` // fill or text color
	Text     string `json:"text,omitempty"`
	FontSize int    `json:"font_size,omitempty"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`             // 0 = canvas width
	H        int    `json:"h"`             // 0 = canvas height
	Fit      string `json:"fit,omitempty"` // stretch; cover/contain in older scenes
	UseAudio bool   `json:"use_audio"`     // video: has sound we should mix
	Monitor  int    `json:"monitor"`       // screen: ddagrab output index
	Cursor   bool   `json:"cursor"`        // screen: draw the mouse

	// screen: capture one window instead of a monitor. Handles change when the
	// app restarts, so the app and title are kept to find it again.
	Window      string `json:"window,omitempty"`
	WindowApp   string `json:"window_app,omitempty"`
	WindowTitle string `json:"window_title,omitempty"`

	// Crop is in source pixels and applied before scaling.
	CropL int  `json:"crop_l,omitempty"`
	CropT int  `json:"crop_t,omitempty"`
	CropR int  `json:"crop_r,omitempty"`
	CropB int  `json:"crop_b,omitempty"`
	FlipH bool `json:"flip_h,omitempty"`
	FlipV bool `json:"flip_v,omitempty"`

	// video: volume 0..2 (nil means 1, for scenes saved before per-source
	// volume). A muted video stays in the mix at 0 so it can be unmuted live.
	Volume *float64 `json:"volume,omitempty"`
	Muted  bool     `json:"muted,omitempty"`
}

type audioIn struct {
	input int
	src   int
}

type feedRef struct {
	path string
	w, h int
	fps  int
}

type Config struct {
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	FPS         int      `json:"fps"`
	Bitrate     int      `json:"bitrate"` // kbps
	Codec       string   `json:"codec"`
	AudioMic    string   `json:"audio_device"`
	VideoVolume *float64 `json:"video_volume,omitempty"` // 0..2 (1 = unchanged)
	MicVolume   *float64 `json:"mic_volume,omitempty"`
	Sources     []Source `json:"sources"`

	Duration  float64 `json:"-"` // -t seconds, 0 for none
	LoopVideo bool    `json:"-"` // loop video inputs
	Seek      float64 `json:"-"` // start videos this far in (reconnects)
	TmpDir    string  `json:"-"` // text layer files go here

	feeds map[int]feedRef // by visible source index; set by openFeeds
}

func (c Config) withDefaults() Config {
	if c.Width == 0 {
		c.Width = 1080
	}
	if c.Height == 0 {
		c.Height = 1920
	}
	if c.FPS == 0 {
		c.FPS = 60
	}
	if c.Bitrate == 0 {
		c.Bitrate = 6000
	}
	if c.Codec == "" {
		c.Codec = "libx264"
	}
	return c
}

// SourceGain is a video's volume (0 if muted) times the master video volume.
func SourceGain(cfg Config, s Source) float64 {
	if s.Muted {
		return 0
	}
	return vol(s.Volume) * vol(cfg.VideoVolume)
}

func vol(p *float64) float64 {
	if p == nil {
		return 1
	}
	return *p
}

func (c Config) Visible() []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if !s.Hidden {
			out = append(out, s)
		}
	}
	return out
}

// resuming reports whether this run seeks into a video after a reconnect.
//
// An input -ss decodes from the previous keyframe and throws frames away up to
// the seek point. With -re that lead-in (up to a GOP, often 5-10 s) plays at
// real speed and nothing is sent, which makes the ingest drop the connection.
// So resumed inputs are read without -re and paced by realtime/arealtime
// instead.
func resuming(cfg Config) bool { return cfg.Seek > 0 && !cfg.LoopVideo }

// buildVideo returns the input args, a filter graph ending in [vout], the
// videos whose audio should be mixed, and the number of inputs.
func buildVideo(cfg Config) (inputs []string, filter string, audio []audioIn, nInputs int) {
	fps := cfg.FPS
	W, H := cfg.Width, cfg.Height
	srcs := cfg.Visible()
	inputIndex := map[int]int{}
	ii := 0
	for si, s := range srcs {
		switch s.Kind {
		case "image":
			inputs = append(inputs, "-loop", "1", "-framerate", strconv.Itoa(fps), "-i", s.Path)
		case "video":
			pre := []string{"-hwaccel", "auto"}
			if cfg.LoopVideo {
				pre = append(pre, "-stream_loop", "-1")
			} else if cfg.Seek > 0 {
				pre = append(pre, "-ss", fmt.Sprintf("%.3f", cfg.Seek))
			}
			inputs = append(inputs, pre...)
			if !resuming(cfg) {
				inputs = append(inputs, "-re")
			}
			inputs = append(inputs, "-i", s.Path)
			if s.UseAudio {
				audio = append(audio, audioIn{input: ii, src: si})
			}
		case "screen":
			f, ok := cfg.feeds[si]
			if s.Window == "" || !ok {
				continue // monitors use ddagrab inside the graph
			}
			inputs = append(inputs, "-f", "rawvideo", "-pix_fmt", "bgra", "-video_size", fmt.Sprintf("%dx%d", f.w, f.h),
				"-framerate", strconv.Itoa(f.fps), "-thread_queue_size", "64", "-i", f.path)
		default:
			continue // color and text layers are generated in the graph
		}
		inputIndex[si] = ii
		ii++
	}

	parts := []string{fmt.Sprintf("color=c=black:s=%dx%d:r=%d[bg]", W, H, fps)}
	prev := "bg"
	for si, s := range srcs {
		cap := fmt.Sprintf("cap%d", si)
		out := fmt.Sprintf("ov%d", si)
		bx, by, bw, bh := boxOf(s, W, H)
		var src string
		switch s.Kind {
		case "text":
			parts = append(parts, textFilter(prev, out, s, cfg.TmpDir))
			prev = out
			continue
		case "color":
			col := s.Color
			if col == "" {
				col = "white"
			}
			parts = append(parts, fmt.Sprintf("color=c=%s:s=%dx%d:r=%d[%s]", col, max(bw, 2), max(bh, 2), fps, cap))
			parts = append(parts, fmt.Sprintf("[%s][%s]overlay=%d:%d:shortest=0:eof_action=repeat[%s]", prev, cap, bx, by, out))
			prev = out
			continue
		case "screen":
			if s.Window != "" {
				if idx, ok := inputIndex[si]; ok {
					src = fmt.Sprintf("[%d:v]", idx)
				} else { // window not found: keep its spot with a placeholder
					parts = append(parts, fmt.Sprintf("color=c=0x1b1b1f:s=%dx%d:r=%d[raw%d]", max(bw, 2), max(bh, 2), fps, si))
					src = fmt.Sprintf("[raw%d]", si)
				}
			} else {
				dm := 0
				if s.Cursor {
					dm = 1
				}
				parts = append(parts, fmt.Sprintf("ddagrab=output_idx=%d:draw_mouse=%d:framerate=%d,hwdownload,format=bgra[raw%d]", s.Monitor, dm, fps, si))
				src = fmt.Sprintf("[raw%d]", si)
			}
		case "image", "video":
			src = fmt.Sprintf("[%d:v]", inputIndex[si])
		default:
			continue
		}
		chain, pos, ok := placeFilter(s, W, H)
		if !ok {
			continue // entirely off the canvas
		}
		parts = append(parts, src+chain+"["+cap+"]")
		parts = append(parts, fmt.Sprintf("[%s][%s]overlay=%s:shortest=0:eof_action=repeat[%s]", prev, cap, pos, out))
		prev = out
	}
	filter = strings.Join(parts, ";") + fmt.Sprintf(";[%s]format=yuv420p[vout]", prev)
	return inputs, filter, audio, ii
}

// boxOf returns a layer's rectangle on the canvas; a zero w/h means the full
// canvas.
func boxOf(s Source, W, H int) (x, y, w, h int) {
	x, y, w, h = s.X, s.Y, s.W, s.H
	if w <= 0 {
		x, w = 0, W
	}
	if h <= 0 {
		y, h = 0, H
	}
	return
}

func fitOf(s Source) string {
	if s.Fit != "" {
		return s.Fit
	}
	if s.Kind == "image" {
		return "cover"
	}
	return "contain"
}

// srcAdjust returns the source's crop and flip filters.
func srcAdjust(s Source) []string {
	var f []string
	l, t, r, b := max(s.CropL, 0), max(s.CropT, 0), max(s.CropR, 0), max(s.CropB, 0)
	if l+t+r+b > 0 {
		f = append(f, fmt.Sprintf("crop=iw-%d:ih-%d:%d:%d", l+r, t+b, l, t))
	}
	if s.FlipH {
		f = append(f, "hflip")
	}
	if s.FlipV {
		f = append(f, "vflip")
	}
	return f
}

func even(n int) int {
	if n < 2 {
		return 2
	}
	return n &^ 1
}

// placeFilter returns the filters that size a source for the canvas, and the
// overlay position.
//
// With "stretch" the source fills its box exactly. The part of the box that
// hangs off the canvas is cropped away first, so a layer made bigger than the
// canvas (Fill) isn't scaled up only to be thrown away. "cover" and "contain"
// are for scenes saved before boxes were exact.
func placeFilter(s Source, W, H int) (chain, pos string, ok bool) {
	f := srcAdjust(s)
	bx, by, bw, bh := boxOf(s, W, H)
	if bw < 2 || bh < 2 {
		return "", "", false
	}
	switch fitOf(s) {
	case "stretch":
		vx0, vy0 := max(bx, 0), max(by, 0)
		vx1, vy1 := min(bx+bw, W), min(by+bh, H)
		if vx1-vx0 < 2 || vy1-vy0 < 2 {
			return "", "", false
		}
		if vx0 != bx || vy0 != by || vx1 != bx+bw || vy1 != by+bh {
			fw, fh := float64(vx1-vx0)/float64(bw), float64(vy1-vy0)/float64(bh)
			fx, fy := float64(vx0-bx)/float64(bw), float64(vy0-by)/float64(bh)
			f = append(f, fmt.Sprintf("crop=iw*%.6f:ih*%.6f:iw*%.6f:ih*%.6f", fw, fh, fx, fy))
		}
		f = append(f, fmt.Sprintf("scale=%d:%d", even(vx1-vx0), even(vy1-vy0)), "setsar=1")
		pos = fmt.Sprintf("%d:%d", vx0, vy0)
	case "cover":
		f = append(f, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase", bw, bh), fmt.Sprintf("crop=%d:%d", bw, bh), "setsar=1")
		pos = fmt.Sprintf("%d:%d", bx, by)
	default: // contain, centred in the box
		f = append(f, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", bw, bh), "setsar=1")
		pos = fmt.Sprintf("%d+(%d-w)/2:%d+(%d-h)/2", bx, bw, by, bh)
	}
	return strings.Join(f, ","), pos, true
}

func ffEsc(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.ReplaceAll(p, ":", `\:`)
	p = strings.ReplaceAll(p, "'", `\'`)
	return p
}

// textFilter draws a text layer. The text is passed through a file (textfile=)
// so it never needs filter escaping.
func textFilter(in, out string, s Source, tmpDir string) string {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	_ = os.MkdirAll(tmpDir, 0o755)
	sum := sha1.Sum([]byte(s.Text))
	tf := filepath.Join(tmpDir, "text_"+hex.EncodeToString(sum[:6])+".txt")
	_ = os.WriteFile(tf, []byte(s.Text), 0o644)
	size := s.FontSize
	if size <= 0 {
		size = 72
	}
	col := s.Color
	if col == "" {
		col = "white"
	}
	font := ""
	if f := defaultFont(); f != "" {
		font = "fontfile='" + ffEsc(f) + "':"
	}
	return fmt.Sprintf("[%s]drawtext=%stextfile='%s':expansion=none:fontsize=%d:fontcolor=%s:x=%d:y=%d:borderw=3:bordercolor=black@0.55[%s]",
		in, font, ffEsc(tf), size, col, s.X, s.Y, out)
}

func codecArgs(cfg Config) []string {
	br := fmt.Sprintf("%dk", cfg.Bitrate)
	gop := strconv.Itoa(cfg.FPS * 2)
	switch {
	case strings.Contains(cfg.Codec, "nvenc"):
		return []string{"-c:v", cfg.Codec, "-preset", "p4", "-tune", "ll", "-rc", "cbr",
			"-b:v", br, "-maxrate", br, "-bufsize", br, "-g", gop, "-bf", "0"}
	case strings.Contains(cfg.Codec, "amf"):
		return []string{"-c:v", cfg.Codec, "-usage", "lowlatency", "-rc", "cbr", "-b:v", br, "-maxrate", br, "-g", gop}
	case strings.Contains(cfg.Codec, "qsv"):
		return []string{"-c:v", cfg.Codec, "-preset", "veryfast", "-b:v", br, "-maxrate", br, "-g", gop}
	case strings.Contains(cfg.Codec, "videotoolbox"):
		return []string{"-c:v", cfg.Codec, "-realtime", "1", "-b:v", br, "-g", gop}
	default:
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency",
			"-b:v", br, "-maxrate", br, "-bufsize", br, "-g", gop, "-bf", "0"}
	}
}

// BuildCommand returns the ffmpeg arguments for a push. Output 0 is the RTMP
// stream; output 1 is a downscaled MJPEG copy on stdout for the preview.
func BuildCommand(rtmp string, cfg Config) []string {
	cfg = cfg.withDefaults()
	args := []string{"-hide_banner", "-loglevel", "info", "-stats_period", "0.5"}
	inputs, filter, audio, nInputs := buildVideo(cfg)
	args = append(args, inputs...)

	// Audio-only inputs go last so the video input indexes don't shift.
	var labels []string
	next := nInputs
	if cfg.AudioMic != "" {
		args = append(args, "-f", "dshow", "-rtbufsize", "128M", "-i", "audio="+cfg.AudioMic)
		filter += fmt.Sprintf(";[%d:a]volume@mic=%.2f[amic]", next, vol(cfg.MicVolume))
		labels = append(labels, "[amic]")
		next++
	}
	pace := ""
	if resuming(cfg) {
		pace = "arealtime," // resumed inputs have no -re (see resuming)
	}
	srcs := cfg.Visible()
	for k, a := range audio {
		// volume@srcN is named so SetAudio can change it while streaming.
		filter += fmt.Sprintf(";[%d:a]%svolume@src%d=%.4f[av%d]", a.input, pace, a.src, SourceGain(cfg, srcs[a.src]), k)
		labels = append(labels, fmt.Sprintf("[av%d]", k))
	}
	audioMap := ""
	switch len(labels) {
	case 0:
		args = append(args, "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo")
		audioMap = fmt.Sprintf("%d:a", next)
	case 1:
		audioMap = labels[0]
	default:
		filter += ";" + strings.Join(labels, "") + fmt.Sprintf("amix=inputs=%d:normalize=0[aout]", len(labels))
		audioMap = "[aout]"
	}

	// realtime keeps a static scene from encoding faster than real time and
	// flooding the ingest. Then split off a copy for the preview.
	filter += ";[vout]realtime,split=2[vpaced][vtap];[vtap]" + previewScale + "[vprev]"
	args = append(args, "-filter_complex", filter)

	args = append(args, "-map", "[vpaced]", "-map", audioMap)
	args = append(args, codecArgs(cfg)...)
	args = append(args, "-c:a", "aac", "-b:a", "160k", "-ar", "44100", "-r", strconv.Itoa(cfg.FPS))
	if cfg.Duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", cfg.Duration))
	}
	args = append(args, "-flvflags", "no_duration_filesize", "-f", "flv", rtmp)

	args = append(args, "-map", "[vprev]", "-an", "-c:v", "mjpeg", "-q:v", previewQ)
	if cfg.Duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", cfg.Duration))
	}
	args = append(args, "-f", "image2pipe", "pipe:1")
	return args
}

var (
	fpsRe   = regexp.MustCompile(`fps=\s*([\d.]+)`)
	speedRe = regexp.MustCompile(`speed=\s*([\d.]+)x`)
	frameRe = regexp.MustCompile(`frame=\s*(\d+)`)
	sizeRe  = regexp.MustCompile(`size=\s*(\d+)(KiB|kB|KB)`)
	dropRe  = regexp.MustCompile(`drop=\s*(\d+)`)
)

type Encoder struct {
	ffmpeg string
	rtmp   string
	cfg    Config
	logf   *os.File

	mu        sync.Mutex
	cmd       *exec.Cmd
	startedAt time.Time
	stopping  bool
	exited    atomic.Bool
	waitOnce  sync.Once

	lastFPS   float64
	lastSpeed float64
	lastFrame int
	drops     int
	upKbps    float64
	lastSize  int64
	lastSizeT time.Time
	errLine   string
	logtail   []string

	frames frameBuf
	feeds  []*capture.Feed
	stdin  io.WriteCloser // for ffmpeg's interactive commands (SetAudio)

	// OnFinish is called when ffmpeg exits by itself, not through Stop.
	OnFinish func()
}

func New(ffmpeg, rtmp string, cfg Config, logPath string) *Encoder {
	e := &Encoder{ffmpeg: ffmpeg, rtmp: rtmp, cfg: cfg.withDefaults()}
	if logPath != "" {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
		if f, err := os.Create(logPath); err == nil {
			e.logf = f
		}
	}
	return e
}

func (e *Encoder) Cfg() Config { return e.cfg }

func (e *Encoder) RTMP() string { return e.rtmp }

func (e *Encoder) StartedAt() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return e.startedAt }

func (e *Encoder) Frame() ([]byte, uint64) { return e.frames.get() }

// Start launches ffmpeg. It does nothing if Stop was already called.
func (e *Encoder) Start() error {
	e.mu.Lock()
	if e.stopping {
		e.mu.Unlock()
		return nil
	}
	if e.cmd != nil {
		e.mu.Unlock()
		return fmt.Errorf("encoder already running")
	}
	cfg := e.cfg
	e.feeds = openFeeds(&cfg)
	args := BuildCommand(e.rtmp, cfg)
	cmd := exec.Command(e.ffmpeg, args...)
	configureProc(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.mu.Unlock()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.mu.Unlock()
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		e.mu.Unlock()
		return err
	}
	e.stdin = stdin
	if err := cmd.Start(); err != nil {
		stopFeeds(e.feeds)
		e.mu.Unlock()
		return err
	}
	assignToJob(cmd)
	e.cmd = cmd
	e.startedAt = time.Now()
	if e.logf != nil {
		fmt.Fprintln(e.logf, "$ ffmpeg", strings.Join(args, " "))
		fmt.Fprintln(e.logf)
	}
	e.mu.Unlock()

	framesDone := make(chan struct{})
	go func() {
		readJPEGs(stdout, e.frames.set)
		close(framesDone)
	}()
	go e.pump(stderr, framesDone)
	return nil
}

// splitLines splits on \r as well as \n; ffmpeg rewrites its progress line
// with \r, so splitting on \n alone would only see it at exit.
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (e *Encoder) pump(stderr io.Reader, framesDone <-chan struct{}) {
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitLines)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		e.parseLine(line)
	}
	e.exited.Store(true)
	select {
	case <-framesDone:
	case <-time.After(3 * time.Second):
	}
	e.reap()
	e.mu.Lock()
	stopping := e.stopping
	cb := e.OnFinish
	feeds := e.feeds
	e.mu.Unlock()
	stopFeeds(feeds)
	if !stopping && cb != nil {
		cb()
	}
}

func (e *Encoder) parseLine(line string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	progress := strings.HasPrefix(line, "frame=")
	if e.logf != nil && !progress {
		fmt.Fprintln(e.logf, line)
	}
	if !progress {
		e.logtail = append(e.logtail, line)
		if len(e.logtail) > 60 {
			e.logtail = e.logtail[len(e.logtail)-60:]
		}
		low := strings.ToLower(line)
		for _, k := range []string{"error", "failed", "cannot", "unable", "invalid"} {
			if strings.Contains(low, k) {
				e.errLine = line
				break
			}
		}
		return
	}
	if m := fpsRe.FindStringSubmatch(line); m != nil {
		e.lastFPS, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := speedRe.FindStringSubmatch(line); m != nil {
		e.lastSpeed, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := frameRe.FindStringSubmatch(line); m != nil {
		e.lastFrame, _ = strconv.Atoi(m[1])
	}
	if m := dropRe.FindStringSubmatch(line); m != nil {
		e.drops, _ = strconv.Atoi(m[1])
	}
	// size= is for output 0 (RTMP); its change over time is the upload rate
	if m := sizeRe.FindStringSubmatch(line); m != nil {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if m[2] == "kB" {
			n *= 1000
		} else {
			n *= 1024
		}
		now := time.Now()
		if !e.lastSizeT.IsZero() {
			if dt := now.Sub(e.lastSizeT).Seconds(); dt >= 0.4 && n >= e.lastSize {
				k := float64(n-e.lastSize) * 8 / 1000 / dt
				if e.upKbps == 0 {
					e.upKbps = k
				} else {
					e.upKbps = e.upKbps*0.5 + k*0.5
				}
				e.lastSize, e.lastSizeT = n, now
			}
		} else {
			e.lastSize, e.lastSizeT = n, now
		}
	}
}

func (e *Encoder) reap() {
	e.mu.Lock()
	cmd := e.cmd
	e.mu.Unlock()
	if cmd == nil {
		return
	}
	e.waitOnce.Do(func() { _ = cmd.Wait() })
}

// Stop kills ffmpeg. A stopped encoder is never restarted by the reconnect
// logic.
func (e *Encoder) Stop() {
	e.mu.Lock()
	e.stopping = true
	cmd := e.cmd
	e.mu.Unlock()
	if cmd != nil && cmd.Process != nil && !e.exited.Load() {
		killTree(cmd)
	}
	e.reap()
	e.mu.Lock()
	feeds := e.feeds
	e.mu.Unlock()
	stopFeeds(feeds)
	e.mu.Lock()
	if e.logf != nil {
		_ = e.logf.Close()
		e.logf = nil
	}
	e.mu.Unlock()
}

func (e *Encoder) Stopping() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.stopping }

func (e *Encoder) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cmd != nil && !e.stopping && !e.exited.Load()
}

func (e *Encoder) Status() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	up := 0.0
	if !e.startedAt.IsZero() {
		up = time.Since(e.startedAt).Seconds()
	}
	tail := e.logtail
	if len(tail) > 14 {
		tail = tail[len(tail)-14:]
	}
	dropPct := 0.0
	if e.lastFrame > 0 {
		dropPct = float64(e.drops) * 100 / float64(e.lastFrame+e.drops)
	}
	return map[string]any{
		"running":     e.cmd != nil && !e.stopping && !e.exited.Load(),
		"uptime":      int(up),
		"fps":         e.lastFPS,
		"speed":       e.lastSpeed,
		"target_fps":  e.cfg.FPS,
		"frames":      e.lastFrame,
		"drops":       e.drops,
		"drop_pct":    dropPct,
		"upload_kbps": int(e.upKbps),
		"error":       e.errLine,
		"log":         append([]string(nil), tail...),
	}
}

// SetAudio changes volumes on the running push without restarting it, through
// ffmpeg's interactive "c" command on stdin. The new levels are also saved in
// the encoder's config so a reconnect keeps them.
func (e *Encoder) SetAudio(cfg Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stdin == nil || e.exited.Load() {
		return fmt.Errorf("not streaming")
	}
	old := e.cfg.Visible()
	now := cfg.Visible()
	if len(old) != len(now) {
		return fmt.Errorf("the scene changed — apply it to the LIVE first")
	}
	var b strings.Builder
	for i := range now {
		if old[i].Kind != "video" || !old[i].UseAudio {
			continue
		}
		fmt.Fprintf(&b, "cvolume@src%d -1 volume %.4f\n", i, SourceGain(cfg, now[i]))
	}
	if e.cfg.AudioMic != "" {
		fmt.Fprintf(&b, "cvolume@mic -1 volume %.4f\n", vol(cfg.MicVolume))
	}
	if b.Len() > 0 {
		if _, err := io.WriteString(e.stdin, b.String()); err != nil {
			return err
		}
	}
	// keep them for reconnects
	for i, s := range e.cfg.Sources {
		for _, n := range cfg.Sources {
			if s.Kind == "video" && n.Kind == "video" && n.Path == s.Path && n.X == s.X && n.Y == s.Y {
				e.cfg.Sources[i].Volume, e.cfg.Sources[i].Muted = n.Volume, n.Muted
			}
		}
	}
	e.cfg.VideoVolume, e.cfg.MicVolume = cfg.VideoVolume, cfg.MicVolume
	return nil
}
