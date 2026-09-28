package manager

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"lastedlive/internal/encoder"
	"lastedlive/internal/hardware"
)

var (
	devMu    sync.Mutex
	devCache map[string]any
)

// FFmpeg returns the ffmpeg path, or "" if there isn't one yet.
func (m *Manager) FFmpeg() string {
	m.ffMu.RLock()
	defer m.ffMu.RUnlock()
	return m.ffmpeg
}

// SetFFmpeg switches to a newly installed ffmpeg.
func (m *Manager) SetFFmpeg(path string) {
	m.ffMu.Lock()
	m.ffmpeg = path
	m.ffMu.Unlock()
	devMu.Lock()
	devCache = nil
	devMu.Unlock()
}

// EncoderDevices returns the ffmpeg path, the machine's CPU and GPUs, working
// encoders and microphones. Checking the hardware encoders means test encodes,
// so the result is kept until ffmpeg changes.
func (m *Manager) EncoderDevices() map[string]any {
	hw := hardware.Detect()
	ff := m.FFmpeg()
	if ff == "" {
		return map[string]any{"ffmpeg": nil, "error": "ffmpeg not found", "hardware": hw,
			"encoders": map[string]any{"available": []string{"libx264"}, "default": "libx264"}, "mics": []string{}}
	}
	devMu.Lock()
	defer devMu.Unlock()
	if devCache == nil {
		devCache = map[string]any{
			"ffmpeg":   ff,
			"hardware": hw,
			"encoders": encoder.ListEncoders(ff, hw.Encoders()),
			"mics":     encoder.ListMics(ff),
		}
	}
	return devCache
}

func (m *Manager) tmpDir() string { return filepath.Join(m.Store.Dir(), "tmp") }

// StartEncoder starts streaming the scene to the account's room. Videos loop
// until the LIVE is ended.
func (m *Manager) StartEncoder(accountID string, cfg encoder.Config) (map[string]any, error) {
	ff := m.FFmpeg()
	if ff == "" {
		return nil, fmt.Errorf("ffmpeg isn't installed yet — install it from Customise → System")
	}
	s := m.Session(accountID)
	if s == nil {
		return nil, fmt.Errorf("not live — go LIVE first")
	}
	if len(cfg.Visible()) == 0 {
		return nil, fmt.Errorf("add at least one visible source to your scene")
	}
	cfg.TmpDir = m.tmpDir()

	for i := range cfg.Sources {
		src := &cfg.Sources[i]
		if src.Hidden || src.Kind != "video" || src.Path == "" {
			continue
		}
		cfg.LoopVideo = true
		if src.UseAudio && !encoder.Probe(ff, src.Path).HasAudio {
			src.UseAudio = false
		}
	}

	m.mu.Lock()
	if e, ok := m.encoders[accountID]; ok && e.Running() {
		m.mu.Unlock()
		return nil, fmt.Errorf("already streaming on this account")
	}
	m.encRetries[accountID] = 0
	enc := encoder.New(ff, s.RTMP, cfg, m.logPathFor(accountID))
	enc.OnFinish = func() { m.onEncoderExit(accountID) }
	m.encoders[accountID] = enc
	prev := m.previews[accountID]
	delete(m.previews, accountID)
	m.mu.Unlock()

	if prev != nil { // the push has its own preview output
		prev.Stop()
	}
	m.StopLayers(accountID)
	if err := enc.Start(); err != nil {
		return nil, err
	}
	time.Sleep(700 * time.Millisecond) // catch bad inputs before reporting success
	st := enc.Status()
	if !enc.Running() {
		if msg, _ := st["error"].(string); msg != "" {
			return st, fmt.Errorf("encoder stopped: %s", msg)
		}
		return st, fmt.Errorf("encoder stopped immediately — check your sources")
	}
	return st, nil
}

