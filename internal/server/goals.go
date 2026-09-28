package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type catalogGift struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Diamonds int    `json:"diamonds"`
	Icon     string `json:"icon"`
}

func giftByID(id int) *catalogGift {
	var list []catalogGift
	if json.Unmarshal(giftsJSON, &list) != nil {
		return nil
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

// iconURI turns a CDN url (.../img/alisg/webcast-sg/resource/x.png~tplv-obj.png)
// into the uri form used in gift objects (webcast-sg/resource/x.png).
func iconURI(u string) string {
	if i := strings.Index(u, "/img/"); i >= 0 {
		u = u[i+len("/img/"):]
		if j := strings.Index(u, "/"); j >= 0 {
			u = u[j+1:]
		}
	}
	if i := strings.Index(u, "~"); i >= 0 {
		u = u[:i]
	}
	return u
}

// handleGoalCommit sets the LIVE goal. The first commit on a LIVE creates the
// goal; its id is kept on the session so later commits update it and GET
// /goals can find it.
func (s *Server) handleGoalCommit(w http.ResponseWriter, r *http.Request) {
	a := s.account(w, r)
	if a == nil {
		return
	}
	cl, room, ok := s.clientAndRoom(w, a)
	if !ok {
		return
	}
	var b struct {
		Description string `json:"description"`
		Reward      string `json:"reward"`
		Items       []struct {
			GiftID int `json:"gift_id"`
			Target int `json:"target"`
		} `json:"items"`
	}
	_ = readJSON(r, &b)
	if len(b.Items) == 0 {
		writeErr(w, 400, "add at least one gift")
		return
	}
	var subs []map[string]any
	for _, it := range b.Items {
		g := giftByID(it.GiftID)
		if g == nil {
			writeErr(w, 400, fmt.Sprintf("unknown gift %d", it.GiftID))
			return
		}
		if it.Target < 1 {
			it.Target = 1
		}
		subs = append(subs, map[string]any{
			"gift": map[string]any{
				"diamond_count": g.Diamonds,
				"icon": map[string]any{"avg_color": "", "height": 0, "image_type": 0, "is_animated": false,
					"open_web_url": "", "uri": iconURI(g.Icon), "url_list": []string{g.Icon}, "width": 0},
				"name": g.Name,
				"type": 1,
			},
			"id": g.ID, "id_str": strconv.Itoa(g.ID), "progress": 0, "recommended_header": "",
			"recommended_text": "", "source": 0, "target": it.Target, "type": 1,
		})
	}
	desc := strings.TrimSpace(b.Description)
	if desc == "" {
		desc = "Let’s reach this LIVE goal together!"
	}
	body := map[string]any{
		"description": desc, "room_id_str": room,
		"status": 2, "type": 1, "subgoals": subs, "auto_create": 1,
		"goal_reward": map[string]any{"reward_content": strings.TrimSpace(b.Reward), "top_n": 0, "has_sticker": false},
	}
	sess := s.m.Session(a.ID)
	if sess != nil && sess.GoalID != "" {
		body["goal_id_str"] = sess.GoalID
	}
	d, err := cl.CommitGoal(body)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if g, ok := d["goal"].(map[string]any); ok && sess != nil {
		if id := fmt.Sprint(g["id_str"]); id != "" && id != "<nil>" {
			sess.GoalID = id
		}
	}
	writeJSON(w, 200, d)
}
