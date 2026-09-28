package tiktok

// Identity is an account's device and locale identity. device_id, install_id
// and web_id belong to the device the session was created on and go in the
// query string of every call, so they're imported together with the cookies.
type Identity struct {
	DeviceID  string
	InstallID string
	WebID     string
	Locale    map[string]string
	Screen    map[string]string
}

// NewIdentity fills in locale/screen defaults; non-empty overrides win.
func NewIdentity(deviceID, installID, webID string, locale, screen map[string]string) Identity {
	l := map[string]string{}
	for k, v := range DefaultLocale {
		l[k] = v
	}
	for k, v := range locale {
		if v != "" {
			l[k] = v
		}
	}
	s := map[string]string{}
	for k, v := range DefaultScreen {
		s[k] = v
	}
	for k, v := range screen {
		if v != "" {
			s[k] = v
		}
	}
	return Identity{DeviceID: deviceID, InstallID: installID, WebID: webID, Locale: l, Screen: s}
}

// CommonParams returns the identity query params sent with almost every
// request. extra is a list of key, value pairs added last (empty values skipped).
func (id Identity) CommonParams(extra ...string) map[string]string {
	p := map[string]string{
		"aid":                 AID,
		"app_name":            AppName,
		"channel":             Channel,
		"device_id":           id.DeviceID,
		"install_id":          id.InstallID,
		"device_platform":     DevicePlatform,
		"version_code":        VersionCode,
		"webcast_sdk_version": WebcastSDKVersion,
		"live_mode":           LiveMode,
		"browser_name":        BrowserName,
		"browser_version":     BrowserVersion,
		"browser_platform":    BrowserPlatform,
	}
	for k, v := range id.Locale {
		p[k] = v
	}
	for k, v := range id.Screen {
		p[k] = v
	}
	if id.WebID != "" {
		p["web_id"] = id.WebID
	}
	for i := 0; i+1 < len(extra); i += 2 {
		if extra[i+1] != "" {
			p[extra[i]] = extra[i+1]
		}
	}
	return p
}
