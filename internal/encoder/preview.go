package encoder

import (
	"os/exec"
	"sync"

	"lastedlive/internal/capture"
)

// BuildPreviewCommand renders the whole scene (no audio) as MJPEG on stdout.
func BuildPreviewCommand(cfg Config) []string {
	cfg = cfg.withDefaults()
	args := []string{"-hide_banner", "-loglevel", "error"}
	inputs, filter, _, _ := buildVideo(cfg)
	args = append(args, inputs...)
	filter += ";[vout]realtime," + previewScale + "[pv]"
	args = append(args, "-filter_complex", filter, "-map", "[pv]", "-an",
		"-c:v", "mjpeg", "-q:v", previewQ, "-f", "image2pipe", "pipe:1")
	return args
}

// Preview frames are up to 1280 px on the long side at 30 FPS, which stays
// sharp on high-DPI screens.
const (
	previewScale = "fps=30,scale=1280:1280:force_original_aspect_ratio=decrease:flags=bicubic"
	previewQ     = "4"
)

type PreviewStreamer struct {
	ffmpeg string
	cfg    Config

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	frames  frameBuf
	feeds   []*capture.Feed
}

func NewPreview(ffmpeg string, cfg Config) *PreviewStreamer {
	return &PreviewStreamer{ffmpeg: ffmpeg, cfg: cfg}
}

func (p *PreviewStreamer) Start() error {
	cfg := p.cfg
	feeds := openFeeds(&cfg)
	cmd := exec.Command(p.ffmpeg, BuildPreviewCommand(cfg)...)
	configureProc(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stopFeeds(feeds)
		return err
	}
	if err := cmd.Start(); err != nil {
		stopFeeds(feeds)
		return err
	}
	assignToJob(cmd)
	p.mu.Lock()
	p.cmd = cmd
	p.running = true
	p.feeds = feeds
	p.mu.Unlock()
	go func() {
		readJPEGs(stdout, p.frames.set)
		_ = cmd.Wait()
		stopFeeds(feeds)
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()
	return nil
}

func (p *PreviewStreamer) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *PreviewStreamer) Frame() ([]byte, uint64) { return p.frames.get() }

func (p *PreviewStreamer) Stop() {
	p.mu.Lock()
	cmd := p.cmd
	feeds := p.feeds
	p.cmd, p.feeds = nil, nil
	p.running = false
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		killTree(cmd)
	}
	stopFeeds(feeds)
}
