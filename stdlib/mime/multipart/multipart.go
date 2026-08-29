// Bounded subset of mime/multipart (RFC 2046 multipart/form-data).
//
// Writer: whole-value-at-once (`WriteField`/`WriteFile` take the complete
// value/data, not real Go's `CreateFormField`/`CreateFormFile` shape
// (which return an `io.Writer` for the caller to stream into) -- simpler,
// still produces correctly-formed output, same "not truly streaming"
// bounded precedent as this project's `encoding/csv`. `SetBoundary` never
// validates its argument (real Go rejects boundaries with illegal
// characters); the self-generated default boundary is built from
// `math/rand`, NOT cryptographically random (real Go's isn't fully either,
// but does use `crypto/rand`, unavailable as real entropy here -- see
// README's `crypto/rand` tracker line).
//
// Reader: parses the WHOLE body up front (via `io.ReadAll` at `NewReader`
// time) into a slice of `*Part`, not real Go's true streaming
// boundary-by-boundary reader -- same bounded shape as `encoding/csv`'s
// `Reader`. `Part.Header` is a real `net/textproto.MIMEHeader`, but parsed
// here directly (one header per line, no RFC 822 continuation-line
// folding) rather than routed through `net/textproto.Reader.ReadMIMEHeader`.
package multipart

import (
	"bytes"
	"io"
	"math/rand"
	"strings"
)

type Writer struct {
	w        io.Writer
	boundary string
}

const boundaryAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomBoundary() string {
	buf := make([]byte, 30)
	for i := 0; i < 30; i++ {
		buf[i] = boundaryAlphabet[rand.Intn(len(boundaryAlphabet))]
	}
	return string(buf)
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w, boundary: randomBoundary()}
}

func (w *Writer) Boundary() string {
	return w.boundary
}

func (w *Writer) SetBoundary(boundary string) error {
	w.boundary = boundary
	return nil
}

func (w *Writer) WriteField(fieldname string, value string) error {
	io.WriteString(w.w, "--"+w.boundary+"\r\n")
	io.WriteString(w.w, "Content-Disposition: form-data; name=\""+fieldname+"\"\r\n\r\n")
	io.WriteString(w.w, value)
	io.WriteString(w.w, "\r\n")
	return nil
}

func (w *Writer) WriteFile(fieldname string, filename string, data []byte) error {
	io.WriteString(w.w, "--"+w.boundary+"\r\n")
	io.WriteString(w.w, "Content-Disposition: form-data; name=\""+fieldname+"\"; filename=\""+filename+"\"\r\n")
	io.WriteString(w.w, "Content-Type: application/octet-stream\r\n\r\n")
	w.w.Write(data)
	io.WriteString(w.w, "\r\n")
	return nil
}

func (w *Writer) Close() error {
	io.WriteString(w.w, "--"+w.boundary+"--\r\n")
	return nil
}

type Part struct {
	Header textproto_MIMEHeader
	body   []byte
	pos    int
}

// A local alias, not an import of net/textproto, so this package's own
// doc comment can describe the field's real type without a second import
// line -- `type MIMEHeader = map[string][]string` isn't how net/textproto
// spells it (a defined type, not an alias), so Part.Header's static type
// below is spelled out directly instead of pulled in as a dependency this
// package doesn't otherwise need.
type textproto_MIMEHeader map[string][]string

func headerGet(h textproto_MIMEHeader, key string) string {
	vals := h[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func dispositionParam(header string, key string) string {
	needle := key + "=\""
	idx := strings.Index(header, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	end := strings.Index(header[start:], "\"")
	if end < 0 {
		return ""
	}
	return header[start : start+end]
}

func (p *Part) FormName() string {
	return dispositionParam(headerGet(p.Header, "Content-Disposition"), "name")
}

func (p *Part) FileName() string {
	return dispositionParam(headerGet(p.Header, "Content-Disposition"), "filename")
}

func (p *Part) Read(buf []byte) (int, error) {
	if p.pos >= len(p.body) {
		return 0, io.EOF
	}
	n := copy(buf, p.body[p.pos:])
	p.pos = p.pos + n
	return n, nil
}

func parseHeaderLines(raw []byte) textproto_MIMEHeader {
	h := textproto_MIMEHeader{}
	lines := strings.Split(string(raw), "\r\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		h[key] = append(h[key], val)
	}
	return h
}

type Reader struct {
	parts []*Part
	idx   int
}

func NewReader(r io.Reader, boundary string) *Reader {
	data, _ := io.ReadAll(r)
	return &Reader{parts: parseParts(data, boundary)}
}

func parseParts(data []byte, boundary string) []*Part {
	var parts []*Part
	delim := []byte("--" + boundary)
	idx := bytes.Index(data, delim)
	if idx < 0 {
		return parts
	}
	rest := data[idx+len(delim):]
	for {
		if len(rest) >= 2 && rest[0] == '-' && rest[1] == '-' {
			break
		}
		if len(rest) >= 2 && rest[0] == '\r' && rest[1] == '\n' {
			rest = rest[2:]
		}
		sep := bytes.Index(rest, []byte("\r\n\r\n"))
		if sep < 0 {
			break
		}
		headerBytes := rest[:sep]
		bodyStart := rest[sep+4:]
		nextIdx := bytes.Index(bodyStart, delim)
		if nextIdx < 0 {
			break
		}
		bodyEnd := nextIdx
		if bodyEnd >= 2 && bodyStart[bodyEnd-2] == '\r' && bodyStart[bodyEnd-1] == '\n' {
			bodyEnd = bodyEnd - 2
		}
		body := bodyStart[:bodyEnd]
		header := parseHeaderLines(headerBytes)
		parts = append(parts, &Part{Header: header, body: body})
		rest = bodyStart[nextIdx+len(delim):]
	}
	return parts
}

func (r *Reader) NextPart() (*Part, error) {
	if r.idx >= len(r.parts) {
		return nil, io.EOF
	}
	p := r.parts[r.idx]
	r.idx = r.idx + 1
	return p, nil
}
