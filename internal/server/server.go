// Package server is the /api HTTP handler. Wails serves it from the webview's
// asset handler, so it runs in-process and nothing listens on a port.
package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"lastedlive/internal/encoder"
	"lastedlive/internal/ffsetup"
	"lastedlive/internal/manager"
	"lastedlive/internal/models"
	"lastedlive/internal/sysstats"
)

//go:embed assets/gifts.json
var giftsJSON []byte

//go:embed assets/gametags.json
var gametagsJSON []byte

type gameTags struct {
	Hashtags []map[string]any `json:"hashtags"`
	Games    []struct {
		ID   any    `json:"id"`
		Name string `json:"name"`
	} `json:"games"`
}

type Server struct {
	m     *manager.Manager
	setup *ffsetup.Installer
	tags  gameTags
}

func New(m *manager.Manager, setup *ffsetup.Installer) http.Handler {
	s := &Server{m: m, setup: setup}
	_ = json.Unmarshal(gametagsJSON, &s.tags)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/gifts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.Header().Set("cache-control", "max-age=86400")
		w.Write(giftsJSON)
	})
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"hashtags": s.tags.Hashtags})
	})
	mux.HandleFunc("GET /api/games", s.handleGames)
	mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"cpu": sysstats.CPUPercent(), "mem": sysstats.MemPercent()})
	})
	mux.HandleFunc("GET /api/img", s.handleImg)
	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.m.EncoderDevices())
	})
	mux.HandleFunc("GET /api/ffmpeg/setup", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.setup.Status())
	})
	mux.HandleFunc("POST /api/ffmpeg/setup", func(w http.ResponseWriter, r *http.Request) {
		if err := s.setup.Start(); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, s.setup.Status())
	})

	mux.HandleFunc("POST /api/login/qr/start", s.handleQRStart)
	mux.HandleFunc("GET /api/login/qr/poll", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.m.QRPoll(r.URL.Query().Get("id")))
	})
	mux.HandleFunc("POST /api/login/session", s.handleSessionLogin)

	mux.HandleFunc("GET /api/accounts", s.handleListAccounts)
	mux.HandleFunc("DELETE /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.m.RemoveAccount(r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/accounts/{id}/validate", s.handleValidate)

	mux.HandleFunc("GET /api/scenes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"scenes": s.m.ListScenes()})
	})
	mux.HandleFunc("POST /api/scenes", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		}
		if readJSON(r, &b) != nil || strings.TrimSpace(b.Name) == "" {
			writeErr(w, 400, "scene name required")
			return
		}
		writeJSON(w, 200, map[string]any{"scenes": s.m.SaveScene(strings.TrimSpace(b.Name), b.Config)})
	})
	mux.HandleFunc("DELETE /api/scenes/{name}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"scenes": s.m.DeleteScene(r.PathValue("name"))})
	})

	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"sessions": s.m.Sessions()})
	})
	mux.HandleFunc("POST /api/accounts/{id}/go-live", s.handleGoLive)
	mux.HandleFunc("POST /api/accounts/{id}/end-live", s.handleEndLive)
	mux.HandleFunc("GET /api/accounts/{id}/live", s.handleLive)
	mux.HandleFunc("GET /api/accounts/{id}/liveinfo", s.handleLiveInfo)

	mux.HandleFunc("POST /api/accounts/{id}/encoder/start", s.handleEncStart)
	mux.HandleFunc("POST /api/accounts/{id}/encoder/stop", s.handleEncStop)
	mux.HandleFunc("POST /api/accounts/{id}/encoder/audio", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Config encoder.Config `json:"config"`
		}
		if err := readJSON(r, &b); err != nil {
			writeErr(w, 400, "bad config")
			return
		}
		if err := s.m.SetLiveAudio(r.PathValue("id"), b.Config); err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/accounts/{id}/encoder/status", s.handleEncStatus)
	mux.HandleFunc("POST /api/accounts/{id}/preview/start", s.handlePreviewStart)
	mux.HandleFunc("POST /api/accounts/{id}/preview/stop", func(w http.ResponseWriter, r *http.Request) {
		s.m.StopPreview(r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/accounts/{id}/preview/frame", s.handlePreviewFrame)

	mux.HandleFunc("GET /api/accounts/{id}/stats", s.handleStats)
	mux.HandleFunc("GET /api/accounts/{id}/rewards", s.handleRewards)
	mux.HandleFunc("GET /api/accounts/{id}/wallet", s.handleWallet)
	mux.HandleFunc("GET /api/accounts/{id}/audience", s.handleAudience)
	mux.HandleFunc("GET /api/accounts/{id}/goals", s.handleGoals)
	mux.HandleFunc("POST /api/accounts/{id}/goals", s.handleGoalCommit)
	s.routesHostTools(mux)
	s.routesSources(mux)
	s.routesLayers(mux)
	mux.HandleFunc("POST /api/accounts/{id}/title", s.handleTitle)
	mux.HandleFunc("POST /api/accounts/{id}/poll/start", s.handlePollStart)
	mux.HandleFunc("POST /api/accounts/{id}/poll/end", s.handlePollEnd)
	mux.HandleFunc("GET /api/accounts/{id}/poll/query", s.handlePollQuery)
	mux.HandleFunc("GET /api/accounts/{id}/wishes", s.handleWishesGet)
	mux.HandleFunc("POST /api/accounts/{id}/wishes/save", s.handleWishesSave)
	mux.HandleFunc("POST /api/accounts/{id}/wishes/start", s.handleWishesStart)

	// unknown /api paths get a JSON 404; everything else is served by Wails
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json")
	w.Header().Set("cache-control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) *models.Account {
	a := s.m.Store.Get(r.PathValue("id"))
	if a == nil {
		writeErr(w, 404, "no such account")
		return nil
	}
	return a
}

func (s *Server) publicAccount(a *models.Account) map[string]any {
	pv := a.Public()
	pv["live"] = s.m.IsLive(a.ID)
	if sess := s.m.Session(a.ID); sess != nil {
		pv["session"] = sess.View()
	}
	return pv
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	list := s.m.Store.All()
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, s.publicAccount(a))
	}
	writeJSON(w, 200, map[string]any{"accounts": out})
}

func (s *Server) handleQRStart(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Label string `json:"label"`
	}
	_ = readJSON(r, &b)
	id, st, err := s.m.QRStart(b.Label)
	if errors.Is(err, manager.ErrAccountLimit) {
		writeErr(w, 409, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"login_id": id, "qrcode_png_b64": st.PNGBase64, "expire_time": st.ExpireTime})
}

