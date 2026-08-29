package main

import (
	"encoding"
	"fmt"
	"strconv"
	"strings"
)

type Point struct {
	X int
	Y int
}

func (p *Point) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(p.X) + "," + strconv.Itoa(p.Y)), nil
}

func (p *Point) UnmarshalText(text []byte) error {
	parts := strings.Split(string(text), ",")
	x, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	y, err2 := strconv.Atoi(parts[1])
	if err2 != nil {
		return err2
	}
	p.X = x
	p.Y = y
	return nil
}

func (p *Point) MarshalBinary() ([]byte, error) {
	return []byte{byte(p.X), byte(p.Y)}, nil
}

func (p *Point) UnmarshalBinary(data []byte) error {
	p.X = int(data[0])
	p.Y = int(data[1])
	return nil
}

func main() {
	p := &Point{X: 3, Y: 4}

	var tm encoding.TextMarshaler = p
	text, err := tm.MarshalText()
	fmt.Println(string(text))
	fmt.Println(err == nil)

	var tu encoding.TextUnmarshaler = &Point{}
	tu.UnmarshalText(text)
	fmt.Println(tu.(*Point).X, tu.(*Point).Y)

	var bm encoding.BinaryMarshaler = p
	bin, _ := bm.MarshalBinary()
	fmt.Println(len(bin))

	var bu encoding.BinaryUnmarshaler = &Point{}
	bu.UnmarshalBinary(bin)
	fmt.Println(bu.(*Point).X, bu.(*Point).Y)
}
