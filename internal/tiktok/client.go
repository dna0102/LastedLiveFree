package tiktok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// TikTokError is an error returned by TikTok (status_code) or the transport.
type TikTokError struct {
	Message string
	Code    int // status_code / error_code, or 0
	Body    any
}

func (e *TikTokError) Error() string { return e.Message }

func errf(code int, body any, format string, a ...any) *TikTokError {
	return &TikTokError{Message: fmt.Sprintf(format, a...), Code: code, Body: body}
}

// Client makes requests for one account: one TLS fingerprint, one cookie jar.
type Client struct {
	Identity Identity
	signer   Signer

	http tls_client.HttpClient

	mu      sync.Mutex
	cookies map[string]string
	debug   bool

	LastLogID string
}

// ClientProfile is the TLS fingerprint. The desktop app is Electron on Chrome
// 136; Chrome_133 is the closest profile tls-client has.
var ClientProfile = profiles.Chrome_133

func NewClient(id Identity, cookies map[string]string, signer Signer) (*Client, error) {
	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(ClientProfile),
		tls_client.WithNotFollowRedirects(),
	}
	hc, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return nil, err
	}
	ck := make(map[string]string, len(cookies))
	for k, v := range cookies {
		if v != "" {
			ck[k] = v
		}
	}
	return &Client{Identity: id, signer: signer, http: hc, cookies: ck}, nil
}

func (c *Client) CookieSnapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.cookies))
	for k, v := range c.cookies {
		out[k] = v
	}
	return out
}

func (c *Client) cookieHeader() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cookies) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c.cookies))
	for k := range c.cookies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(c.cookies[k])
	}
	return b.String()
}

func (c *Client) setCookie(k, v string) {
	c.mu.Lock()
	c.cookies[k] = v
	c.mu.Unlock()
}

type Req struct {
	Params      map[string]string
	Form        map[string]string // urlencoded body
	JSON        any               // JSON body
	Body        []byte            // raw body; takes precedence over Form/JSON
	Headers     map[string]string
	NoCommon    bool // skip the common identity params
	StoreRegion string
}

type Response struct {
	Status int
	Raw    []byte
	Data   map[string]any
}

func (c *Client) Get(rawURL string, r *Req) (map[string]any, error) {
	return c.do("GET", rawURL, r)
}

func (c *Client) Post(rawURL string, r *Req) (map[string]any, error) {
	return c.do("POST", rawURL, r)
}

func (c *Client) baseHeaders(storeRegion string) http.Header {
	h := http.Header{
		"accept":             {"application/json, text/plain, */*"},
		"user-agent":         {UserAgent},
		"sec-ch-ua":          {SecChUA},
		"sec-ch-ua-platform": {SecChUAPlatform},
		"sdk_aid":            {AID},
		"x-ss-dp":            {""},
		"priority":           {"u=1, i"},
	}
	region := storeRegion
	if region == "" {
		region = c.Identity.Locale["priority_region"]
	}
	if region != "" {
		h.Set("x-tt-store-region", region)
	}
	return h
}

func (c *Client) do(method, rawURL string, r *Req) (map[string]any, error) {
	resp, raw, err := c.doRaw(method, rawURL, r)
	if err != nil {
		return nil, err
	}
	return c.parse(resp, raw)
}

// GetRaw sends a GET and returns the body as-is (for protobuf responses).
func (c *Client) GetRaw(rawURL string, r *Req) ([]byte, int, error) {
	resp, raw, err := c.doRaw("GET", rawURL, r)
	if err != nil {
		return nil, 0, err
	}
	return raw, resp.StatusCode, nil
}

func (c *Client) doRaw(method, rawURL string, r *Req) (*http.Response, []byte, error) {
	if r == nil {
		r = &Req{}
	}
	method = strings.ToUpper(method)

	q := url.Values{}
	if !r.NoCommon {
		for k, v := range c.Identity.CommonParams() {
			q.Set(k, v)
		}
	}
	for k, v := range r.Params {
		q.Set(k, v)
	}

	var body []byte
	headers := c.baseHeaders(r.StoreRegion)
	switch {
	case r.Body != nil:
		body = r.Body
	case r.JSON != nil:
		b, err := json.Marshal(r.JSON)
		if err != nil {
			return nil, nil, err
		}
		body = b
		headers.Set("content-type", "application/json")
	case r.Form != nil:
		form := url.Values{}
		for k, v := range r.Form {
			form.Set(k, v)
		}
		body = []byte(form.Encode())
		if headers.Get("content-type") == "" {
			headers.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
		}
	}
	for k, v := range r.Headers {
		headers.Set(k, v)
	}

	headers.Set("webcast-ntp-t0", fmt.Sprintf("%.4f", float64(time.Now().UnixNano())/1e6))
	for k, v := range c.signer.Sign(method, rawURL, q.Encode(), body) {
		headers.Set(k, v)
	}
	// The whole jar goes to every host. Domain matching the way a browser
	// would doesn't work here: TikTok expects the session cookies on
	// webcast, api and www alike.
	if ck := c.cookieHeader(); ck != "" {
		headers.Set("cookie", ck)
	}
	// Same header order as the desktop app.
	headers[http.HeaderOrderKey] = []string{
		"accept", "content-type", "user-agent", "sec-ch-ua", "sec-ch-ua-platform",
		"sdk_aid", "x-ss-dp", "priority", "x-tt-store-region", "webcast-ntp-t0",
		"x-khronos", "x-ss-stub", "cookie",
	}

	full := rawURL
	if enc := q.Encode(); enc != "" {
		if strings.Contains(rawURL, "?") {
			full = rawURL + "&" + enc
		} else {
			full = rawURL + "?" + enc
		}
	}

	resp, err := c.send(method, full, body, headers)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	c.absorb(resp)
	if lid := resp.Header.Get("x-tt-logid"); lid != "" {
		c.LastLogID = lid
	}
	if c.debug {
		ct := resp.Header.Get("content-type")
		snip := string(raw)
		if len(snip) > 400 {
			snip = snip[:400]
		}
		fmt.Printf("[dbg] %s %s -> %d ct=%q body=%s\n", method, rawURL, resp.StatusCode, ct, snip)
	}
	return resp, raw, nil
}

