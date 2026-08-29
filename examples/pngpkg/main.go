package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	x := 0
	for x < 4 {
		y := 0
		for y < 3 {
			off := img.PixOffset(x, y)
			img.Pix[off] = byte(x * 50)
			img.Pix[off+1] = byte(y * 50)
			img.Pix[off+2] = 100
			img.Pix[off+3] = 255
			y = y + 1
		}
		x = x + 1
	}

	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	fmt.Println(err == nil)
	fmt.Println(buf.Len() > 0)

	decoded, derr := png.Decode(bytes.NewReader(buf.Bytes()))
	fmt.Println(derr == nil)
	fmt.Println(decoded.Bounds().Dx() == 4)
	fmt.Println(decoded.Bounds().Dy() == 3)

	match := true
	x = 0
	for x < 4 {
		y := 0
		for y < 3 {
			off1 := img.PixOffset(x, y)
			off2 := decoded.PixOffset(x, y)
			if img.Pix[off1] != decoded.Pix[off2] || img.Pix[off1+1] != decoded.Pix[off2+1] || img.Pix[off1+2] != decoded.Pix[off2+2] || img.Pix[off1+3] != decoded.Pix[off2+3] {
				match = false
			}
			y = y + 1
		}
		x = x + 1
	}
	fmt.Println(match)

	_, badErr := png.Decode(bytes.NewReader([]byte("not a png")))
	fmt.Println(badErr != nil)
}
