package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func main() {
	fmt.Println(strings.TrimSpace("  hi  "))
	fmt.Println(strings.Trim("xxhixx", "x"))
	fields := strings.Fields("  a  b c ")
	fmt.Println(len(fields), fields[0], fields[1], fields[2])
	before, after, found := strings.Cut("key=value", "=")
	fmt.Println(before, after, found)
	fmt.Println(strings.ReplaceAll("aaa", "a", "b"))
	fmt.Println(strings.EqualFold("Go", "GO"))
	fmt.Println(strings.LastIndex("abcabc", "b"))

	n, err := strconv.ParseInt("42", 10, 64)
	fmt.Println(n, err)
	h, err2 := strconv.ParseInt("0x2A", 0, 64)
	fmt.Println(h, err2)
	fmt.Println(strconv.FormatInt(255, 16))

	var b bytes.Buffer
	b.WriteString("hello world")
	out, _ := io.ReadAll(&b)
	fmt.Println(string(out))

	fmt.Println(string(bytes.TrimSpace([]byte("  hi  "))))
	cutb, cuta, cutok := bytes.Cut([]byte("k=v"), []byte("="))
	fmt.Println(string(cutb), string(cuta), cutok)

	fmt.Println(path.Clean("a/b/../c"))
	fmt.Println(path.IsAbs("/a/b"))
	dir, file := path.Split("/a/b/c.go")
	fmt.Println(dir, file)
	fmt.Println(filepath.Clean("a//b/./c"))

	xs := []int{5, 3, 1, 4, 2}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	fmt.Println(xs[0], xs[1], xs[2], xs[3], xs[4])
	idx := sort.SearchInts(xs, 3)
	fmt.Println(idx)

	fmt.Println(math.Floor(3.7))
	fmt.Println(math.Ceil(3.2))
	fmt.Println(math.Pow(2, 10))
	fmt.Println(math.IsNaN(math.NaN()))
	fmt.Println(math.IsInf(math.Inf(1), 1))
	fmt.Println(math.Mod(10, 3))

	fmt.Println(utf8.RuneLen('A'))
	fmt.Println(utf8.ValidString("hello"))
}