func (c *Client) SetDebug(v bool) { c.debug = v }

// send retries transient connection failures. They happen before anything
// reaches TikTok, so a retry is safe.
func (c *Client) send(method, full string, body []byte, headers http.Header) (*http.Response, error) {
	const attempts = 3
	backoff := []time.Duration{500 * time.Millisecond, 1200 * time.Millisecond}
	var last error
	for i := 0; i < attempts; i++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, full, rdr)
		if err != nil {
			return nil, err
		}
		req.Header = headers
		resp, err := c.http.Do(req)
		if err == nil {
			return resp, nil
		}
		last = err
		if !transient(err) {
			break
		}
		if i < attempts-1 {
			time.Sleep(backoff[min(i, len(backoff)-1)])
		}
	}
	low := strings.ToLower(last.Error())
	hint := "network error reaching TikTok"
	switch {
	case strings.Contains(low, "connect"):
		hint = "couldn't connect to TikTok — check your internet connection"
	case strings.Contains(low, "timeout") || strings.Contains(low, "deadline"):
		hint = "connection timed out — your network is slow or TikTok is unreachable"
	}
	return nil, errf(0, nil, "%s (%v)", hint, last)
}

func transient(err error) bool {
	low := strings.ToLower(err.Error())
	for _, s := range []string{"connect", "timeout", "deadline", "reset", "eof", "refused", "no such host", "tls"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// absorb picks up rotated cookies (msToken via x-ms-token, the rest via Set-Cookie).
func (c *Client) absorb(resp *http.Response) {
	if t := resp.Header.Get("x-ms-token"); t != "" {
		c.setCookie("msToken", t)
	}
	for _, sc := range resp.Header["Set-Cookie"] {
		if i := strings.Index(sc, "="); i > 0 {
			name := sc[:i]
			rest := sc[i+1:]
			if j := strings.Index(rest, ";"); j >= 0 {
				rest = rest[:j]
			}
			if name != "" && rest != "" {
				c.setCookie(name, rest)
			}
		}
	}
}

// parse decodes the JSON envelope and turns a non-zero status_code into an error.
func (c *Client) parse(resp *http.Response, raw []byte) (map[string]any, error) {
	ct := resp.Header.Get("content-type")
	if resp.StatusCode >= 400 && !strings.Contains(ct, "json") {
		// e.g. goal/pin returns a bare 403 when unsigned
		return nil, errf(resp.StatusCode, nil, "TikTok refused the request (HTTP %d) — this action needs TikTok's native signature", resp.StatusCode)
	}
	if !strings.Contains(ct, "application/json") && !strings.Contains(ct, "text/plain") {
		// protobuf or other binary body
		return map[string]any{"_raw": raw, "_status": resp.StatusCode}, nil
	}
	// An empty 200 with a JSON content type is how TikTok drops an unsigned
	// request to an Argus-checked endpoint such as room/chat.
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errf(-1, nil, "empty response — endpoint likely requires the native signer (Argus/Ladon)")
	}
	// UseNumber: room/user/stream ids are 19 digits and don't survive float64.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data map[string]any
	if err := dec.Decode(&data); err != nil {
		snippet := string(raw)
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, errf(0, snippet, "non-JSON response (%d)", resp.StatusCode)
	}
	status := firstNum(data, "status_code", "error_code")
	if status != nil && *status != 0 {
		msg := nestedMessage(data)
		if msg == "" {
			if m, ok := data["message"].(string); ok {
				msg = m
			}
		}
		if msg == "" {
			msg = fmt.Sprintf("status_code=%d", *status)
		}
		return nil, errf(*status, data, "%s", msg)
	}
	if biz := resp.Header.Get("bd-tt-error-code"); biz != "" && biz != "0" {
		return nil, errf(atoiSafe(biz), data, "bd-tt-error-code=%s", biz)
	}
	return data, nil
}

func firstNum(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case json.Number:
				return intPtr(atoiSafe(n.String()))
			case float64:
				i := int(n)
				return &i
			case string:
				if n == "" {
					continue
				}
				return intPtr(atoiSafe(n))
			}
		}
	}
	return nil
}

func nestedMessage(data map[string]any) string {
	if d, ok := data["data"].(map[string]any); ok {
		if m, ok := d["message"].(string); ok {
			return m
		}
	}
	return ""
}

func intPtr(i int) *int { return &i }

func atoiSafe(s string) int {
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

// FetchAsset downloads a public CDN file (gift icon, avatar). No cookies or
// signing are needed.
func (c *Client) FetchAsset(rawURL string) ([]byte, string, error) {
	h := http.Header{
		"accept":            {"image/avif,image/webp,image/apng,image/*,*/*;q=0.8"},
		"accept-language":   {"en-US,en;q=0.9"},
		"user-agent":        {UserAgent},
		http.HeaderOrderKey: {"accept", "accept-language", "user-agent"},
	}
	resp, err := c.send("GET", rawURL, nil, h)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("cdn returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	return b, resp.Header.Get("content-type"), nil
}
