package main

import (
	"fmt"
	"image"
	"image/color"
)

func main() {
	r := image.Rect(0, 0, 4, 3)
	fmt.Println(r.Dx(), r.Dy())
	fmt.Println(r.Empty())

	p := image.Pt(2, 1)
	fmt.Println(p.In(r))
	fmt.Println(image.Pt(10, 10).In(r))

	moved := r.Add(image.Pt(5, 5))
	fmt.Println(moved.Min.X, moved.Min.Y, moved.Max.X, moved.Max.Y)

	a := image.Rect(0, 0, 5, 5)
	b := image.Rect(3, 3, 8, 8)
	inter := a.Intersect(b)
	fmt.Println(inter.Min.X, inter.Min.Y, inter.Max.X, inter.Max.Y)
	uni := a.Union(b)
	fmt.Println(uni.Min.X, uni.Min.Y, uni.Max.X, uni.Max.Y)

	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	var _ image.Image = img
	fmt.Println(img.Bounds().Dx(), img.Bounds().Dy())

	img.Set(1, 1, color.Rgba{R: 10, G: 20, B: 30, A: 255})
	c := img.At(1, 1)
	cr, cg, cb, ca := c.RGBA()
	fmt.Println(cr>>8, cg>>8, cb>>8, ca>>8)

	outside := img.At(100, 100)
	or, og, ob, oa := outside.RGBA()
	fmt.Println(or, og, ob, oa)
}
