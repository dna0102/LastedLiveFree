package capture

import (
	"bytes"
	"image"
	"image/jpeg"
)

// thumbJPEG shrinks a BGRA frame to at most maxW wide (box filter) and encodes
// it as JPEG.
func thumbJPEG(bgra []byte, w, h, maxW int) ([]byte, error) {
	step := 1
	for w/step > maxW {
		step++
	}
	tw, th := w/step, h/step
	img := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			var r, g, b int
			for dy := 0; dy < step; dy++ {
				row := ((y*step + dy) * w) * 4
				for dx := 0; dx < step; dx++ {
					i := row + (x*step+dx)*4
					b += int(bgra[i])
					g += int(bgra[i+1])
					r += int(bgra[i+2])
				}
			}
			n := step * step
			o := (y*tw + x) * 4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes(), err
}
