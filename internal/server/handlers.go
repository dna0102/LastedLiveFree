package server

import (
	"fmt"
	"net/http"
	"strconv"

	"lastedlive/internal/encoder"
	"lastedlive/internal/models"
	"lastedlive/internal/tiktok"
)

// clientAndRoom returns the account's client and LIVE room id, or writes a
// 409 if it isn't LIVE.
func (s *Server) clientAndRoom(w http.ResponseWriter, a *models.Account) (*tiktok.Client, string, bool) {
	sess := s.m.Session(a.ID)
	if sess == nil {
		writeErr(w, 409, "this account isn't LIVE")
		return nil, "", false
	}
	cl, err := s.m.ClientFor(a)
	if err != nil {
		writeErr(w, 502, err.Error())
		return nil, "", false
	}
	return cl, sess.RoomID, true
}

// secUID returns the account's sec_user_id, loading the profile first if it's
// missing (accounts imported from the old tool don't have it). Writes a 502 if
// it still can't be found.
func (s *Server) secUID(w http.ResponseWriter, a *models.Account) (string, bool) {
	if a.SecUserID == "" {
		_, _ = s.m.Validate(a)
	}
	if a.SecUserID == "" {
		writeErr(w, 502, "couldn't load this account's profile — try Refresh profile on the dashboard")
		return "", false
	}
	return a.SecUserID, true
}

func (s *Server) client(w http.ResponseWriter, a *models.Account) *tiktok.Client {
	cl, err := s.m.ClientFor(a)
	if err != nil {
		writeErr(w, 502, err.Error())
		return nil
	}
	return cl
}

func (s *Server) handleGoLive(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	var b struct {
		Title     string `json:"title"`
		HashtagID string `json:"hashtag_id"`
		GameTagID string `json:"game_tag_id"`
	}
	_ = readJSON(r, &b)
	sess, err := s.m.GoLive(a, b.Title, b.HashtagID, b.GameTagID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, sess.View())
}

func (s *Server) handleEndLive(w http.ResponseWriter, r *http.Request) {
	res, err := s.m.EndLive(r.PathValue("id"))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	sess := s.m.Session(r.PathValue("id"))
	if sess == nil {
		writeJSON(w, 200, map[string]any{"live": false})
		return
	}
	v := sess.View()
	v["live"] = true
	writeJSON(w, 200, v)
}

