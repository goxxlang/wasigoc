// Tiny subset of flag: String/Int/Bool (+ Var forms), Parse, Args/NArg/Arg.
// No FlagSet type (only the package-level "CommandLine" flag set real Go
// itself is built on), no Usage/PrintDefaults, no Float64/Duration flags.
package flag

import (
	"os"
	"strconv"
	"strings"
)

var stringFlags = make(map[string]*string)
var intFlags = make(map[string]*int)
var boolFlags = make(map[string]*bool)
var usages = make(map[string]string)
var parsedArgs []string
var didParse bool

func String(name string, value string, usage string) *string {
	p := new(string)
	*p = value
	stringFlags[name] = p
	usages[name] = usage
	return p
}

func StringVar(p *string, name string, value string, usage string) {
	*p = value
	stringFlags[name] = p
	usages[name] = usage
}

func Int(name string, value int, usage string) *int {
	p := new(int)
	*p = value
	intFlags[name] = p
	usages[name] = usage
	return p
}

func IntVar(p *int, name string, value int, usage string) {
	*p = value
	intFlags[name] = p
	usages[name] = usage
}

func Bool(name string, value bool, usage string) *bool {
	p := new(bool)
	*p = value
	boolFlags[name] = p
	usages[name] = usage
	return p
}

func BoolVar(p *bool, name string, value bool, usage string) {
	*p = value
	boolFlags[name] = p
	usages[name] = usage
}

func trimDashes(s string) (string, bool) {
	if strings.HasPrefix(s, "--") {
		return s[2:], true
	}
	if strings.HasPrefix(s, "-") {
		return s[1:], true
	}
	return s, false
}

// Parse reads os.Args[1:]. Supports "-name value", "-name=value",
// "--name value", "--name=value", and bare "-flag"/"--flag" for bool
// flags; stops at the first non-flag argument or a bare "--".
func Parse() {
	raw := os.Args[1:]
	i := 0
	for i < len(raw) {
		a := raw[i]
		name, isFlag := trimDashes(a)
		if !isFlag {
			break
		}
		if name == "" {
			i++
			break
		}
		val := ""
		hasVal := false
		eq := strings.Index(name, "=")
		if eq >= 0 {
			val = name[eq+1:]
			name = name[0:eq]
			hasVal = true
		}
		if p, ok := boolFlags[name]; ok {
			if hasVal {
				b, _ := strconv.ParseBool(val)
				*p = b
			} else {
				*p = true
			}
			i++
			continue
		}
		if !hasVal {
			if i+1 < len(raw) {
				val = raw[i+1]
				i = i + 2
			} else {
				i++
			}
		} else {
			i++
		}
		if p, ok := stringFlags[name]; ok {
			*p = val
			continue
		}
		if p, ok := intFlags[name]; ok {
			n, _ := strconv.Atoi(val)
			*p = n
			continue
		}
	}
	parsedArgs = raw[i:]
	didParse = true
}

func Args() []string {
	return parsedArgs
}

func NArg() int {
	return len(parsedArgs)
}

func Arg(i int) string {
	if i < 0 || i >= len(parsedArgs) {
		return ""
	}
	return parsedArgs[i]
}

func Parsed() bool {
	return didParse
}
