package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

func recEq(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func main() {
	// Simple records.
	r1 := csv.NewReader(strings.NewReader("name,age\nAlice,30\nBob,25\n"))
	recs1, err1 := r1.ReadAll()
	fmt.Println(err1 == nil)
	fmt.Println(len(recs1))
	fmt.Println(recEq(recs1[0], []string{"name", "age"}))
	fmt.Println(recEq(recs1[1], []string{"Alice", "30"}))
	fmt.Println(recEq(recs1[2], []string{"Bob", "25"}))

	// Quoted field with an embedded comma.
	r2 := csv.NewReader(strings.NewReader("a,\"b,c\",d\n"))
	rec2, err2 := r2.Read()
	fmt.Println(err2 == nil)
	fmt.Println(recEq(rec2, []string{"a", "b,c", "d"}))

	// Quoted field with an embedded newline.
	r3 := csv.NewReader(strings.NewReader("\"line1\nline2\",x\n"))
	rec3, err3 := r3.Read()
	fmt.Println(err3 == nil)
	fmt.Println(recEq(rec3, []string{"line1\nline2", "x"}))

	// Doubled-quote escaping.
	r4 := csv.NewReader(strings.NewReader("\"she said \"\"hi\"\"\",y\n"))
	rec4, err4 := r4.Read()
	fmt.Println(err4 == nil)
	fmt.Println(recEq(rec4, []string{"she said \"hi\"", "y"}))

	// Comment lines and blank lines are skipped.
	r5 := csv.NewReader(strings.NewReader("# a comment\na,b\n\nc,d\n"))
	r5.Comment = byte(35)
	recs5, err5 := r5.ReadAll()
	fmt.Println(err5 == nil)
	fmt.Println(len(recs5))
	fmt.Println(recEq(recs5[0], []string{"a", "b"}))
	fmt.Println(recEq(recs5[1], []string{"c", "d"}))

	// TrimLeadingSpace.
	r6 := csv.NewReader(strings.NewReader("a,  b,   c\n"))
	r6.TrimLeadingSpace = true
	rec6, _ := r6.Read()
	fmt.Println(recEq(rec6, []string{"a", "b", "c"}))

	// FieldsPerRecord mismatch is a real error.
	r7 := csv.NewReader(strings.NewReader("a,b,c\nd,e\n"))
	_, err7a := r7.Read()
	fmt.Println(err7a == nil)
	_, err7b := r7.Read()
	fmt.Println(err7b != nil)

	// Unterminated quoted field is a real error.
	r8 := csv.NewReader(strings.NewReader("\"unterminated"))
	_, err8 := r8.Read()
	fmt.Println(err8 != nil)

	// Writer round trip: fields needing quotes (comma, quote, newline,
	// leading/trailing space) survive Write -> Read unchanged.
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	original := [][]string{
		[]string{"plain", "has,comma", "has\"quote", "has\nnewline", " leading space", "trailing space "},
	}
	errW := w.WriteAll(original)
	fmt.Println(errW == nil)

	r9 := csv.NewReader(strings.NewReader(buf.String()))
	recs9, err9 := r9.ReadAll()
	fmt.Println(err9 == nil)
	fmt.Println(len(recs9))
	fmt.Println(recEq(recs9[0], original[0]))
}
