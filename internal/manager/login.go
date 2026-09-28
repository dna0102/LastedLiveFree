package manager

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"lastedlive/internal/models"
	"lastedlive/internal/tiktok"
	"lastedlive/internal/webutil"
)

// ErrAccountLimit is returned when logging in while an account already exists.
// The free version runs one account at a time.
var ErrAccountLimit = errors.New("the free version supports one account — log out of the current one first")

type qrEntry struct {
	q       *tiktok.QRLogin
	label   string
	created time.Time
}

var (
	qrMu    sync.Mutex
	qrLogin = map[string]*qrEntry{}
)

func (m *Manager) hasAccount() bool { return len(m.Store.All()) > 0 }

func (m *Manager) QRStart(label string) (string, *tiktok.QRStart, error) {
	if m.hasAccount() {
		return "", nil, ErrAccountLimit
	}
	q, err := tiktok.NewQRLogin("")
	if err != nil {
		return "", nil, err
	}
	st, err := q.Start()
	if err != nil {
		return "", nil, err
	}
	id := webutil.ShortID() + webutil.ShortID()
	qrMu.Lock()
	for k, e := range qrLogin { // forget old attempts
		if time.Since(e.created) > 10*time.Minute {
			delete(qrLogin, k)
		}
	}
	qrLogin[id] = &qrEntry{q: q, label: strings.TrimSpace(label), created: time.Now()}
	qrMu.Unlock()
	return id, st, nil
}

// QRPoll checks a QR login. Once it's confirmed, the account is saved and its
// profile loaded.
func (m *Manager) QRPoll(id string) map[string]any {
	qrMu.Lock()
	e := qrLogin[id]
	qrMu.Unlock()
	if e == nil {
		return map[string]any{"status": "error", "detail": "login expired — refresh the QR code"}
	}
	p := e.q.Poll()
	if p.Status != "ok" {
		return map[string]any{"status": p.Status, "detail": p.Detail}
	}
	qrMu.Lock()
	delete(qrLogin, id)
	qrMu.Unlock()

	if m.hasAccount() {
		return map[string]any{"status": "error", "detail": ErrAccountLimit.Error()}
	}
	label := e.label
	if label == "" {
		label = firstNonEmpty(p.Username, p.Nickname, "account")
	}
	a := &models.Account{
		ID: m.uniqueID(label), Label: label, DeviceID: e.q.DeviceID, Cookies: p.Cookies,
		UserID: p.UserID, SecUserID: p.SecUID, Username: p.Username, Nickname: p.Nickname,
	}
	if err := m.Store.Upsert(a); err != nil {
		return map[string]any{"status": "error", "detail": err.Error()}
	}
	_, _ = m.Validate(a)
	return map[string]any{"status": "ok", "account": a.Public()}
}

func (m *Manager) ImportSession(label, blob, deviceID string) (*models.Account, error) {
	if m.hasAccount() {
		return nil, ErrAccountLimit
	}
	parsed := tiktok.ParseSessionBlob(blob)
	if parsed.Cookies["sessionid"] == "" {
		return nil, fmt.Errorf("no sessionid found — paste the cookies from a logged-in TikTok session")
	}
	if deviceID == "" {
		deviceID = parsed.DeviceID
	}
	if deviceID == "" {
		deviceID = webutil.RandomDeviceID()
	}
	label = strings.TrimSpace(label)
	a := &models.Account{
		ID: m.uniqueID(firstNonEmpty(label, "account")), Label: label, DeviceID: deviceID,
		InstallID: parsed.InstallID, WebID: parsed.WebID, Cookies: parsed.Cookies,
	}
	if err := m.Store.Upsert(a); err != nil {
		return nil, err
	}
	if _, err := m.Validate(a); err != nil {
		_, _ = m.Store.Delete(a.ID)
		m.dropClient(a.ID)
		return nil, fmt.Errorf("session rejected by TikTok: %w", err)
	}
	if a.Label == "" {
		a.Label = firstNonEmpty(a.Nickname, a.Username)
		_ = m.Store.Upsert(a)
	}
	return a, nil
}

func (m *Manager) RemoveAccount(id string) error {
	if m.IsLive(id) {
		_, _ = m.EndLive(id)
	}
	m.StopPreview(id)
	m.dropClient(id)
	_, err := m.Store.Delete(id)
	return err
}

func (m *Manager) uniqueID(label string) string {
	id := webutil.Slugify(label)
	if id == "" {
		id = "account"
	}
	if m.Store.Get(id) == nil {
		return id
	}
	return id + "-" + webutil.ShortID()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
