package tiktok

// Read-only host features. Most return TikTok's data object as-is; the stats
// are typed because they're polled constantly.

// RealtimeStats are the running totals for the current LIVE. WatchCnt counts
// every view so far, not the current audience.
type RealtimeStats struct {
	IsLive      bool   `json:"is_live"`
	RoomID      string `json:"room_id"`
	WatchCnt    int    `json:"live_watch_cnt"`
	LikeCnt     int    `json:"live_like_cnt"`
	CommentCnt  int    `json:"live_comment_cnt"`
	ConsumeUcnt int    `json:"live_consume_ucnt"`
	NewFans     int    `json:"live_new_fans_ucnt"`
	NewSubs     int    `json:"new_subscribers_cnt"`
	TotalScore  int    `json:"total_score"`
}

func (c *Client) GetRealtimeStats() (*RealtimeStats, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/game/studio/realtime_stats/", nil)
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	if d == nil {
		return &RealtimeStats{}, nil
	}
	rs := &RealtimeStats{IsLive: boolVal(d["is_live"])}
	if room, ok := d["room_stats"].(map[string]any); ok {
		rs.RoomID = str(room["room_id"])
		rs.WatchCnt = numAsInt(room["live_watch_cnt"])
		rs.LikeCnt = numAsInt(room["live_like_cnt"])
		rs.CommentCnt = numAsInt(room["live_comment_cnt"])
		rs.ConsumeUcnt = numAsInt(room["live_consume_ucnt"])
		rs.NewFans = numAsInt(room["live_new_fans_ucnt"])
		rs.NewSubs = numAsInt(room["new_subscribers_cnt"])
		rs.TotalScore = numAsInt(room["total_score"])
	}
	return rs, nil
}

func (c *Client) GetRoomStats() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/game/studio/room_stats/")
}

func (c *Client) GetRealtimeRewards(roomID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/wallet_api_tiktok/real_time_live_rewards", &Req{
		Params: map[string]string{"room_id": roomID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetEndLiveRewards() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/wallet_api_tiktok/room_end_live_rewards")
}

func (c *Client) GetWalletInfo() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/wallet_api_tiktok/wallet/info/")
}

func (c *Client) GetOnlineAudience(roomID, anchorID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/ranklist/online_audience/", &Req{
		Params: map[string]string{"room_id": roomID, "anchor_id": anchorID, "source": "0"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetChatSettingConfigs() (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/anchor_tool/chat_setting/config/list/", &Req{
		Params: map[string]string{"offset": "0", "count": "20"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetAnchorSettings() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/room/anchor_settings/read/")
}

func (c *Client) GetGoodyBagTemplate() (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/goody_bag/template/", &Req{
		Form: map[string]string{"room_id": "0", "biz": "1", "scene": "1"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetGoodyBagRoom(roomID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/goody_bag/room/", &Req{Params: map[string]string{"room_id": roomID}})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetEnvelopeTemplate(anchorID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/envelope/template/", &Req{
		Params: map[string]string{"room_id": "0", "anchor_id": anchorID, "scene": "1"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetEnvelopeList(roomID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/envelope/list/", &Req{Params: map[string]string{"room_id": roomID}})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// GetGoal returns the host's goal state. It doesn't include a room's goal
// unless asked for it by id; see GetRoomGoal.
func (c *Client) GetGoal(secOwnerID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/goal/get/", &Req{
		Params: map[string]string{"sec_owner_id": secOwnerID, "type": "1", "source": "5"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// GetRoomGoal returns one goal of a room (as data.specified_goal).
func (c *Client) GetRoomGoal(secOwnerID, roomID, goalID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/goal/get/", &Req{
		Params: map[string]string{"sec_owner_id": secOwnerID, "room_id": roomID, "type": "1", "source": "5", "goal_id": goalID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetGoalHistory(secOwnerID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/goal/history/", &Req{
		Params: map[string]string{"sec_owner_id": secOwnerID, "type": "1", "offset": "0", "limit": "5"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetSubGoal(secOwnerID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/sub/goal/get/", &Req{
		Params: map[string]string{"sec_owner_id": secOwnerID, "room_id": "", "goal_id": "0", "goal_scene": "0"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetSubInfo(secAnchorID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/sub/privilege/get_sub_info/", &Req{
		Params: map[string]string{"sec_anchor_id": secAnchorID, "source": "live_take_page",
			"need_entrance_data": "true", "need_current_state": "true"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}
