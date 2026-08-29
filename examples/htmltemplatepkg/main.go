package main

import (
	"bytes"
	"fmt"
	"html/template"
)

type Comment struct {
	Author string
	Body   string
}

func main() {
	t, err := template.New("c").Parse("<p>{{.Author}} says: {{.Body}}</p>")
	fmt.Println(err == nil)
	var buf bytes.Buffer
	t.Execute(&buf, Comment{Author: "Bob", Body: "<script>alert('x')</script> & stuff"})
	expected := "<p>Bob says: &lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt; &amp; stuff</p>"
	fmt.Println(buf.String() == expected)
}
