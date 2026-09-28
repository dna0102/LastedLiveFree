package tiktok

import (
	"encoding/json"
	"time"
)

// SendChat posts a comment as the host. TikTok drops it unless the request
// carries the native Argus signature, which we can't produce.
func (c *Client) SendChat(roomID, content string) (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/room/chat/", &Req{
		JSON: map[string]any{
			"room_id":                            roomID,
			"content":                            content,
			"emotes_with_index":                  "",
			"client_start_timestamp_millisecond": time.Now().UnixMilli(),
		},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) StartPoll(roomID string, options []string, durationMs int) (map[string]any, error) {
	optJSON, _ := json.Marshal(options)
	resp, err := c.Post(BaseWebcast+"/webcast/room/poll/start", &Req{
		JSON: map[string]any{
			"room_id":     roomID,
			"option_list": string(optJSON), // a JSON string, not an array
			"kind":        0,
			"duration_ms": durationMs,
		},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) EndPoll(roomID, pollID string) (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/room/poll/end", &Req{
		JSON: map[string]any{"room_id": roomID, "end_type": 1, "poll_id": pollID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) QueryPoll(roomID, pollID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/room/poll/query", &Req{
		Params: map[string]string{"room_id": roomID, "poll_id": pollID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) LatestPoll(roomID string) (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/room/poll/latest", &Req{
		Form: map[string]string{"room_id": roomID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetPollTemplates(roomID, anchorID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/room/poll/customizable/templates/list", &Req{
		Params: map[string]string{"room_id": roomID, "anchor_id": anchorID},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// UpdateLiveTitle changes the title (and optionally the cover) of a running LIVE.
func (c *Client) UpdateLiveTitle(roomID, title, coverURI string) (map[string]any, error) {
	body := map[string]any{"title": title, "room_id": roomID}
	if coverURI != "" {
		body["cover_uri"] = coverURI
	}
	resp, err := c.Post(BaseWebcast+"/webcast/anchor/live_info/update/", &Req{JSON: body})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// UpdateAboutMe sets the "About me" text. TikTok reviews it before it shows.
func (c *Client) UpdateAboutMe(templateID string, contents []string) (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/anchor/about_me/update/", &Req{
		JSON: map[string]any{"template_id": templateID, "method_type": 1, "input_content_list": contents},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// SetScreenshotCover turns the automatic screenshot cover on (1) or off (0).
func (c *Client) SetScreenshotCover(status int) (map[string]any, error) {
	return c.Post(BaseWebcast+"/webcast/room/screenshot_cover/update/", &Req{
		Form: map[string]string{"status": itoa(int64(status))},
	})
}

// PinGoal pins a sub-goal on screen. Needs the native signature (returns 403
// without it).
func (c *Client) PinGoal(goalID, roomID, subGoalID string) (map[string]any, error) {
	return c.Post(BaseWebcast+"/webcast/goal/pin/", &Req{
		Form: map[string]string{"goal_id": goalID, "room_id": roomID, "sub_goal_id": subGoalID, "type": "1"},
	})
}

// CommitGoal creates or updates a LIVE goal. Each sub-goal carries a full gift
// object, so the caller builds the body. Without goal_id_str TikTok creates a
// new goal; with it, that goal is updated.
func (c *Client) CommitGoal(body map[string]any) (map[string]any, error) {
	resp, err := c.Post(BaseWebcast+"/webcast/goal/commit/", &Req{JSON: body})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}
