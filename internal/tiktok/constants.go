// Package tiktok is the per-account HTTP client and the subset of the TikTok
// LIVE Studio API that Lasted Live uses. The identity values below match
// TikTok LIVE Studio 1.36.6 on Windows.
package tiktok

const (
	AID                = "8311"
	AppName            = "tiktok_live_studio"
	Channel            = "studio"
	VersionCode        = "1.36.6"
	WebcastSDKVersion  = "1366"
	LiveID             = "12"
	LiveMode           = "6" // PC studio room
	AppLanguage        = "en"
	DevicePlatform     = "windows"
	BrowserPlatform    = "Win32"
	BrowserName        = "Mozilla"
	PrimaryAuthCookie  = "sessionid"
	WebcastSDKVersionN = "1366"
)

// The desktop app's Electron user agent.
const (
	BrowserVersion = "5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) " +
		"TikTokLIVEStudio/1.36.6 Chrome/136.0.7103.59 Electron/36.4.0-alpha.63 " +
		"TTElectron/36.4.0-alpha.63 Safari/537.36"

	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) " +
		"TikTokLIVEStudio/1.36.6 Chrome/136.0.7103.59 Electron/36.4.0-alpha.63 " +
		"TTElectron/36.4.0-alpha.63 Safari/537.36"

	SecChUA         = `"Not.A/Brand";v="99", "Chromium";v="136"`
	SecChUAPlatform = `"Windows"`
)

// Hosts for the EU (no1a / eu-ttp2) cluster.
const (
	HostWebcast = "webcast16-normal-no1a.tiktokv.eu"
	HostAPI     = "api16-normal-no1a.tiktokv.eu"
	HostWWW     = "www.tiktok.com"
	HostHotAPI  = "hotapi16-normal-no1a.tiktokv.eu"
	HostWS      = "webcast-ws16-normal-no1a.tiktokv.eu"

	BaseWebcast = "https://" + HostWebcast
	BaseAPI     = "https://" + HostAPI
	BaseWWW     = "https://" + HostWWW
	BaseHotAPI  = "https://" + HostHotAPI
)

// SessionCookieNames are the cookies that make up a logged-in session.
var SessionCookieNames = []string{
	"sessionid", "sessionid_ss", "sid_tt", "sid_guard",
	"uid_tt", "uid_tt_ss", "sid_ucp_v1", "ssid_ucp_v1",
	"tt_session_tlb_tag", "store-idc", "store-country-code",
	"store-country-code-src", "store-country-sign", "tt-target-idc",
	"tt-target-idc-sign", "msToken", "odin_tt",
}

// Per-account locale and screen defaults; accounts can override them.
var (
	DefaultLocale = map[string]string{
		"timezone_name":    "Europe/London",
		"browser_language": "en-GB",
		"language":         "en",
		"app_language":     "en",
		"webcast_language": "en",
		"priority_region":  "gb",
	}
	DefaultScreen = map[string]string{
		"screen_width":  "1920",
		"screen_height": "1080",
	}
)
