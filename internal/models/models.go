// Package models defines the saved account. The JSON layout is the same as the
// old Python tool's accounts.json, so that file can be used as-is.
package models

type Account struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	DeviceID  string            `json:"device_id"`
	InstallID string            `json:"install_id"`
	WebID     string            `json:"web_id,omitempty"`
	Cookies   map[string]string `json:"cookies"`
	Locale    map[string]string `json:"locale,omitempty"`
	Screen    map[string]string `json:"screen,omitempty"`

	DefaultGoalType   *int `json:"default_goal_type,omitempty"`
	DefaultGoalTarget *int `json:"default_goal_target,omitempty"`

	// Profile details, refreshed on login and on "Refresh profile".
	UserID         string `json:"user_id,omitempty"`
	SecUserID      string `json:"sec_user_id,omitempty"`
	Username       string `json:"username,omitempty"`
	Nickname       string `json:"nickname,omitempty"`
	DisplayID      string `json:"display_id,omitempty"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	Bio            string `json:"bio,omitempty"`
	FollowerCount  int    `json:"follower_count,omitempty"`
	FollowingCount int    `json:"following_count,omitempty"`
	TotalLikes     int    `json:"total_likes,omitempty"`
	Verified       bool   `json:"verified,omitempty"`
}

// Public is what the UI gets: everything but the cookies.
func (a *Account) Public() map[string]any {
	return map[string]any{
		"id":              a.ID,
		"label":           firstNonEmpty(a.Label, a.Username, a.ID),
		"device_id":       a.DeviceID,
		"has_cookies":     a.Cookies["sessionid"] != "",
		"user_id":         a.UserID,
		"username":        firstNonEmpty(a.Username, a.DisplayID),
		"nickname":        a.Nickname,
		"display_id":      a.DisplayID,
		"avatar_url":      a.AvatarURL,
		"follower_count":  a.FollowerCount,
		"following_count": a.FollowingCount,
		"total_likes":     a.TotalLikes,
		"bio":             a.Bio,
		"verified":        a.Verified,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
