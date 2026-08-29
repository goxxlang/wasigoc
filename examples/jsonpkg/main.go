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

	var bad any
	badErr2 := json.Unmarshal([]byte("{bad json"), &bad)
	fmt.Println(badErr2 != nil)

	fn := func() {}
	_, badErr3 := json.Marshal(fn)
	fmt.Println(badErr3 != nil)
}
