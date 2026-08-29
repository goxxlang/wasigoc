// Tiny subset of log: no timestamp/file-line prefix (no time.Now yet, see
// README's stdlib tracker) and no *Logger type -- just the package-level
// funcs, writing to stdout (os.Stdout isn't wired up as an io.Writer yet
// either, so real Go's stderr default isn't available).
//
// No Printf/Fatalf/Panicf: fmt.Printf/Sprintf's format string is parsed at
// wasigoc *compile* time and must be a string literal at that call site --
// a `format string` parameter forwarded from a caller can never satisfy
// that, so there is no way to build a Printf-shaped wrapper at all here.
package log

import (
	"fmt"
	"os"
)

func printAll(v []any) {
	for i, x := range v {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Print(x)
	}
}

func Print(v ...any) {
	printAll(v)
}

func Println(v ...any) {
	printAll(v)
	fmt.Print("\n")
}

func Fatal(v ...any) {
	printAll(v)
	os.Exit(1)
}

func Fatalln(v ...any) {
	printAll(v)
	fmt.Print("\n")
	os.Exit(1)
}

func sprintAll(v []any) string {
	s := ""
	for i, x := range v {
		if i > 0 {
			s = s + " "
		}
		s = s + fmt.Sprint(x)
	}
	return s
}

func Panic(v ...any) {
	panic(sprintAll(v))
}

func Panicln(v ...any) {
	panic(sprintAll(v) + "\n")
}
