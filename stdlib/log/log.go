// Tiny subset of log: no timestamp/file-line prefix (no time.Now yet, see
// README's stdlib tracker) and no *Logger type -- just the package-level
// funcs, writing to stdout (os.Stdout isn't wired up as an io.Writer yet
// either, so real Go's stderr default isn't available).
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

// Printf/Fatalf/Panicf forward format+v straight to fmt.Printf/Sprintf,
// which support a non-literal (parameter) format string via a runtime
// verb walk -- see docs/language.md's "Limits worth knowing" for what
// that path doesn't check at compile time.
func Printf(format string, v ...any) {
	fmt.Printf(format, v...)
}

func Fatalf(format string, v ...any) {
	fmt.Printf(format, v...)
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

func Panicf(format string, v ...any) {
	panic(fmt.Sprintf(format, v...))
}
