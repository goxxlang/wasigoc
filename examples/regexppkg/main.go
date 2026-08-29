package main

import (
	"fmt"
	"regexp"
)

func main() {
	re := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	fmt.Println(re.MatchString("hello_world1"))
	fmt.Println(re.MatchString("1hello"))
	fmt.Println(re.MatchString(""))

	re2 := regexp.MustCompile(`\d+`)
	fmt.Println(re2.FindString("abc123def456"))
	all := re2.FindAllString("abc123def456", -1)
	fmt.Println(len(all))
	for _, s := range all {
		fmt.Println(s)
	}

	re3 := regexp.MustCompile(`(foo|bar)+`)
	fmt.Println(re3.MatchString("foobarfoo"))
	fmt.Println(re3.MatchString("baz"))

	re4 := regexp.MustCompile(`\s+`)
	fmt.Println(re4.ReplaceAllString("a   b\tc  d", " "))

	re5 := regexp.MustCompile(`,\s*`)
	parts := re5.Split("a, b,c ,  d", -1)
	fmt.Println(len(parts))
	for _, p := range parts {
		fmt.Println(p)
	}

	re6 := regexp.MustCompile(`colou?r`)
	fmt.Println(re6.MatchString("color"))
	fmt.Println(re6.MatchString("colour"))
	fmt.Println(re6.MatchString("colouur"))

	re7 := regexp.MustCompile(`a.c`)
	fmt.Println(re7.MatchString("abc"))
	fmt.Println(re7.MatchString("a\nc"))

	_, err := regexp.Compile(`(unclosed`)
	fmt.Println(err != nil)

	ok, err2 := regexp.MatchString(`^\w+@\w+\.\w+$`, "user@example.com")
	fmt.Println(ok, err2)
}
