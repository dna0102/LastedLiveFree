package tiktok

// A small protobuf wire-format reader, enough to decode the IM feed without
// generated code.

type pbField struct {
	Num   int
	Type  int    // 0 varint, 1 fixed64, 2 bytes, 5 fixed32
	Int   uint64 // varint / fixed value
	Bytes []byte // length-delimited value
}

func pbVarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}

// pbFields splits a message into fields, stopping at the first malformed one.
func pbFields(b []byte) []pbField {
	var out []pbField
	for len(b) > 0 {
		key, n := pbVarint(b)
		if n == 0 {
			break
		}
		b = b[n:]
		f := pbField{Num: int(key >> 3), Type: int(key & 7)}
		switch f.Type {
		case 0:
			v, n := pbVarint(b)
			if n == 0 {
				return out
			}
			f.Int, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return out
			}
			for i := 0; i < 8; i++ {
				f.Int |= uint64(b[i]) << (8 * i)
			}
			b = b[8:]
		case 2:
			l, n := pbVarint(b)
			if n == 0 || uint64(len(b)-n) < l {
				return out
			}
			f.Bytes, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return out
			}
			for i := 0; i < 4; i++ {
				f.Int |= uint64(b[i]) << (8 * i)
			}
			b = b[4:]
		default:
			return out
		}
		out = append(out, f)
	}
	return out
}

// pbMsg is a message's fields by number; repeated fields keep every value.
type pbMsg map[int][]pbField

func pbParse(b []byte) pbMsg {
	m := pbMsg{}
	for _, f := range pbFields(b) {
		m[f.Num] = append(m[f.Num], f)
	}
	return m
}

func (m pbMsg) str(n int) string {
	if v := m[n]; len(v) > 0 {
		return string(v[0].Bytes)
	}
	return ""
}

func (m pbMsg) int(n int) int64 {
	if v := m[n]; len(v) > 0 {
		return int64(v[0].Int)
	}
	return 0
}

func (m pbMsg) msg(n int) pbMsg {
	if v := m[n]; len(v) > 0 {
		return pbParse(v[0].Bytes)
	}
	return pbMsg{}
}

func (m pbMsg) all(n int) []pbMsg {
	var out []pbMsg
	for _, f := range m[n] {
		out = append(out, pbParse(f.Bytes))
	}
	return out
}