func (s *Server) handleLiveInfo(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.GetLiveInfo()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	out := map[string]any{"title": d["current_title"]}
	if c, ok := d["current_room_cover"].(map[string]any); ok {
		if l, ok := c["url_list"].([]any); ok && len(l) > 0 {
			out["cover_url"] = l[0]
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleEncStart(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	var b struct {
		Config encoder.Config `json:"config"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "bad scene config")
		return
	}
	st, err := s.m.StartEncoder(a.ID, b.Config)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleEncStop(w http.ResponseWriter, r *http.Request) {
	if err := s.m.StopEncoder(r.PathValue("id")); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleEncStatus(w http.ResponseWriter, r *http.Request) {
	st := s.m.EncoderStatus(r.PathValue("id"))
	if st == nil {
		writeJSON(w, 200, map[string]any{"running": false})
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handlePreviewStart(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	var b struct {
		Config encoder.Config `json:"config"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "bad scene config")
		return
	}
	if err := s.m.StartPreview(a.ID, b.Config); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handlePreviewFrame returns the latest preview JPEG, or 204 if there's nothing
// newer than ?since=<seq>.
func (s *Server) handlePreviewFrame(w http.ResponseWriter, r *http.Request) {
	jpg, seq := s.m.PreviewFrame(r.PathValue("id"))
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	if jpg == nil || seq <= since {
		w.Header().Set("cache-control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("content-type", "image/jpeg")
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("x-frame-seq", strconv.FormatUint(seq, 10))
	w.Header().Set("access-control-expose-headers", "x-frame-seq")
	_, _ = w.Write(jpg)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	st, err := cl.GetRealtimeStats()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	// live_watch_cnt ("watch") counts every view so far. The current audience
	// comes from the message feed, or the online audience list until the feed
	// has reported.
	var viewers any
	if n, ok := s.m.CurrentViewers(a.ID); ok {
		viewers = n
	} else if sess := s.m.Session(a.ID); sess != nil {
		if aud, err := cl.GetOnlineAudience(sess.RoomID, a.UserID); err == nil {
			viewers = aud["total"]
		}
	}
	writeJSON(w, 200, map[string]any{
		"live":    s.m.IsLive(a.ID), // ours, not TikTok's is_live
		"viewers": viewers, "views": st.WatchCnt,
		"watch": st.WatchCnt, "likes": st.LikeCnt, "comments": st.CommentCnt,
		"gifters": st.ConsumeUcnt, "new_fans": st.NewFans, "new_subs": st.NewSubs, "diamonds": st.TotalScore,
	})
}

func (s *Server) handleRewards(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	d, err := cl.GetRealtimeRewards(room)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"amount": d["amount"], "currency": d["currency"], "diamonds": d["diamonds"]})
}

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.GetWalletInfo()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	coins := any(0)
	if ex, ok := d["exchange"].(map[string]any); ok {
		coins = ex["coins"]
	}
	writeJSON(w, 200, map[string]any{"diamonds": d["diamond"], "coins": coins, "frozen": d["frozen_diamond"]})
}

func (s *Server) handleAudience(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	d, err := cl.GetOnlineAudience(room, a.UserID)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	type viewer struct {
		Rank   int    `json:"rank"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
		Score  any    `json:"score"`
	}
	var out []viewer
	if ranks, ok := d["ranks"].([]any); ok {
		for i, x := range ranks {
			rk, _ := x.(map[string]any)
			u, _ := rk["user"].(map[string]any)
			v := viewer{Rank: i + 1, Score: rk["score"]}
			if u != nil {
				v.Name = fmt.Sprint(u["nickname"])
				if av, ok := u["avatar_thumb"].(map[string]any); ok {
					if l, ok := av["url_list"].([]any); ok && len(l) > 0 {
						v.Avatar = fmt.Sprint(l[0])
					}
				}
			}
			out = append(out, v)
		}
	}
	writeJSON(w, 200, map[string]any{"viewers": out, "total": d["total"]})
}

func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	sec, ok := s.secUID(w, a)
	if !ok {
		return
	}
	var d map[string]any
	var err error
	if sess := s.m.Session(a.ID); sess != nil && sess.GoalID != "" {
		d, err = cl.GetRoomGoal(sec, sess.RoomID, sess.GoalID)
	} else {
		d, err = cl.GetGoal(sec)
	}
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleTitle(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	var b struct {
		Title string `json:"title"`
	}
	_ = readJSON(r, &b)
	cover := ""
	if li, err := cl.GetLiveInfo(); err == nil {
		if c, ok := li["current_room_cover"].(map[string]any); ok {
			cover = fmt.Sprint(c["uri"])
		}
	}
	if _, err := cl.UpdateLiveTitle(room, b.Title, cover); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if sess := s.m.Session(a.ID); sess != nil {
		sess.Title = b.Title
	}
	writeJSON(w, 200, map[string]any{"ok": true, "title": b.Title})
}

func (s *Server) handlePollStart(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	var b struct {
		Options    []string `json:"options"`
		DurationMs int      `json:"duration_ms"`
	}
	_ = readJSON(r, &b)
	if len(b.Options) < 2 {
		writeErr(w, 400, "add at least 2 options")
		return
	}
	if b.DurationMs == 0 {
		b.DurationMs = 60000
	}
	d, err := cl.StartPoll(room, b.Options, b.DurationMs)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handlePollEnd(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	var b struct {
		PollID string `json:"poll_id"`
	}
	_ = readJSON(r, &b)
	d, err := cl.EndPoll(room, b.PollID)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handlePollQuery(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	d, err := cl.QueryPoll(room, r.URL.Query().Get("poll_id"))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleWishesGet(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	d, err := cl.VPGetSettings()
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	type item struct {
		GiftID any    `json:"gift_id"`
		Label  string `json:"label"`
	}
	var items []item
	if lst, ok := d["gift_pick_list"].([]any); ok {
		for _, p := range lst {
			pm, _ := p.(map[string]any)
			gp, _ := pm["gift_pick"].(map[string]any)
			if gp != nil {
				items = append(items, item{GiftID: gp["gift_id"], Label: fmt.Sprint(gp["customized_desc"])})
			}
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "display_mode": d["display_mode"],
		"round_duration_sec": d["round_duration_sec"]})
}

func (s *Server) handleWishesStart(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	items, opts := readWishes(r)
	if len(items) == 0 {
		writeErr(w, 400, "add at least one wish")
		return
	}
	// TikTok checks /start against the wishes saved on the account.
	if err := cl.VPSave(items, opts); err != nil {
		writeErr(w, 502, "couldn't save your wishes to TikTok: "+err.Error())
		return
	}
	d, err := cl.VPStart(room, items, opts)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

// handleWishesSave saves the wishes to the account without starting a round.
func (s *Server) handleWishesSave(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl := s.client(w, a)
	if cl == nil {
		return
	}
	items, opts := readWishes(r)
	if err := cl.VPSave(items, opts); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func readWishes(r *http.Request) ([]tiktok.Wish, tiktok.VPOptions) {
	var b struct {
		Items []struct {
			GiftID int    `json:"gift_id"`
			Label  string `json:"label"`
		} `json:"items"`
		DisplayMode      int  `json:"display_mode"`
		RoundDurationSec int  `json:"round_duration_sec"`
		HasScore         bool `json:"has_score"`
		HasDuration      bool `json:"has_duration"`
		AutoRestart      bool `json:"enable_auto_restart"`
	}
	_ = readJSON(r, &b)
	var items []tiktok.Wish
	for _, it := range b.Items {
		items = append(items, tiktok.Wish{GiftID: it.GiftID, Label: it.Label})
	}
	opts := tiktok.DefaultVPOptions()
	if b.DisplayMode != 0 {
		opts.DisplayMode = b.DisplayMode
	}
	if b.RoundDurationSec != 0 {
		opts.RoundDurationSec = b.RoundDurationSec
	}
	opts.HasScore, opts.HasDuration, opts.EnableAutoRestart = b.HasScore, b.HasDuration, b.AutoRestart
	return items, opts
}
