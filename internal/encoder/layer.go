package encoder

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"

	"lastedlive/internal/capture"
)

// LayerPreview streams a single source (video, monitor or window) as MJPEG,
// without the scene's crop or position. The Studio canvas places, crops and
// flips each layer itself, so editing a source doesn't restart ffmpeg; only
// adding, removing or swapping sources does.
type LayerPreview struct {
	ffmpeg string
	src    Source

	mu     sync.Mutex
	cmd    *exec.Cmd
	feeds  []*capture.Feed
	frames frameBuf
}

// layerMax caps a layer frame's longest side: sharp on the canvas, cheap enough
// to run several.
const layerMax = 1280

func NewLayerPreview(ffmpeg string, s Source) *LayerPreview {
	s.CropL, s.CropT, s.CropR, s.CropB, s.FlipH, s.FlipV = 0, 0, 0, 0, false, false
	return &LayerPreview{ffmpeg: ffmpeg, src: s}
}

func (l *LayerPreview) Frame() ([]byte, uint64) { return l.frames.get() }

func (l *LayerPreview) args() ([]string, []*capture.Feed, error) {
	s := l.src
	args := []string{"-hide_banner", "-loglevel", "error"}
	var feeds []*capture.Feed
	in := "[0:v]"
	switch s.Kind {
	case "video":
		args = append(args, "-hwaccel", "auto", "-stream_loop", "-1", "-re", "-i", s.Path)
	case "screen":
		if s.Window != "" {
			h := capture.ResolveWindow(s.Window, s.WindowApp, s.WindowTitle)
			if h == "" {
				return nil, nil, fmt.Errorf("window not found")
			}
			f, err := capture.StartFeed(h, windowFPS)
			if err != nil {
				return nil, nil, err
			}
			feeds = append(feeds, f)
			w, hh := f.Size()
			args = append(args, "-f", "rawvideo", "-pix_fmt", "bgra", "-video_size", fmt.Sprintf("%dx%d", w, hh),
				"-framerate", strconv.Itoa(windowFPS), "-thread_queue_size", "64", "-i", f.Path())
		} else {
			dm := 0
			if s.Cursor {
				dm = 1
			}
			args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("ddagrab=output_idx=%d:draw_mouse=%d:framerate=30,hwdownload,format=bgra", s.Monitor, dm))
		}
	default:
		return nil, nil, fmt.Errorf("no layer preview for %q sources", s.Kind)
	}
	filter := fmt.Sprintf("%sfps=30,scale=%d:%d:force_original_aspect_ratio=decrease:flags=bicubic,realtime[l]", in, layerMax, layerMax)
	args = append(args, "-filter_complex", filter, "-map", "[l]", "-an", "-c:v", "mjpeg", "-q:v", previewQ, "-f", "image2pipe", "pipe:1")
	return args, feeds, nil
}

func (l *LayerPreview) Start() error {
	args, feeds, err := l.args()
	if err != nil {
		return err
	}
	cmd := exec.Command(l.ffmpeg, args...)
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
	l.mu.Lock()
	l.cmd, l.feeds = cmd, feeds
	l.mu.Unlock()
	go func() {
		readJPEGs(stdout, l.frames.set)
		_ = cmd.Wait()
		stopFeeds(feeds)
	}()
	return nil
}

func (l *LayerPreview) Stop() {
	l.mu.Lock()
	cmd, feeds := l.cmd, l.feeds
	l.cmd, l.feeds = nil, nil
	l.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		killTree(cmd)
	}
	stopFeeds(feeds)
}