// onEncoderExit runs when ffmpeg exits by itself, which means the stream
// dropped: reconnect. Encoders stopped on purpose are left alone.
func (m *Manager) onEncoderExit(accountID string) {
	m.mu.RLock()
	enc := m.encoders[accountID]
	m.mu.RUnlock()
	if enc == nil || enc.Stopping() {
		return
	}
	cfg := enc.Cfg()
	uptime := 0.0
	if t := enc.StartedAt(); !t.IsZero() {
		uptime = time.Since(t).Seconds()
	}

	// The retry count resets if it had been running a while.
	m.mu.Lock()
	if uptime > 30 {
		m.encRetries[accountID] = 0
	}
	retries := m.encRetries[accountID]
	if retries >= 8 {
		m.mu.Unlock()
		return
	}
	m.encRetries[accountID] = retries + 1
	rtmp := enc.RTMP()
	if s := m.sessions[accountID]; s != nil {
		rtmp = s.RTMP
	}
	m.mu.Unlock()

	time.Sleep(3 * time.Second)

	newEnc := encoder.New(m.FFmpeg(), rtmp, cfg, m.logPathFor(accountID))
	newEnc.OnFinish = func() { m.onEncoderExit(accountID) }

	m.mu.Lock()
	if m.encoders[accountID] != enc || enc.Stopping() {
		m.mu.Unlock()
		return // replaced or stopped while we waited
	}
	m.encoders[accountID] = newEnc
	m.mu.Unlock()
	_ = newEnc.Start()
}

func (m *Manager) StopEncoder(accountID string) error {
	m.mu.Lock()
	enc := m.encoders[accountID]
	m.mu.Unlock()
	if enc == nil {
		return fmt.Errorf("not streaming")
	}
	enc.Stop()
	return nil
}

func (m *Manager) EncoderStatus(accountID string) map[string]any {
	m.mu.RLock()
	enc := m.encoders[accountID]
	m.mu.RUnlock()
	if enc == nil {
		return nil
	}
	return enc.Status()
}

// StartPreview (re)starts the whole-scene preview. It does nothing while the
// account is streaming, since the push has its own preview.
func (m *Manager) StartPreview(accountID string, cfg encoder.Config) error {
	if m.FFmpeg() == "" {
		return fmt.Errorf("ffmpeg not found")
	}
	m.mu.RLock()
	enc := m.encoders[accountID]
	m.mu.RUnlock()
	if enc != nil && enc.Running() {
		return nil
	}
	if len(cfg.Visible()) == 0 {
		m.StopPreview(accountID)
		return nil
	}
	cfg.TmpDir = m.tmpDir()
	ps := encoder.NewPreview(m.FFmpeg(), cfg)
	m.mu.Lock()
	old := m.previews[accountID]
	m.previews[accountID] = ps
	m.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	return ps.Start()
}

// PreviewFrame returns the latest frame of the stream if the account is
// streaming, else of the scene preview.
func (m *Manager) PreviewFrame(accountID string) ([]byte, uint64) {
	m.mu.RLock()
	enc := m.encoders[accountID]
	ps := m.previews[accountID]
	m.mu.RUnlock()
	if enc != nil && enc.Running() {
		return enc.Frame()
	}
	if ps != nil {
		return ps.Frame()
	}
	return nil, 0
}

func (m *Manager) StopPreview(accountID string) {
	m.mu.Lock()
	ps := m.previews[accountID]
	delete(m.previews, accountID)
	m.mu.Unlock()
	if ps != nil {
		ps.Stop()
	}
}

func (m *Manager) Shutdown() {
	stopAllLayers()
	m.mu.Lock()
	var previews []*encoder.PreviewStreamer
	for _, p := range m.previews {
		previews = append(previews, p)
	}
	var encoders []*encoder.Encoder
	for _, e := range m.encoders {
		encoders = append(encoders, e)
	}
	m.mu.Unlock()
	for _, p := range previews {
		p.Stop()
	}
	for _, e := range encoders {
		e.Stop()
	}
}

func (m *Manager) SetLiveAudio(accountID string, cfg encoder.Config) error {
	m.mu.RLock()
	enc := m.encoders[accountID]
	m.mu.RUnlock()
	if enc == nil || !enc.Running() {
		return fmt.Errorf("not streaming")
	}
	return enc.SetAudio(cfg)
}
