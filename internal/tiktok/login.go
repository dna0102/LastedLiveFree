package tiktok

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// QR login against the passport API:
//
//	GET /passport/mobile/get_qrcode/      -> qrcode (base64 PNG), token, expire_time
//	GET /passport/mobile/check_qrconnect/ -> new, scanned, then confirmed or expired
//
// The "confirmed" response sets the full session (sessionid, sid_tt,
// sid_guard, ...) as cookies.

const basePassport = "https://api16-normal-no1a.tiktokv.eu"

// QRLogin is a QR login in progress.
type QRLogin struct {
	c        *Client
	DeviceID string
	verifyFP string
	Token    string
}

type QRStart struct {
	Token      string `json:"token"`
	PNGBase64  string `json:"qrcode_png_b64"`
	IndexURL   string `json:"qrcode_index_url"`
	ExpireTime int    `json:"expire_time"`
}

type QRPoll struct {
	Status   string            `json:"status"` // pending | scanned | expired | ok | error
	Detail   string            `json:"detail"`
	Cookies  map[string]string `json:"-"`
	UserID   string            `json:"user_id"`
	SecUID   string            `json:"sec_user_id"`
	Username string            `json:"username"`
	Nickname string            `json:"nickname"`
}

// NewQRLogin starts a login session. A device id is generated if deviceID is empty.
func NewQRLogin(deviceID string) (*QRLogin, error) {
	if deviceID == "" {
		deviceID = randDigits19()
	}
	id := NewIdentity(deviceID, "", "", nil, nil)
	cl, err := NewClient(id, map[string]string{"msToken": randToken(128)}, NoopSigner{})
	if err != nil {
		return nil, err
	}
	return &QRLogin{c: cl, DeviceID: deviceID, verifyFP: verifyFP()}, nil
}

func (q *QRLogin) query(extra map[string]string) map[string]string {
	p := map[string]string{
		"language": "en", "app_language": "en", "webcast_language": "en",
		"channel": Channel, "device_type": "PC", "os_version": "10.0.26200",
		"device_platform": DevicePlatform, "version_code": VersionCode,
		"webcast_sdk_version": WebcastSDKVersion, "aid": AID,
		"account_sdk_source": "web", "sdk_version": "2.1.9",
		"device_id": q.DeviceID, "verifyFp": q.verifyFP,
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

var passportHeaders = map[string]string{"referer": "https://www.tiktok.com/"}

func (q *QRLogin) Start() (*QRStart, error) {
	resp, err := q.c.Get(basePassport+"/passport/mobile/get_qrcode/", &Req{
		Params: q.query(map[string]string{"service": "https://www.tiktok.com"}), NoCommon: true, Headers: passportHeaders,
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	if d == nil || str(d["qrcode"]) == "" {
		return nil, errf(0, resp, "get_qrcode returned no QR (TikTok may require a web signature)")
	}
	q.Token = str(d["token"])
	return &QRStart{Token: q.Token, PNGBase64: str(d["qrcode"]), IndexURL: str(d["qrcode_index_url"]), ExpireTime: numAsInt(d["expire_time"])}, nil
}

func (q *QRLogin) Poll() QRPoll {
	if q.Token == "" {
		return QRPoll{Status: "error", Detail: "start first"}
	}
	resp, err := q.c.Get(basePassport+"/passport/mobile/check_qrconnect/", &Req{
		Params: q.query(map[string]string{"token": q.Token, "service": "https://www.tiktok.com"}), NoCommon: true, Headers: passportHeaders,
	})
	if err != nil {
		return QRPoll{Status: "error", Detail: err.Error()}
	}
	d, _ := resp["data"].(map[string]any)
	switch str(d["status"]) {
	case "new":
		return QRPoll{Status: "pending", Detail: "waiting for scan"}
	case "scanned":
		return QRPoll{Status: "scanned", Detail: "scanned — confirm on your phone"}
	case "expired":
		return QRPoll{Status: "expired", Detail: "QR expired"}
	case "confirmed":
		if ru := str(d["redirect_url"]); ru != "" && q.c.CookieSnapshot()["sessionid"] == "" {
			_, _ = q.c.Get(ru, &Req{NoCommon: true, Headers: passportHeaders})
		}
		ck := q.c.CookieSnapshot()
		ud, _ := d["user_data"].(map[string]any)
		p := QRPoll{Status: "ok", Detail: "logged in", Cookies: ck}
		if ud != nil {
			p.UserID = firstStr(ud, "user_id_str", "user_id")
			p.SecUID = str(ud["sec_user_id"])
			p.Username = firstStr(ud, "username", "screen_name")
			p.Nickname = str(ud["name"])
		}
		if ck["sessionid"] == "" {
			return QRPoll{Status: "error", Detail: "confirmed but no session cookie was issued"}
		}
		return p
	}
	return QRPoll{Status: "error", Detail: "unexpected status: " + str(d["status"])}
}

func randDigits19() string {
	var b strings.Builder
	b.WriteByte('7')
	for i := 0; i < 18; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String()
}

func randToken(n int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	var b strings.Builder
	for i := 0; i < n; i++ {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alpha))))
		b.WriteByte(alpha[k.Int64()])
	}
	return b.String()
}

// verifyFP generates the verifyFp value the web client sends:
// verify_<base36 ms>_<random blocks>. It isn't validated as a signature.
func verifyFP() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	blk := func(k int) string {
		var b strings.Builder
		for i := 0; i < k; i++ {
			n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
			b.WriteByte(chars[n.Int64()])
		}
		return b.String()
	}
	return "verify_" + strconv.FormatInt(time.Now().UnixMilli(), 36) + "_" + blk(4) + "_" + blk(4) + "_" + blk(4) + "_" + blk(12)
}
