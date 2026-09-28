package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var scenesMu sync.Mutex

type Scene struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

func (m *Manager) scenesFile() string { return filepath.Join(m.Store.Dir(), "scenes.json") }

func (m *Manager) ListScenes() []Scene {
	scenesMu.Lock()
	defer scenesMu.Unlock()
	raw, err := os.ReadFile(m.scenesFile())
	if err != nil {
		return []Scene{}
	}
	var out []Scene
	if json.Unmarshal(raw, &out) != nil {
		return []Scene{}
	}
	return out
}

// SaveScene saves a scene, replacing any with the same name.
func (m *Manager) SaveScene(name string, config map[string]any) []Scene {
	scenesMu.Lock()
	defer scenesMu.Unlock()
	var list []Scene
	if raw, err := os.ReadFile(m.scenesFile()); err == nil {
		_ = json.Unmarshal(raw, &list)
	}
	out := make([]Scene, 0, len(list)+1)
	for _, s := range list {
		if s.Name != name {
			out = append(out, s)
		}
	}
	out = append(out, Scene{Name: name, Config: config})
	m.writeScenes(out)
	return out
}

func (m *Manager) DeleteScene(name string) []Scene {
	scenesMu.Lock()
	defer scenesMu.Unlock()
	var list []Scene
	if raw, err := os.ReadFile(m.scenesFile()); err == nil {
		_ = json.Unmarshal(raw, &list)
	}
	out := make([]Scene, 0, len(list))
	for _, s := range list {
		if s.Name != name {
			out = append(out, s)
		}
	}
	m.writeScenes(out)
	return out
}

func (m *Manager) writeScenes(list []Scene) {
	raw, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(m.scenesFile(), raw, 0o600)
}
