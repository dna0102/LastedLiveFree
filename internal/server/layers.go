package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lastedlive/internal/encoder"
)

var imageTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp"}

// Formats the webview can play, for "Hear it in Studio".
var mediaTypes = map[string]string{".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime",
	".webm": "video/webm", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".wav": "audio/wav"}

func (s *Server) routesLayers(mux *http.ServeMux) {
	// Studio canvas layers: {"layers": {"<key>": <source>, ...}}
	mux.HandleFunc("POST /api/accounts/{id}/layers", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Layers map[string]encoder.Source `json:"layers"`
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, 400, "bad layers")
			return
		}
		writeJSON(w, 200, map[string]any{"errors": s.m.SyncLayers(r.PathValue("id"), b.Layers)})
	})
	mux.HandleFunc("GET /api/accounts/{id}/layers/{key}/frame", func(w http.ResponseWriter, r *http.Request) {
		jpg, seq, err := s.m.LayerFrame(r.PathValue("id"), r.PathValue("key"))
		since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
		w.Header().Set("cache-control", "no-store")
		if err != nil || jpg == nil || seq <= since {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("content-type", "image/jpeg")
		w.Header().Set("x-frame-seq", strconv.FormatUint(seq, 10))
		_, _ = w.Write(jpg)
	})
	// Video files for "Hear it in Studio". ServeContent handles Range requests.
	mux.HandleFunc("GET /api/media", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		ct, ok := mediaTypes[strings.ToLower(filepath.Ext(p))]
		if !ok {
			writeErr(w, 403, "not a supported video")
			return
		}
		f, err := os.Open(p)
		if err != nil {
			writeErr(w, 404, "file not found")
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			writeErr(w, 404, "file not found")
			return
		}
		w.Header().Set("content-type", ct)
		http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
	})
	// Image files for image layers. Only image extensions are served.
	mux.HandleFunc("GET /api/file", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		ct, ok := imageTypes[strings.ToLower(filepath.Ext(p))]
		if !ok {
			writeErr(w, 403, "not an image")
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			writeErr(w, 404, "file not found")
			return
		}
		w.Header().Set("content-type", ct)
		w.Header().Set("cache-control", "no-store")
		_, _ = w.Write(b)
	})
}
