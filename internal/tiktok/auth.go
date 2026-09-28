package tiktok

import "encoding/json"

// Session checks. Logging in happens in login.go (QR) or by pasting cookies;
// this file only validates a session and keeps it fresh.

type LoginInfo struct {
	UserID        string
	SecUserID     string
	Username      string
	Nickname      string
	StoreCountry  string
	Email         string
	FollowerCount int
	Raw           map[string]any
}

// CheckLogin confirms the cookies still belong to a logged-in account.
// passport/account/info/v2 doesn't need Argus, so it works unsigned.
func (c *Client) CheckLogin() (*LoginInfo, error) {
	resp, err := c.Post(BaseWWW+"/passport/account/info/v2/", &Req{
		Form:     map[string]string{"aid": AID, "language": "en", "need_group_app_id": "1"},
		NoCommon: true,
	})
	if err != nil {
		return nil, err
	}
	data := resp
	if d, ok := resp["data"].(map[string]any); ok {
		data = d
	}
	uid := str(data["uid"])
	if uid == "" {
		uid = str(data["user_id"])
	}
	if uid == "" {
		return nil, errf(0, resp, "session invalid or expired")
	}
	return &LoginInfo{
		UserID:        uid,
		SecUserID:     firstStr(data, "sec_uid", "sec_user_id"),
		Username:      firstStr(data, "username", "unique_id"),
		Nickname:      firstStr(data, "nickname", "screen_name"),
		StoreCountry:  str(data["store_country"]),
		Email:         str(data["email"]),
		FollowerCount: numAsInt(data["follower_count"]),
		Raw:           data,
	}, nil
}

// TokenBeat refreshes the session (odin_tt etc.).
func (c *Client) TokenBeat() (map[string]any, error) {
	return c.Get(BaseWebcast+"/passport/token/beat/v2/", &Req{
		Headers: map[string]string{"referer": "https://www.tiktok.com/ucenter_web/live_studio/login"},
	})
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return trimFloat(t) // no scientific notation for ids
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return ""
	}
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return itoa(int64(f))
	}
	return ""
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func numAsInt(v any) int {
	switch t := v.(type) {
	case json.Number:
		return atoiSafe(t.String())
	case float64:
		return int(t)
	case string:
		return atoiSafe(t)
	default:
		return 0
	}
}
