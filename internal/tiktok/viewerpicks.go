package tiktok

// Viewer Wishes (viewer_picks): TikTok's on-screen list of gifts the host is
// asking for. Up to six gifts, each with an optional label.

const (
	vpBase   = BaseWebcast + "/webcast/viewer_picks"
	MaxPicks = 6
	VPScene  = 3
)

func (c *Client) VPGetSettings() (map[string]any, error) {
	return c.getData(vpBase + "/settings")
}

func (c *Client) VPStart(roomID string, items []Wish, o VPOptions) (map[string]any, error) {
	var list []map[string]any
	for _, it := range items {
		if it.GiftID == 0 {
			continue
		}
		list = append(list, map[string]any{
			"gift_id":         it.GiftID,
			"customized_desc": it.Label,
			"extra":           map[string]any{},
		})
		if len(list) >= MaxPicks {
			break
		}
	}
	body := map[string]any{
		"room_id":                   roomID,
		"gift_pick_list":            list,
		"round_duration_sec":        o.RoundDurationSec,
		"has_score":                 o.HasScore,
		"has_duration":              o.HasDuration,
		"enable_auto_restart":       o.EnableAutoRestart,
		"started_from_auto_restart": false,
		"display_mode":              o.DisplayMode,
		"scene":                     VPScene,
	}
	resp, err := c.Post(vpBase+"/start", &Req{JSON: body})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) VPInfo(roomID string) (map[string]any, error) {
	resp, err := c.Get(vpBase+"/info", &Req{Params: map[string]string{"room_id": roomID}})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

type Wish struct {
	GiftID int    `json:"gift_id"`
	Label  string `json:"label"`
}

type VPOptions struct {
	DisplayMode       int  `json:"display_mode"`       // 1 horizontal, 2 vertical
	RoundDurationSec  int  `json:"round_duration_sec"` // 300/900/1800/3600
	HasScore          bool `json:"has_score"`
	HasDuration       bool `json:"has_duration"`
	EnableAutoRestart bool `json:"enable_auto_restart"`
}

// DefaultVPOptions: vertical, 60 minute rounds, score/timer/auto-restart on.
func DefaultVPOptions() VPOptions {
	return VPOptions{DisplayMode: 2, RoundDurationSec: 3600, HasScore: true, HasDuration: true, EnableAutoRestart: true}
}
