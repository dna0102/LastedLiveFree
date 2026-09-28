// Package manager ties the account, its TikTok client and the encoder together:
// going LIVE, streaming, ending, and the background checks.
//
// Whether an account is LIVE is tracked here, not asked from TikTok:
// realtime_stats.is_live keeps saying true for minutes after a room ends.
package manager

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"lastedlive/internal/encoder"
	"lastedlive/internal/models"
	"lastedlive/internal/store"
	"lastedlive/internal/tiktok"
)

type LiveSession struct {
	AccountID string    `json:"account_id"`
	RoomID    string    `json:"room_id"`
	StreamID  string    `json:"stream_id"`
	RTMP      string    `json:"-"`
	ShareURL  string    `json:"share_url"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"started_at"`
	GoalID    string    `json:"-"` // goal we created for this room; see GetRoomGoal
	stopHB    chan struct{}
	chat      *chatFeed
}

// View is the session as sent to the UI (no RTMP key).
func (s *LiveSession) View() map[string]any {
	return map[string]any{
		"account_id": s.AccountID,
		"room_id":    s.RoomID,
		"stream_id":  s.StreamID,
		"title":      s.Title,
		"share_url":  s.ShareURL,
		"started_at": s.StartedAt.Unix(),
	}
}

type Manager struct {
	Store  *store.Store
	signer tiktok.Signer

	ffMu   sync.RWMutex
	ffmpeg string // empty until ffmpeg is found or installed

	mu         sync.RWMutex
	clients    map[string]*tiktok.Client
	sessions   map[string]*LiveSession
	encoders   map[string]*encoder.Encoder
	encRetries map[string]int
	previews   map[string]*encoder.PreviewStreamer
}

func New(st *store.Store, ffmpeg string) *Manager {
	return &Manager{
		Store:      st,
		signer:     tiktok.NoopSigner{},
		ffmpeg:     ffmpeg,
		clients:    map[string]*tiktok.Client{},
		sessions:   map[string]*LiveSession{},
		encoders:   map[string]*encoder.Encoder{},
		encRetries: map[string]int{},
		previews:   map[string]*encoder.PreviewStreamer{},
	}
}

// ClientFor returns the account's client, creating it on first use. Cookies
// rotated by TikTok are saved on Validate and EndLive.
func (m *Manager) ClientFor(a *models.Account) (*tiktok.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cl, ok := m.clients[a.ID]; ok {
		return cl, nil
	}
	id := tiktok.NewIdentity(a.DeviceID, a.InstallID, a.WebID, a.Locale, a.Screen)
	cl, err := tiktok.NewClient(id, a.Cookies, m.signer)
	if err != nil {
		return nil, err
	}
	m.clients[a.ID] = cl
	return cl, nil
}

func (m *Manager) dropClient(id string) {
	m.mu.Lock()
	delete(m.clients, id)
	m.mu.Unlock()
}

func (m *Manager) persistCookies(a *models.Account, cl *tiktok.Client) {
	snap := cl.CookieSnapshot()
	if len(snap) == 0 {
		return
	}
	for k, v := range snap {
		a.Cookies[k] = v
	}
	_ = m.Store.Upsert(a)
}

// Validate checks the session and refreshes the saved profile.
func (m *Manager) Validate(a *models.Account) (map[string]any, error) {
	cl, err := m.ClientFor(a)
	if err != nil {
		return nil, err
	}
	info, err := cl.CheckLogin()
	if err != nil {
		return nil, err
	}
	a.UserID = orStr(info.UserID, a.UserID)
	a.Username = orStr(info.Username, a.Username)
	a.Nickname = orStr(info.Nickname, a.Nickname)
	out := map[string]any{"user_id": info.UserID, "username": info.Username, "nickname": info.Nickname}
	if prof, perr := cl.GetFullProfile(a.UserID); perr == nil {
		a.SecUserID = orStr(prof.SecUserID, a.SecUserID)
		a.DisplayID = orStr(prof.DisplayID, a.DisplayID)
		a.Nickname = orStr(prof.Nickname, a.Nickname)
		a.AvatarURL = orStr(prof.AvatarURL, a.AvatarURL)
		if prof.Bio != "" {
			a.Bio = prof.Bio
		}
		a.FollowerCount = prof.FollowerCount
		a.FollowingCount = prof.FollowingCount
		a.TotalLikes = prof.TotalLikes
		a.Verified = prof.Verified
		out["profile"] = map[string]any{
			"display_id": prof.DisplayID, "avatar_url": prof.AvatarURL,
			"follower_count": prof.FollowerCount, "following_count": prof.FollowingCount,
			"total_likes": prof.TotalLikes, "verified": prof.Verified,
		}
	}
	m.persistCookies(a, cl)
	return out, nil
}

