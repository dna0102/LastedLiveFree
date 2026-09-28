package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"lastedlive/internal/manager"
	"lastedlive/internal/models"
)

func (s *Server) routesHostTools(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/accounts/{id}/chat", s.handleChat)
	mux.HandleFunc("GET /api/accounts/{id}/about-me", s.handleAboutMeGet)
	mux.HandleFunc("POST /api/accounts/{id}/about-me", s.handleAboutMeSet)
	mux.HandleFunc("GET /api/accounts/{id}/gift-gallery", s.handleGiftGallery)
	mux.HandleFunc("GET /api/accounts/{id}/top-gifters", s.handleTopGifters)
	mux.HandleFunc("GET /api/accounts/{id}/poll/templates", s.handlePollTemplates)
}

func userView(u map[string]any) map[string]any {
	if u == nil {
		return nil
	}
	av := ""
	for _, k := range []string{"avatar_thumb", "avatar_medium", "avatar_large"} {
		if img, ok := u[k].(map[string]any); ok {
			if l, ok := img["url_list"].([]any); ok && len(l) > 0 {
				av = fmt.Sprint(l[0])
				break
			}
		}
	}
	id := fmt.Sprint(u["id_str"])
	if id == "" || id == "<nil>" {
		id = fmt.Sprint(u["id"])
	}
	return map[string]any{"id": id, "nickname": u["nickname"], "handle": u["display_id"], "avatar": av,
		"follows_you": u["is_follower"], "you_follow": u["is_following"]}
}

func (s *Server) liveRoom(a *models.Account) string {
	if sess := s.m.Session(a.ID); sess != nil {
		return sess.RoomID
	}
	return ""
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	items, last, errMsg, live := s.m.ChatSince(r.PathValue("id"), since)
	if items == nil {
		items = []manager.ChatItem{}
	}
	writeJSON(w, 200, map[string]any{"live": live, "items": items, "seq": last, "error": errMsg})
}

func (s *Server) handleAboutMeGet(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.GetAboutMe()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	out := map[string]any{"text": "", "template_id": "6", "max": 240}
	if am, ok := d["about_me"].(map[string]any); ok {
		out["audit_status"] = am["audit_status"]
		cur := fmt.Sprint(am["current_template_id"])
		if tl, ok := am["template_list"].([]any); ok {
			for _, t := range tl {
				tm, _ := t.(map[string]any)
				if fmt.Sprint(tm["id"]) != cur {
					continue
				}
				out["template_id"] = cur
				if boxes, ok := tm["input_box_list"].([]any); ok && len(boxes) > 0 {
					bm, _ := boxes[0].(map[string]any)
					out["text"], out["max"] = bm["content"], bm["max_character_count"]
				}
			}
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleAboutMeSet(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	var b struct {
		Text       string `json:"text"`
		TemplateID string `json:"template_id"`
	}
	_ = readJSON(r, &b)
	if b.TemplateID == "" {
		b.TemplateID = "6" // the free-text ("Blank") template
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.UpdateAboutMe(b.TemplateID, []string{strings.TrimSpace(b.Text)})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleGiftGallery(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.GiftGallery(a.UserID, s.liveRoom(a))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	var gifts []any
	if l, ok := d["normal_gifts"].([]any); ok {
		for _, x := range l {
			g, _ := x.(map[string]any)
			sp, _ := g["sponsor_info"].(map[string]any)
			sponsor := ""
			if sp != nil {
				sponsor = fmt.Sprint(sp["nickname"])
			}
			gifts = append(gifts, map[string]any{
				"id": g["gift_id"], "name": g["name"], "coins": g["coin_price"],
				"sent": g["current_sent_count"], "goal": g["goal_count"], "lit": g["sponsored"],
				"image": g["image_url"], "unlit_image": g["unlighted_image_url"], "sponsor": sponsor,
			})
		}
	}
	writeJSON(w, 200, map[string]any{"gifts": gifts, "ends_at": d["current_period_ends_at"],
		"league": d["gallery_ranking_league"]})
}

func (s *Server) handleTopGifters(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.TopGifters()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	out := []any{}
	if l, ok := d["rank_list"].([]any); ok {
		for _, x := range l {
			m, _ := x.(map[string]any)
			u, _ := m["user"].(map[string]any)
			if v := userView(u); v != nil {
				v["score"] = m["score"]
				out = append(out, v)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"users": out, "room_id": d["latest_room_id_str"]})
}

func (s *Server) handlePollTemplates(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	d, err := cl.GetPollTemplates(room, a.UserID)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	var out []any
	if l, ok := d["templates"].([]any); ok {
		for _, x := range l {
			t, _ := x.(map[string]any)
			var opts []string
			if ol, ok := t["poll_option_list"].([]any); ok {
				for _, o := range ol {
					if om, ok := o.(map[string]any); ok {
						opts = append(opts, fmt.Sprint(om["display_content"]))
					} else {
						opts = append(opts, fmt.Sprint(o))
					}
				}
			}
			out = append(out, map[string]any{"id": t["id_str"], "options": opts, "duration_ms": t["duration_ms"]})
		}
	}
	writeJSON(w, 200, map[string]any{"templates": out})
}
