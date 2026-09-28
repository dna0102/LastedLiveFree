package encoder

import "lastedlive/internal/capture"

// windowFPS is the window capture rate. Grabbing a GPU-rendered window takes
// 15-30 ms, so 30 FPS is what we can sustain; the output is still 60 FPS.
const windowFPS = 30

// openFeeds starts a capture for each visible window source and records its
// pipe in cfg. Windows that can't be found are skipped (drawn as placeholders).
func openFeeds(cfg *Config) []*capture.Feed {
	cfg.feeds = map[int]feedRef{}
	var feeds []*capture.Feed
	for si, s := range cfg.Visible() {
		if s.Kind != "screen" || s.Window == "" {
			continue
		}
		h := capture.ResolveWindow(s.Window, s.WindowApp, s.WindowTitle)
		if h == "" {
			continue
		}
		f, err := capture.StartFeed(h, windowFPS)
		if err != nil {
			continue
		}
		w, hh := f.Size()
		cfg.feeds[si] = feedRef{path: f.Path(), w: w, h: hh, fps: windowFPS}
		feeds = append(feeds, f)
	}
	return feeds
}

func stopFeeds(feeds []*capture.Feed) {
	for _, f := range feeds {
		f.Stop()
	}
}
