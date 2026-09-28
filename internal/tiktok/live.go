package tiktok

// CheckGoLivePermission reports whether the account may go LIVE from Studio.
func (c *Client) CheckGoLivePermission() map[string]any {
	out := map[string]any{}
	if r, err := c.Get(BaseWebcast+"/webcast/room/live_podcast/", nil); err == nil {
		if d, ok := r["data"].(map[string]any); ok {
			if scen, ok := d["live_scenario"].(map[string]any); ok {
				out["enable_live_studio"] = scen["enable_live_studio"]
				out["enable_live_video"] = scen["enable_live_video"]
				out["show_live_studio"] = scen["show_live_studio"]
			}
		}
	} else {
		out["live_podcast_error"] = err.Error()
	}
	if r, err := c.Get(BaseWebcast+"/webcast/eco/check_access/", &Req{Params: map[string]string{"permission_type": "1"}}); err == nil {
		if d, ok := r["data"].(map[string]any); ok {
			out["check_access"] = d
		}
	} else {
		out["check_access_error"] = err.Error()
	}
	return out
}

func (c *Client) ApplyStudioPermission() (map[string]any, error) {
	return c.Post(BaseWebcast+"/webcast/room/live_permission/apply/", &Req{
		Form: map[string]string{"permission_name": "live_studio"},
	})
}

// RoomConfig holds the settings for a new room. Anything not listed here is
// sent with the same values the desktop app uses.
type RoomConfig struct {
	Title       string
	HashtagID   string // topic, e.g. "5" = Gaming
	GameTagID   string // e.g. "9118" = EA SPORTS FC
	CoverURI    string
	LiveSubOnly bool
}

func (cfg RoomConfig) body() map[string]string {
	b := map[string]string{
		"title":                                  cfg.Title,
		"live_studio":                            "1",
		"gen_replay":                             "true",
		"chat_auth":                              "1",
		"gift_auth":                              "1",
		"hashtag_id":                             firstNonEmpty(cfg.HashtagID, "5"),
		"game_bitrate_type":                      "default",
		"screenshot_cover_status":                "1",
		"close_room_when_close_stream":           "false",
		"live_sub_only":                          boolStr(cfg.LiveSubOnly, "1", "0"),
		"disable_preview_sub_only":               "1",
		"chat_sub_only_auth":                     "2",
		"chat_l2":                                "1",
		"star_comment_switch":                    "true",
		"multi_stream_source":                    "1",
		"multi_stream_scene":                     "0",
		"rtc_net_enabled":                        "false",
		"is_group_live_session":                  "false",
		"open_commercial_content_toggle":         "false",
		"commercial_content_promote_myself":      "false",
		"commercial_content_promote_third_party": "false",
		"visible_scope_type":                     "0",
	}
	if cfg.GameTagID != "" {
		b["game_tag_id"] = cfg.GameTagID
	}
	if cfg.CoverURI != "" {
		b["cover_uri"] = cfg.CoverURI
	}
	return b
}

type StreamURLs struct {
	RoomID      string
	StreamID    string
	RTMPPushURL string
	PushURLs    []string
	ShareURL    string
	Raw         map[string]any
}

func (c *Client) CreateRoom(cfg RoomConfig) (*StreamURLs, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/room/create/", &Req{Form: cfg.body()})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	if d == nil {
		return nil, errf(0, resp, "room/create returned no data")
	}
	su, _ := d["stream_url"].(map[string]any)
	roomID := firstStr(d, "id_str", "id")
	rtmp := ""
	streamID := ""
	var pushURLs []string
	if su != nil {
		rtmp = str(su["rtmp_push_url"])
		streamID = firstStr(su, "id_str", "id")
		if lst, ok := su["push_urls"].([]any); ok {
			for _, u := range lst {
				pushURLs = append(pushURLs, str(u))
			}
		}
	}
	if roomID == "" || rtmp == "" {
		return nil, errf(0, resp, "room/create returned no stream_url (signer=noop may be gated)")
	}
	return &StreamURLs{
		RoomID: roomID, StreamID: streamID, RTMPPushURL: rtmp,
		PushURLs: pushURLs, ShareURL: str(d["share_url"]), Raw: d,
	}, nil
}

const (
	PingCreated  = 1
	PingLive     = 2
	PingFinished = 4
)

// PingAnchor reports the room's status. Studio sends PingLive every few seconds
// while streaming.
func (c *Client) PingAnchor(roomID, streamID string, status int) (map[string]any, error) {
	return c.Post(BaseWebcast+"/webcast/room/ping/anchor/", &Req{
		Form: map[string]string{"status": itoa(int64(status)), "room_id": roomID, "stream_id": streamID},
	})
}

// EndLive closes the room: pre_finish, finished pings, then finish_info. Errors
// are recorded rather than returned so a network hiccup on one step doesn't stop
// the rest.
func (c *Client) EndLive(roomID, streamID string) map[string]any {
	results := map[string]any{}
	if r, err := c.Get(BaseWebcast+"/webcast/room/anchor_pre_finish/", &Req{Params: map[string]string{"room_id": roomID}}); err == nil {
		results["pre_finish"] = r
	} else {
		results["pre_finish_error"] = err.Error()
	}
	for i := 0; i < 3; i++ {
		if _, err := c.PingAnchor(roomID, streamID, PingFinished); err != nil {
			results["ping_error"] = err.Error()
		}
	}
	if r, err := c.Post(BaseWebcast+"/webcast/room/anchor_finish_info/", &Req{Form: map[string]string{"room_id": roomID}}); err == nil {
		results["finish_info"] = r
	} else {
		results["finish_info_error"] = err.Error()
	}
	return results
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}
