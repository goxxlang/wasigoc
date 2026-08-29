package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

type uniform struct {
	c color.Color
}

func (u *uniform) ColorModel() color.Model     { return color.RGBAModel }
func (u *uniform) Bounds() image.Rectangle     { return image.Rect(-1000000000, -1000000000, 1000000000, 1000000000) }
func (u *uniform) At(x int, y int) color.Color { return u.c }

func main() {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	x := 0
	for x < 2 {
		y := 0
		for y < 2 {
			dst.Set(x, y, color.Rgba{R: 100, G: 100, B: 100, A: 255})
			y = y + 1
		}
		x = x + 1
	}

	src := &uniform{c: color.Rgba{R: 200, G: 50, B: 0, A: 255}}
	draw.Draw(dst, image.Rect(0, 0, 1, 1), src, image.Pt(0, 0), draw.Src)
	c := dst.At(0, 0).(color.Rgba)
	fmt.Println(c.R == 200)
	fmt.Println(c.G == 50)
	fmt.Println(c.B == 0)
	fmt.Println(c.A == 255)

	c2 := dst.At(1, 0).(color.Rgba)
	fmt.Println(c2.R == 100)

	blendSrc := &uniform{c: color.Rgba{R: 100, G: 25, B: 0, A: 128}}
	draw.Draw(dst, image.Rect(0, 1, 1, 2), blendSrc, image.Pt(0, 0), draw.Over)
	c3 := dst.At(0, 1).(color.Rgba)
	fmt.Println(c3.R == 150)
	fmt.Println(c3.G == 75)
	fmt.Println(c3.B == 49)
	fmt.Println(c3.A == 255)
}
