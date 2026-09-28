package manager

import (
	"fmt"
	"sync"

	"lastedlive/internal/encoder"
)

// Layer previews for the Studio canvas, per account, keyed by a source id the
// UI derives from what the source shows (file, monitor or window). Moving or
// cropping a source doesn't change its key, so it doesn't restart anything.
var (
	layerMu sync.Mutex
	layers  = map[string]map[string]*encoder.LayerPreview{}
)

// SyncLayers starts previews for new keys in want and stops ones no longer in
// it. It returns start errors by key (a closed window, for example).
func (m *Manager) SyncLayers(accountID string, want map[string]encoder.Source) map[string]string {
	if m.FFmpeg() == "" {
		return map[string]string{"*": "ffmpeg not found"}
	}
	layerMu.Lock()
	cur := layers[accountID]
	if cur == nil {
		cur = map[string]*encoder.LayerPreview{}
		layers[accountID] = cur
	}
	var stop []*encoder.LayerPreview
	for k, l := range cur {
		if _, ok := want[k]; !ok {
			stop = append(stop, l)
			delete(cur, k)
		}
	}
	var start []string
	for k := range want {
		if _, ok := cur[k]; !ok {
			start = append(start, k)
		}
	}
	layerMu.Unlock()

	for _, l := range stop {
		l.Stop()
	}
	errs := map[string]string{}
	for _, k := range start {
		l := encoder.NewLayerPreview(m.FFmpeg(), want[k])
		if err := l.Start(); err != nil {
			errs[k] = err.Error()
			continue
		}
		layerMu.Lock()
		if old := layers[accountID][k]; old != nil { // another sync started it first
			layerMu.Unlock()
			l.Stop()
			continue
		}
		layers[accountID][k] = l
		layerMu.Unlock()
	}
	return errs
}

func (m *Manager) StopLayers(accountID string) { m.SyncLayers(accountID, nil) }

func (m *Manager) LayerFrame(accountID, key string) ([]byte, uint64, error) {
	layerMu.Lock()
	l := layers[accountID][key]
	layerMu.Unlock()
	if l == nil {
		return nil, 0, fmt.Errorf("no such layer")
	}
	jpg, seq := l.Frame()
	return jpg, seq, nil
}

func stopAllLayers() {
	layerMu.Lock()
	var all []*encoder.LayerPreview
	for _, ls := range layers {
		for _, l := range ls {
			all = append(all, l)
		}
	}
	layers = map[string]map[string]*encoder.LayerPreview{}
	layerMu.Unlock()
	for _, l := range all {
		l.Stop()
	}
}
