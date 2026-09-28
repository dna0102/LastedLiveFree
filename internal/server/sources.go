package server

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"lastedlive/internal/capture"
	"lastedlive/internal/encoder"
)

// FilePicker shows the native file dialog. The desktop app sets it at startup;
// kind is "image" or "video".
var FilePicker func(kind string) (string, error)

func (s *Server) routesSources(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/capture/sources", func(w http.ResponseWriter, r *http.Request) {
		wins := capture.ListWindows()
		if wins == nil {
			wins = []capture.Window{}
		}
		writeJSON(w, 200, map[string]any{"monitors": encoder.ListMonitors(s.m.FFmpeg()), "windows": wins})
	})
	mux.HandleFunc("GET /api/capture/thumb", s.handleCaptureThumb)
	mux.HandleFunc("GET /api/media/info", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			writeErr(w, 404, "file not found")
			return
		}
		mi := encoder.Probe(s.m.FFmpeg(), p)
		if mi.Width == 0 {
			writeErr(w, 400, "couldn't read this file — is it an image or video?")
			return
		}
		writeJSON(w, 200, map[string]any{"width": mi.Width, "height": mi.Height, "duration": mi.Duration, "has_audio": mi.HasAudio})
	})
	mux.HandleFunc("POST /api/pick-file", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Kind string `json:"kind"`
		}
		_ = readJSON(r, &b)
		if FilePicker == nil {
			writeErr(w, 501, "file dialog unavailable")
			return
		}
		p, err := FilePicker(b.Kind)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"path": p})
	})
}

func (s *Server) handleCaptureThumb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	maxW, _ := strconv.Atoi(q.Get("w"))
	if maxW <= 0 || maxW > 640 {
		maxW = 360
	}
	var jpg []byte
	var err error
	if hw := q.Get("window"); strings.HasPrefix(hw, "0x") {
		jpg, err = capture.WindowThumb(hw, maxW)
	} else {
		idx, _ := strconv.Atoi(q.Get("monitor"))
		jpg, err = encoder.MonitorThumb(s.m.FFmpeg(), idx, maxW)
	}
	if err != nil || len(jpg) == 0 {
		writeErr(w, 502, "couldn't capture a thumbnail")
		return
	}
	w.Header().Set("content-type", "image/jpeg")
	w.Header().Set("cache-control", "no-store")
	_, _ = w.Write(jpg)
}
