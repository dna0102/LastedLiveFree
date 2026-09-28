package tiktok

import (
	"encoding/json"
	"strings"
)

type ParsedSession struct {
	Cookies   map[string]string
	DeviceID  string
	InstallID string
	WebID     string
}

// ParseSessionBlob reads cookies from any of the usual export formats and
// picks out device_id/install_id/web_id if present:
//   - a Cookie header: "a=b; c=d"
//   - a JSON object: {"sessionid": "..."}
//   - a Cookie-Editor / EditThisCookie array: [{"name": "...", "value": "..."}]
//   - a Netscape cookies.txt file
func ParseSessionBlob(blob string) ParsedSession {
	s := strings.TrimSpace(blob)
	out := ParsedSession{Cookies: map[string]string{}}
	if s == "" {
		return out
	}

	switch s[0] {
	case '[':
		parseCookieArray(s, &out)
	case '{':
		parseCookieObject(s, &out)
	default:
		if looksNetscape(s) {
			parseNetscape(s, &out)
		} else {
			parseHeader(s, &out)
		}
	}
	extractIdentity(&out)
	return out
}

func setCookie(out *ParsedSession, name, value string) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || value == "" {
		return
	}
	out.Cookies[name] = value
}

func parseCookieArray(s string, out *ParsedSession) {
	var arr []map[string]any
	if json.Unmarshal([]byte(s), &arr) != nil {
		return
	}
	for _, c := range arr {
		name, _ := c["name"].(string)
		val, _ := c["value"].(string)
		setCookie(out, name, val)
	}
}

func parseCookieObject(s string, out *ParsedSession) {
	var obj map[string]any
	if json.Unmarshal([]byte(s), &obj) != nil {
		return
	}
	// {"cookies": {...}} or {"cookies": [...]}
	if inner, ok := obj["cookies"]; ok {
		b, _ := json.Marshal(inner)
		bs := strings.TrimSpace(string(b))
		if strings.HasPrefix(bs, "[") {
			parseCookieArray(bs, out)
		} else if strings.HasPrefix(bs, "{") {
			parseCookieObject(bs, out)
		}
	}
	for k, v := range obj {
		if k == "cookies" {
			continue
		}
		if sv, ok := v.(string); ok {
			setCookie(out, k, sv)
		}
	}
}

func looksNetscape(s string) bool {
	return strings.Contains(s, "\t") &&
		(strings.Contains(s, "TRUE") || strings.Contains(s, "FALSE") || strings.Contains(s, "# Netscape"))
}

func parseNetscape(s string, out *ParsedSession) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) >= 7 {
			setCookie(out, f[5], f[6])
		}
	}
}

func parseHeader(s string, out *ParsedSession) {
	// "a=b; c=d", or one pair per line
	repl := strings.NewReplacer("\r", "", "\n", ";")
	for _, part := range strings.Split(repl.Replace(s), ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.Index(part, "="); i > 0 {
			setCookie(out, part[:i], part[i+1:])
		}
	}
}

// extractIdentity looks for device/install/web ids among the parsed values.
func extractIdentity(out *ParsedSession) {
	for _, k := range []string{"device_id", "deviceId", "tt_device_id"} {
		if v := out.Cookies[k]; v != "" {
			out.DeviceID = v
		}
	}
	for _, k := range []string{"install_id", "iid", "installId"} {
		if v := out.Cookies[k]; v != "" {
			out.InstallID = v
		}
	}
	for _, k := range []string{"web_id", "webId", "ttwid", "tt_webid"} {
		if v := out.Cookies[k]; v != "" && out.WebID == "" {
			out.WebID = v
		}
	}
}
