package main

import (
	"expvar"
	"fmt"
)

func main() {
	reqs := expvar.NewInt("requests")
	reqs.Add(1)
	reqs.Add(4)
	fmt.Println(reqs.Value())
	fmt.Println(reqs.String())

	name := expvar.NewString("name")
	name.Set("wasigoc")
	fmt.Println(name.Value())
	fmt.Println(name.String())

	ratio := expvar.NewFloat("ratio")
	ratio.Set(0.5)
	ratio.Add(0.25)
	fmt.Println(ratio.Value())

	m := expvar.NewMap("stats")
	m.Set("a", reqs)
	m.Set("b", name)
	fmt.Println(m.String())

	count := 0
	m.Do(func(kv expvar.KeyValue) {
		count = count + 1
	})
	fmt.Println(count)

	got := expvar.Get("requests")
	fmt.Println(got.String() == reqs.String())

	missing := expvar.Get("does-not-exist")
	fmt.Println(missing == nil)

	total := 0
	expvar.Do(func(kv expvar.KeyValue) {
		total = total + 1
	})
	fmt.Println(total)
}
