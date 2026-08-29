package main

import (
	"fmt"
	"image/color"
)

func main() {
	red := color.Rgba{R: 255, G: 0, B: 0, A: 255}
	var c color.Color = red
	r, g, b, a := c.RGBA()
	fmt.Println(r, g, b, a)

	gray := color.GrayModel.Convert(red)
	grayVal, ok := gray.(color.Gray)
	fmt.Println(ok)
	fmt.Println(grayVal.Y)

	white := color.CMYK{C: 0, M: 0, Y: 0, K: 0}
	rgbaWhite := color.RGBAModel.Convert(white)
	rw, ok2 := rgbaWhite.(color.Rgba)
	fmt.Println(ok2)
	fmt.Println(rw.R, rw.G, rw.B, rw.A)

	semi := color.NRGBA{R: 255, G: 0, B: 0, A: 128}
	rgbaSemi := color.RGBAModel.Convert(semi)
	rs, _ := rgbaSemi.(color.Rgba)
	fmt.Println(rs.R, rs.G, rs.B, rs.A)

	fmt.Println(color.Black.Y, color.White.Y)
}
