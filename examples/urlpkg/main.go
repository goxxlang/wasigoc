package main

import (
	"fmt"
	"net/url"
)

func main() {
	fmt.Println(url.QueryEscape("hello world/foo?"))
	s, err := url.QueryUnescape("hello+world%2Ffoo%3F")
	fmt.Println(s, err)

	fmt.Println(url.PathEscape("a b/c"))

	u, err2 := url.Parse("https://example.com/path/to/thing?a=1&b=two#frag")
	fmt.Println(err2)
	fmt.Println(u.Scheme, u.Host, u.Path, u.RawQuery, u.Fragment)
	fmt.Println(u.String())

	q, err3 := url.ParseQuery(u.RawQuery)
	fmt.Println(err3)
	fmt.Println(q["a"], q["b"])

	u2, _ := url.Parse("/just/a/path")
	fmt.Println(u2.Scheme == "", u2.Path)
}
