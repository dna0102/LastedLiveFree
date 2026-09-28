package tiktok

import (
	"fmt"
	"strconv"
)

// IMBatch is one fetch of a room's message feed (comments, gifts, likes, joins).
type IMBatch struct {
	Events      []IMEvent
	Cursor      string
	InternalExt string
	IntervalMs  int64
	Raw         []IMRaw // every message, including types we don't decode
}

type IMRaw struct {
	Method  string
	Payload []byte
}

// FetchIM fetches the room's message feed over HTTP (the websocket's polling
// fallback). Pass the previous batch's Cursor and InternalExt to get only new
// messages.
func (c *Client) FetchIM(roomID, cursor, internalExt string) (*IMBatch, error) {
	raw, status, err := c.GetRaw(BaseWebcast+"/webcast/im/fetch/", &Req{Params: map[string]string{
		"room_id": roomID, "cursor": cursor, "internal_ext": internalExt,
		"resp_content_type": "protobuf", "identity": "anchor", "did_rule": "0", "fetch_rule": "1",
		"last_rtt": "0", "live_id": "12", "history_comment_cursor": "", "sup_ws_ds_opt": "1",
		"debug": "false", "host": "https://webcast.tiktokv.com", "version_code": "300400",
		"browser_online": "true", "cookie_enabled": "true", "force_https": "false",
	}})
	if err != nil {
		return nil, err
	}
	if status != 200 || len(raw) == 0 {
		return nil, fmt.Errorf("im/fetch: http %d, %d bytes", status, len(raw))
	}
	top := pbParse(raw)
	b := &IMBatch{Cursor: top.str(2), InternalExt: top.str(5), IntervalMs: top.int(3)}
	for _, m := range top.all(1) {
		b.Raw = append(b.Raw, IMRaw{Method: m.str(1), Payload: m.bytes(2)})
	}
	if len(b.Raw) == 0 && b.Cursor == "" {
		return nil, fmt.Errorf("im/fetch: unrecognised response (%d bytes)", len(raw))
	}
	for _, r := range b.Raw {
		if ev, ok := decodeIM(r.Method, r.Payload); ok {
			b.Events = append(b.Events, ev)
		}
	}
	return b, nil
}

func (m pbMsg) bytes(n int) []byte {
	if v := m[n]; len(v) > 0 {
		return v[0].Bytes
	}
	return nil
}

type IMEvent struct {
	Kind    string `json:"kind"` // chat | gift | like | join | follow | share | viewers | system
	MsgID   string `json:"id"`
	Time    int64  `json:"t"` // unix ms
	User    IMUser `json:"user"`
	Text    string `json:"text,omitempty"`
	GiftID  int64  `json:"gift_id,omitempty"`
	Gift    string `json:"gift,omitempty"`
	Icon    string `json:"icon,omitempty"`
	Count   int64  `json:"count,omitempty"`    // gift repeats, or likes in this batch
	Coins   int64  `json:"coins,omitempty"`    // price of one gift
	Total   int64  `json:"total,omitempty"`    // like total, or viewers watching now
	AllTime int64  `json:"all_time,omitempty"` // viewers: everyone who has watched
	Pending bool   `json:"pending,omitempty"`  // gift streak still going
}

type IMUser struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Handle   string `json:"handle"`
	Avatar   string `json:"avatar"`
}

func imUser(m pbMsg) IMUser {
	u := IMUser{Nickname: m.str(3), Handle: m.str(38)}
	if id := m.int(1); id != 0 {
		u.ID = strconv.FormatUint(uint64(id), 10)
	}
	if av := m.msg(9); len(av) > 0 {
		if l := av[1]; len(l) > 0 {
			u.Avatar = string(l[0].Bytes)
		}
	}
	return u
}

// imCommon reads the header every webcast message has in field 1
// (msg_id = 2, create_time = 4).
func imCommon(p pbMsg, ev *IMEvent) {
	c := p.msg(1)
	if id := c.int(2); id != 0 {
		ev.MsgID = strconv.FormatUint(uint64(id), 10)
	}
	ev.Time = c.int(4)
}

func decodeIM(method string, payload []byte) (IMEvent, bool) {
	p := pbParse(payload)
	ev := IMEvent{}
	imCommon(p, &ev)
	switch method {
	case "WebcastChatMessage":
		ev.Kind, ev.User, ev.Text = "chat", imUser(p.msg(2)), p.str(3)
	case "WebcastGiftMessage":
		ev.Kind, ev.User = "gift", imUser(p.msg(7))
		ev.GiftID, ev.Count = p.int(2), p.int(5)
		g := p.msg(15)
		ev.Gift, ev.Coins = g.str(16), g.int(12)
		if img := g.msg(1); len(img) > 0 {
			if l := img[1]; len(l) > 0 {
				ev.Icon = string(l[0].Bytes)
			}
		}
		ev.Pending = g.int(11) == 1 && p.int(9) == 0 // streakable gift, streak not over
	case "WebcastLikeMessage":
		ev.Kind, ev.User, ev.Count, ev.Total = "like", imUser(p.msg(5)), p.int(2), p.int(3)
	case "WebcastMemberMessage":
		ev.Kind, ev.User, ev.Total = "join", imUser(p.msg(2)), p.int(3)
	case "WebcastSocialMessage":
		ev.User = imUser(p.msg(2))
		switch p.int(4) {
		case 1:
			ev.Kind = "follow"
		case 3:
			ev.Kind = "share"
		default:
			return ev, false
		}
	case "WebcastRoomUserSeqMessage":
		// 3 = watching now, 7 = everyone so far
		ev.Kind, ev.Total, ev.AllTime = "viewers", p.int(3), p.int(7)
	default:
		return ev, false
	}
	return ev, true
}
