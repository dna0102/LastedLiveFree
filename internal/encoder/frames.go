package encoder

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

type frameBuf struct {
	mu  sync.Mutex
	jpg []byte
	seq uint64
}

// frameSeq is shared by every preview so sequence numbers keep increasing when
// one preview process replaces another, or the live push takes over.
var frameSeq atomic.Uint64

func (f *frameBuf) set(b []byte) {
	f.mu.Lock()
	f.jpg = b
	f.seq = frameSeq.Add(1)
	f.mu.Unlock()
}

func (f *frameBuf) get() ([]byte, uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jpg, f.seq
}

var (
	jpegSOI = []byte{0xFF, 0xD8}
	jpegEOI = []byte{0xFF, 0xD9}
)

// readJPEGs splits ffmpeg's image2pipe MJPEG output into frames. ffmpeg doesn't
// embed thumbnails, so each SOI..EOI pair is exactly one frame.
func readJPEGs(r io.Reader, emit func([]byte)) {
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 64<<10)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				s := bytes.Index(buf, jpegSOI)
				if s < 0 {
					buf = buf[:0]
					break
				}
				e := bytes.Index(buf[s+2:], jpegEOI)
				if e < 0 {
					if s > 0 {
						buf = append(buf[:0], buf[s:]...)
					}
					break
				}
				end := s + 2 + e + 2
				frame := make([]byte, end-s)
				copy(frame, buf[s:end])
				emit(frame)
				buf = append(buf[:0], buf[end:]...)
			}
			if len(buf) > 16<<20 { // garbage, not a frame
				buf = buf[:0]
			}
		}
		if err != nil {
			return
		}
	}
}
