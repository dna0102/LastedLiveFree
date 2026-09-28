package manager

import (
	"sync"
	"time"

	"lastedlive/internal/tiktok"
)

// chatFeed holds recent messages from a room's feed (comments, gifts, likes,
// joins) for the UI to read with ?since=<seq>.
type chatFeed struct {
	mu     sync.Mutex
	seq    uint64
	items  []ChatItem
	seen   map[string]bool
	errMsg string

	viewers   int64 // watching now, from the latest RoomUserSeq message
	viewersAt time.Time
}

type ChatItem struct {
	Seq uint64 `json:"seq"`
	tiktok.IMEvent
}

const chatKeep = 500

func (f *chatFeed) add(evs []tiktok.IMEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ev := range evs {
		if ev.Kind == "viewers" {
			f.viewers, f.viewersAt = ev.Total, time.Now()
		}
		if ev.Kind == "gift" && ev.Pending {
			continue // only show a gift streak once it ends
		}
		if ev.MsgID != "" {
			if f.seen[ev.MsgID] {
				continue
			}
			f.seen[ev.MsgID] = true
		}
		f.seq++
		f.items = append(f.items, ChatItem{Seq: f.seq, IMEvent: ev})
	}
	if len(f.items) > chatKeep {
		f.items = append([]ChatItem(nil), f.items[len(f.items)-chatKeep:]...)
	}
	if len(f.seen) > 4*chatKeep {
		f.seen = map[string]bool{}
		for _, it := range f.items {
			f.seen[it.MsgID] = true
		}
	}
}

func (f *chatFeed) since(seq uint64) ([]ChatItem, uint64, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ChatItem
	for _, it := range f.items {
		if it.Seq > seq {
			out = append(out, it)
		}
	}
	return out, f.seq, f.errMsg
}

func (m *Manager) pollChat(cl *tiktok.Client, s *LiveSession) {
	cursor, ext := "", ""
	wait := 2 * time.Second
	for {
		select {
		case <-s.stopHB:
			return
		case <-time.After(wait):
		}
		b, err := cl.FetchIM(s.RoomID, cursor, ext)
		if err != nil {
			s.chat.mu.Lock()
			s.chat.errMsg = err.Error()
			s.chat.mu.Unlock()
			wait = 6 * time.Second
			continue
		}
		s.chat.mu.Lock()
		s.chat.errMsg = ""
		s.chat.mu.Unlock()
		cursor, ext = b.Cursor, b.InternalExt
		s.chat.add(b.Events)
		wait = time.Duration(b.IntervalMs) * time.Millisecond
		if wait < 1500*time.Millisecond {
			wait = 1500 * time.Millisecond
		}
	}
}

func (m *Manager) ChatSince(accountID string, seq uint64) ([]ChatItem, uint64, string, bool) {
	s := m.Session(accountID)
	if s == nil || s.chat == nil {
		return nil, 0, "", false
	}
	items, last, errMsg := s.chat.since(seq)
	return items, last, errMsg, true
}

// CurrentViewers returns how many people are watching, from the room's feed.
// ok is false until TikTok has sent a count, or if the last one is over two
// minutes old.
func (m *Manager) CurrentViewers(accountID string) (int64, bool) {
	s := m.Session(accountID)
	if s == nil || s.chat == nil {
		return 0, false
	}
	s.chat.mu.Lock()
	defer s.chat.mu.Unlock()
	if s.chat.viewersAt.IsZero() || time.Since(s.chat.viewersAt) > 2*time.Minute {
		return 0, false
	}
	return s.chat.viewers, true
}
