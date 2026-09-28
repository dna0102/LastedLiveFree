package tiktok

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Request signing.
//
// TikTok LIVE Studio adds these headers to every request:
//
//	X-Khronos  unix seconds
//	X-SS-Stub  MD5 of the body (POST only)
//	X-Ladon    encrypted blob, produced by native code
//	X-Argus    encrypted protobuf, produced by native code
//
// We can only generate the first two. Most endpoints don't check Argus/Ladon,
// but a few (sending chat, kicking/muting viewers, pinning goals) silently
// drop or reject unsigned requests.
const emptyBodyMD5 = "d41d8cd98f00b204e9800998ecf8427e"

// Signer returns signature headers to merge onto an outbound request.
type Signer interface {
	Name() string
	Sign(method, url, query string, body []byte) map[string]string
}

// SSStub is the uppercase MD5 hex of the request body.
func SSStub(body []byte) string {
	if len(body) == 0 {
		return strings.ToUpper(emptyBodyMD5)
	}
	sum := md5.Sum(body)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func Khronos() string { return strconv.FormatInt(time.Now().Unix(), 10) }

// NoopSigner sets X-Khronos and X-SS-Stub only.
type NoopSigner struct{}

func (NoopSigner) Name() string { return "noop" }

func (NoopSigner) Sign(method, url, query string, body []byte) map[string]string {
	h := map[string]string{"x-khronos": Khronos()}
	if strings.EqualFold(method, "POST") {
		h["x-ss-stub"] = SSStub(body)
	}
	return h
}