func (m *Manager) GoLive(a *models.Account, title, hashtagID, gameTagID string) (*LiveSession, error) {
	m.mu.RLock()
	_, live := m.sessions[a.ID]
	m.mu.RUnlock()
	if live {
		return nil, fmt.Errorf("%s is already live", a.ID)
	}
	cl, err := m.ClientFor(a)
	if err != nil {
		return nil, err
	}
	perm := cl.CheckGoLivePermission()
	if v, ok := perm["enable_live_studio"].(bool); ok && !v {
		_, _ = cl.ApplyStudioPermission()
	}
	if title == "" {
		title = "Let's go LIVE!"
	}
	if hashtagID == "" {
		hashtagID = "5"
	}
	urls, err := cl.CreateRoom(tiktok.RoomConfig{Title: title, HashtagID: hashtagID, GameTagID: gameTagID})
	if err != nil {
		return nil, err
	}
	cl.PingAnchor(urls.RoomID, urls.StreamID, tiktok.PingCreated)

	s := &LiveSession{
		AccountID: a.ID, RoomID: urls.RoomID, StreamID: urls.StreamID,
		RTMP: urls.RTMPPushURL, ShareURL: urls.ShareURL, Title: title,
		StartedAt: time.Now(), stopHB: make(chan struct{}),
		chat: &chatFeed{seen: map[string]bool{}},
	}
	go m.heartbeat(cl, s)
	go m.pollChat(cl, s)

	m.mu.Lock()
	m.sessions[a.ID] = s
	m.mu.Unlock()
	m.persistCookies(a, cl)
	return s, nil
}

func (m *Manager) heartbeat(cl *tiktok.Client, s *LiveSession) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	// ping right away so TikTok picks up the stream quickly
	cl.PingAnchor(s.RoomID, s.StreamID, tiktok.PingLive)
	for {
		select {
		case <-s.stopHB:
			return
		case <-t.C:
			cl.PingAnchor(s.RoomID, s.StreamID, tiktok.PingLive)
		}
	}
}

// EndLive stops the encoder, closes the room and forgets the session.
func (m *Manager) EndLive(accountID string) (map[string]any, error) {
	m.mu.Lock()
	s := m.sessions[accountID]
	delete(m.sessions, accountID)
	enc := m.encoders[accountID]
	delete(m.encRetries, accountID)
	m.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("%s is not live", accountID)
	}
	if enc != nil {
		enc.Stop()
	}
	if s.stopHB != nil {
		close(s.stopHB)
	}
	a := m.Store.Get(accountID)
	if a == nil {
		return map[string]any{"ok": true, "note": "session cleared"}, nil
	}
	cl, err := m.ClientFor(a)
	if err != nil {
		return map[string]any{"ok": true, "note": "session cleared; " + err.Error()}, nil
	}
	res := cl.EndLive(s.RoomID, s.StreamID)
	m.persistCookies(a, cl)
	return map[string]any{"ok": true, "result_keys": len(res)}, nil
}

func (m *Manager) Session(accountID string) *LiveSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[accountID]
}

// IsLive reports whether we have the account LIVE (see the package doc).
func (m *Manager) IsLive(accountID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[accountID]
	return ok
}

func (m *Manager) Sessions() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []map[string]any{}
	for _, s := range m.sessions {
		out = append(out, s.View())
	}
	return out
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (m *Manager) logPathFor(accountID string) string {
	name := fmt.Sprintf("%s_encoder_%s.log", accountID, time.Now().Format("20060102-150405"))
	return filepath.Join(m.Store.Dir(), "logs", name)
}