func (s *Server) handleSessionLogin(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Label    string `json:"label"`
		Cookies  string `json:"cookies"`
		DeviceID string `json:"device_id"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	a, err := s.m.ImportSession(b.Label, b.Cookies, b.DeviceID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"account": s.publicAccount(a)})
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	if _, err := s.m.Validate(a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "account": s.publicAccount(a)})
}

func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	aliases := map[string]string{"eafc": "ea sports fc", "fifa": "ea sports fc", "cod": "call of duty",
		"gta": "grand theft auto", "lol": "league of legends", "valo": "valorant", "r6": "rainbow six",
		"dbd": "dead by daylight", "rl": "rocket league", "genshin": "genshin impact", "ff": "free fire"}
	terms := []string{q}
	if a, ok := aliases[q]; ok {
		terms = append(terms, a)
	}
	type g struct {
		ID   any    `json:"id"`
		Name string `json:"name"`
	}
	var out []g
	for _, it := range s.tags.Games {
		name := strings.ToLower(it.Name)
		for _, t := range terms {
			if t == "" || strings.Contains(name, t) {
				out = append(out, g{ID: it.ID, Name: it.Name})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		si := strings.HasPrefix(strings.ToLower(out[i].Name), q)
		sj := strings.HasPrefix(strings.ToLower(out[j].Name), q)
		return si && !sj
	})
	if len(out) > 30 {
		out = out[:30]
	}
	writeJSON(w, 200, map[string]any{"games": out})
}
