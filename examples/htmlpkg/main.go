package main

import (
	"fmt"
	"html"
)

func main() {
	fmt.Println(html.EscapeString(`<script>alert("hi & 'bye'")</script>`) ==
		"&lt;script&gt;alert(&#34;hi &amp; &#39;bye&#39;&#34;)&lt;/script&gt;")

	// Round trip: escape then unescape recovers the original.
	orig := `<a href="x">Tom & Jerry's "great" show</a>`
	escaped := html.EscapeString(orig)
	fmt.Println(html.UnescapeString(escaped) == orig)

	// Named entities beyond what EscapeString itself produces.
	fmt.Println(html.UnescapeString("A &amp; B") == "A & B")
	fmt.Println(html.UnescapeString("&lt;tag&gt;") == "<tag>")
	fmt.Println(html.UnescapeString("&apos;hi&apos;") == "'hi'")

	// Numeric character references, decimal and hex.
	fmt.Println(html.UnescapeString("&#65;&#66;&#67;") == "ABC")
	fmt.Println(html.UnescapeString("&#x41;&#x42;&#x43;") == "ABC")

	// Unknown/malformed entities pass through unchanged.
	fmt.Println(html.UnescapeString("A & B") == "A & B")
	fmt.Println(html.UnescapeString("&unknownEntity;") == "&unknownEntity;")
	fmt.Println(html.UnescapeString("100% done") == "100% done")
}
