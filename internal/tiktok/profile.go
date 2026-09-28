package tiktok

import "encoding/json"

func firstURL(obj any) string {
	m, ok := obj.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"url_list", "urls"} {
		if lst, ok := m[key].([]any); ok && len(lst) > 0 {
			return str(lst[0])
		}
	}
	return ""
}

type Profile struct {
	UserID         string         `json:"user_id"`
	SecUserID      string         `json:"sec_user_id"`
	Nickname       string         `json:"nickname"`
	DisplayID      string         `json:"display_id"`
	Bio            string         `json:"bio"`
	AvatarURL      string         `json:"avatar_url"`
	FollowerCount  int            `json:"follower_count"`
	FollowingCount int            `json:"following_count"`
	TotalLikes     int            `json:"total_likes"`
	VideoCount     int            `json:"video_count"`
	Verified       bool           `json:"verified"`
	Raw            map[string]any `json:"-"`
}

func (c *Client) GetFullProfile(userID string) (*Profile, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/user/", &Req{
		Params: map[string]string{"target_uid": userID, "anchor_id": userID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	if d == nil {
		return nil, errf(0, resp, "no profile data")
	}
	follow, _ := d["follow_info"].(map[string]any)
	stats, _ := d["author_stats"].(map[string]any)
	avatar := firstURL(d["avatar_medium"])
	if avatar == "" {
		avatar = firstURL(d["avatar_large"])
	}
	if avatar == "" {
		avatar = firstURL(d["avatar_thumb"])
	}
	p := &Profile{
		UserID:    firstStr(d, "id", "short_id"),
		SecUserID: str(d["sec_uid"]),
		Nickname:  str(d["nickname"]),
		DisplayID: str(d["display_id"]),
		Bio:       str(d["bio_description"]),
		AvatarURL: avatar,
		Verified:  boolVal(d["verified"]),
		Raw:       d,
	}
	if follow != nil {
		p.FollowerCount = numAsInt(follow["follower_count"])
		p.FollowingCount = numAsInt(follow["following_count"])
	}
	if stats != nil {
		p.TotalLikes = numAsInt(stats["video_total_favorite_count"])
		p.VideoCount = numAsInt(stats["video_total_count"])
	}
	return p, nil
}

func (c *Client) GetLiveInfo() (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/anchor/live_info/get/", nil)
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetAlertboxSetting() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/game/studio/fetch_alertbox_setting/")
}

func (c *Client) GetGoalsourceSetting() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/game/studio/fetch_goalsource_setting/")
}

func (c *Client) getData(url string) (map[string]any, error) {
	resp, err := c.Get(url, nil)
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetMaterials(roomID string) (map[string]any, error) {
	inner, _ := json.Marshal(map[string]any{"room_id": roomID, "tags": []any{}, "offset": 0, "count": 50})
	resp, err := c.Post(BaseWebcast+"/webcast/game/studio/fetch_material/", &Req{
		Form: map[string]string{"data": string(inner)},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func boolVal(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case json.Number:
		return t.String() != "0" && t.String() != ""
	case string:
		return t == "true" || t == "1"
	default:
		return false
	}
}
