package server

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"lastedlive/internal/tiktok"
)

// Hosts /api/img will fetch from. Anything else is refused, so the route can't
// be used to fetch arbitrary URLs.
var cdnSuffixes = []string{".tiktokcdn.com", ".tiktokcdn-eu.com", ".tiktokcdn-us.com", ".ibyteimg.com", ".byteimg.com", ".tiktokv.com"}

func allowedCDN(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, s := range cdnSuffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	return false
}

var (
	imgSem      = make(chan struct{}, 8) // limits concurrent CDN fetches
	imgInflight sync.Map                 // cache key -> *sync.Mutex, one fetch per image
)

// handleImg serves GET /api/img?u=<url>: a TikTok CDN image, cached on disk.
func (s *Server) handleImg(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	u, err := url.Parse(raw)
	if err != nil || !allowedCDN(u) {
		http.Error(w, "not a TikTok CDN url", http.StatusBadRequest)
		return
	}
	sum := sha1.Sum([]byte(raw))
	key := hex.EncodeToString(sum[:])
	dir := filepath.Join(s.m.Store.Dir(), "cache", "img")
	file := filepath.Join(dir, key)

	serve := func(b []byte, ctype string) {
		if ctype == "" {
			ctype = http.DetectContentType(b)
		}
		w.Header().Set("content-type", ctype)
		w.Header().Set("cache-control", "public, max-age=604800, immutable")
		_, _ = w.Write(b)
	}
	if b, err := os.ReadFile(file); err == nil {
		serve(b, "")
		return
	}

	mu, _ := imgInflight.LoadOrStore(key, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if b, err := os.ReadFile(file); err == nil { // fetched while we waited
		serve(b, "")
		return
	}

	cl := s.assetClient()
	if cl == nil {
		http.Error(w, "image client unavailable", http.StatusServiceUnavailable)
		return
	}
	imgSem <- struct{}{}
	b, ctype, err := cl.FetchAsset(raw)
	<-imgSem
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if strings.HasPrefix(ctype, "image/") || strings.HasPrefix(http.DetectContentType(b), "image/") {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(file, b, 0o644)
	}
	serve(b, ctype)
}

var (
	assetOnce sync.Once
	assetCl   *tiktok.Client
)

// assetClient returns a cookie-less client for CDN downloads. It works before
// any account is added, so gift icons load on the login screen too.
func (s *Server) assetClient() *tiktok.Client {
	assetOnce.Do(func() {
		assetCl, _ = tiktok.NewClient(tiktok.NewIdentity("0", "", "", nil, nil), nil, tiktok.NoopSigner{})
	})
	return assetCl
}
