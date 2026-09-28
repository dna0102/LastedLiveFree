// Package webutil has small id helpers.
package webutil

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases s and replaces anything but letters and digits with dashes.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func ShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RandomDeviceID makes up a 19-digit device id for sessions imported without one.
func RandomDeviceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var n uint64
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	// real ids are 19 digits starting with 7
	s := "7"
	digits := []byte("0123456789")
	for i := 0; i < 18; i++ {
		s += string(digits[n%10])
		n /= 10
		if n == 0 {
			n = uint64(b[i%8]) + 1
		}
	}
	return s
}
