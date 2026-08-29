// Tiny subset of path/filepath. WASI has no drive letters or backslash
// separators; paths are slash-separated the same as package path.
package filepath

func Base(name string) string {
	if name == "" {
		return "."
	}
	n := len(name)
	for n > 0 && name[n-1:n] == "/" {
		n--
	}
	if n == 0 {
		return "/"
	}
	i := n - 1
	for i >= 0 && name[i:i+1] != "/" {
		i--
	}
	return name[i+1 : n]
}

func Dir(name string) string {
	n := len(name)
	i := n - 1
	for i >= 0 && name[i:i+1] != "/" {
		i--
	}
	if i < 0 {
		return "."
	}
	for i > 0 && name[i:i+1] == "/" {
		i--
	}
	if i == 0 && name[0:1] == "/" {
		return "/"
	}
	return name[0 : i+1]
}

func Ext(name string) string {
	for i := len(name) - 1; i >= 0 && name[i:i+1] != "/"; i-- {
		if name[i:i+1] == "." {
			return name[i:]
		}
	}
	return ""
}

func Join(elem ...string) string {
	out := ""
	for i := 0; i < len(elem); i++ {
		e := elem[i]
		if e == "" {
			continue
		}
		if out == "" {
			out = e
			continue
		}
		if HasSlash(out) {
			out = out + e
		} else {
			out = out + "/" + e
		}
	}
	if out == "" {
		return ""
	}
	return out
}

func HasSlash(s string) bool {
	n := len(s)
	return n > 0 && s[n-1:n] == "/"
}

func ToSlash(path string) string {
	return path
}

func FromSlash(path string) string {
	return path
}

func IsAbs(path string) bool {
	return len(path) > 0 && path[0:1] == "/"
}

func Split(path string) (string, string) {
	i := len(path) - 1
	for i >= 0 && path[i:i+1] != "/" {
		i--
	}
	return path[0 : i+1], path[i+1:]
}

func splitSegs(path string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i:i+1] == "/" {
			if i > start {
				out = append(out, path[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func Clean(path string) string {
	if path == "" {
		return "."
	}
	abs := IsAbs(path)
	segs := splitSegs(path)
	var out []string
	for i := 0; i < len(segs); i++ {
		seg := segs[i]
		if seg == "." {
			continue
		}
		if seg == ".." {
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[0 : len(out)-1]
				continue
			}
			if abs {
				continue
			}
			out = append(out, "..")
			continue
		}
		out = append(out, seg)
	}
	res := ""
	for i := 0; i < len(out); i++ {
		if i > 0 {
			res = res + "/"
		}
		res = res + out[i]
	}
	if abs {
		res = "/" + res
	}
	if res == "" {
		return "."
	}
	return res
}
