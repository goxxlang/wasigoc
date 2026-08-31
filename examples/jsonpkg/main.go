package main

import (
	"encoding/json"
	"fmt"
)

type Point struct {
	X int
	Y int
}

type NamedPoint struct {
	Label string
	P     Point
}

type Tagged struct {
	Name string `json:"name"`
	Skip int    `json:"-"`
}

func main() {
	src := `{"name":"Alice","age":30,"tags":["a","b"],"active":true,"score":4.5,"nested":{"x":1},"nil":null}`
	var v any
	err := json.Unmarshal([]byte(src), &v)
	fmt.Println(err)

	m, ok := v.(map[string]any)
	fmt.Println(ok)
	name, _ := m["name"].(string)
	fmt.Println(name)
	age, _ := m["age"].(float64)
	fmt.Println(age)
	active, _ := m["active"].(bool)
	fmt.Println(active)
	score, _ := m["score"].(float64)
	fmt.Println(score)
	tags, _ := m["tags"].([]any)
	fmt.Println(len(tags))
	fmt.Println(m["nil"] == nil)

	out, err2 := json.Marshal(m)
	fmt.Println(err2)

	var v2 any
	json.Unmarshal(out, &v2)
	out2, _ := json.Marshal(v2)
	fmt.Println(string(out) == string(out2))

	simple := map[string]any{"b": 2.0, "a": 1.0}
	sb, _ := json.Marshal(simple)
	fmt.Println(string(sb))

	arr := []any{1.0, "two", true, nil}
	ab, _ := json.Marshal(arr)
	fmt.Println(string(ab))

	esc := map[string]any{"s": "line1\nline2\t\"q\""}
	eb, _ := json.Marshal(esc)
	fmt.Println(string(eb))

	var back any
	err3 := json.Unmarshal(eb, &back)
	fmt.Println(err3)
	bm, _ := back.(map[string]any)
	fmt.Println(bm["s"])

	pb, perr := json.Marshal(Point{X: 5, Y: 7})
	fmt.Println(perr)
	fmt.Println(string(pb))

	np := NamedPoint{Label: "origin", P: Point{X: 0, Y: 0}}
	nb, nerr := json.Marshal(np)
	fmt.Println(nerr)
	fmt.Println(string(nb))

	var p Point
	uerr := json.Unmarshal([]byte(`{"X":9,"Y":4}`), &p)
	fmt.Println(uerr)
	fmt.Println(p.X)
	fmt.Println(p.Y)

	var a Point
	a.X = 1
	a.Y = 2
	var b Point
	b.X = 3
	b.Y = 4
	var pts []Point
	pts = append(pts, a)
	pts = append(pts, b)
	sl, serr := json.Marshal(pts)
	fmt.Println(serr)
	fmt.Println(string(sl))

	var np2 NamedPoint
	nerr2 := json.Unmarshal([]byte(`{"Label":"here","P":{"X":8,"Y":1}}`), &np2)
	fmt.Println(nerr2)
	fmt.Println(np2.Label)
	inner := np2.P
	fmt.Println(inner.X)

	var bad any
	badErr2 := json.Unmarshal([]byte("{bad json"), &bad)
	fmt.Println(badErr2 != nil)

	fn := func() {}
	_, badErr3 := json.Marshal(fn)
	fmt.Println(badErr3 != nil)

	tg := Tagged{Name: "bob", Skip: 9}
	tb, _ := json.Marshal(tg)
	fmt.Println(string(tb))
	var backT Tagged
	json.Unmarshal([]byte(`{"name":"ann"}`), &backT)
	fmt.Println(backT.Name)
}
