package main

import (
	"bytes"
	"fmt"
	"image/gif"
)

// Real GIF fixtures. `base` was produced by Pillow 12.3.0 (a real GIF
// encoder), `interlace` is the same 4x3 image hand-framed with a real
// 4-pass interlaced pixel order and compressed through real Go's own
// compress/lzw.Writer, `transparent` is `base` re-saved by Pillow with a
// Graphic Control Extension marking color index 0 transparent. All three
// were decoded with real Go's own image/gif.Decode as the independent
// oracle for the expected pixel values checked below.
var baseGIF = []byte{
	71, 73, 70, 56, 55, 97, 4, 0, 3, 0, 131, 0, 0, 0, 0, 100, 50, 0, 100, 100, 0, 100, 150, 0,
	100, 0, 50, 100, 50, 50, 100, 100, 50, 100, 150, 50, 100, 0, 100, 100, 50, 100, 100, 100,
	100, 100, 150, 100, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 44, 0, 0, 0, 0, 4, 0, 3, 0, 0,
	8, 16, 0, 1, 4, 16, 48, 128, 64, 1, 3, 7, 16, 36, 80, 176, 32, 32, 0, 59,
}

var interlaceGIF = []byte{
	71, 73, 70, 56, 57, 97, 4, 0, 3, 0, 131, 0, 0, 0, 0, 100, 50, 0, 100, 100, 0, 100, 150, 0,
	100, 0, 50, 100, 50, 50, 100, 100, 50, 100, 150, 50, 100, 0, 100, 100, 50, 100, 100, 100,
	100, 100, 150, 100, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 44, 0, 0, 0, 0, 4, 0, 3, 0, 64,
	4, 9, 16, 4, 49, 80, 82, 139, 20, 115, 34, 0, 59,
}

var transparentGIF = []byte{
	71, 73, 70, 56, 57, 97, 4, 0, 3, 0, 131, 0, 0, 0, 0, 100, 50, 0, 100, 100, 0, 100, 150, 0,
	100, 0, 50, 100, 50, 50, 100, 100, 50, 100, 150, 50, 100, 0, 100, 100, 50, 100, 100, 100,
	100, 100, 150, 100, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 33, 249, 4, 1, 0, 0, 0, 0, 44,
	0, 0, 0, 0, 4, 0, 3, 0, 0, 8, 16, 0, 1, 4, 16, 48, 128, 64, 1, 3, 7, 16, 36, 80, 176, 32,
	32, 0, 59,
}

func check(name string, data []byte, wantAlpha0 bool) {
	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Println(name, "ERROR", err)
		return
	}
	b := img.Bounds()
	ok := b.Dx() == 4 && b.Dy() == 3
	match := true
	x := 0
	for x < 4 {
		y := 0
		for y < 3 {
			off := img.PixOffset(x, y)
			r := img.Pix[off]
			g := img.Pix[off+1]
			bl := img.Pix[off+2]
			a := img.Pix[off+3]
			if x == 0 && y == 0 && wantAlpha0 {
				if r != 0 || g != 0 || bl != 0 || a != 0 {
					match = false
				}
			} else {
				if r != byte(x*50) || g != byte(y*50) || bl != 100 || a != 255 {
					match = false
				}
			}
			y = y + 1
		}
		x = x + 1
	}
	fmt.Println(ok, match)
}

func main() {
	check("base", baseGIF, false)
	check("interlace", interlaceGIF, false)
	check("transparent", transparentGIF, true)

	_, err := gif.Decode(bytes.NewReader([]byte("not a gif")))
	fmt.Println(err != nil)
}
