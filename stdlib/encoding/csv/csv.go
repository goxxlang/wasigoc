// RFC 4180 CSV, a real recursive-free field/record scanner (handles quoted
// fields with embedded commas/newlines and doubled-quote escaping), not a
// naive split-on-comma. Bounded deliberately: `Comma`/`Comment` are ASCII
// bytes (real Go's are runes -- multi-byte delimiters aren't supported, an
// uncommon case in practice), no `LazyQuotes`/`ReuseRecord` (this project's
// `wasigo::Slice` always makes a fresh backing array per `Read` anyway, so
// `ReuseRecord` would be a no-op even in real Go's sense of the word).
// Reader slurps its whole input up front via io.ReadAll rather than
// streaming -- same bounded-but-real shape as go/scanner working off a
// whole source string, not a byte-at-a-time io.Reader loop.
package csv

import (
	"errors"
	"io"
)

type Reader struct {
	Comma            byte
	Comment          byte
	FieldsPerRecord  int
	TrimLeadingSpace bool

	data []byte
	pos  int
}

func NewReader(r io.Reader) *Reader {
	data, _ := io.ReadAll(r)
	return &Reader{Comma: byte(44), data: data, pos: 0}
}

func (r *Reader) skipCommentsAndBlankLines() {
	for r.pos < len(r.data) {
		c := r.data[r.pos]
		if c == byte(13) {
			r.pos = r.pos + 1
			continue
		}
		if c == byte(10) {
			r.pos = r.pos + 1
			continue
		}
		if r.Comment != 0 && c == r.Comment {
			for r.pos < len(r.data) && r.data[r.pos] != byte(10) {
				r.pos = r.pos + 1
			}
			if r.pos < len(r.data) {
				r.pos = r.pos + 1
			}
			continue
		}
		break
	}
}

func (r *Reader) parseUnquotedField() string {
	start := r.pos
	for r.pos < len(r.data) {
		c := r.data[r.pos]
		if c == r.Comma || c == byte(10) || c == byte(13) {
			break
		}
		r.pos = r.pos + 1
	}
	return string(r.data[start:r.pos])
}

func (r *Reader) parseQuotedField() (string, error) {
	r.pos = r.pos + 1
	var out []byte
	for {
		if r.pos >= len(r.data) {
			return "", errors.New("csv: unexpected end of file in quoted field")
		}
		c := r.data[r.pos]
		if c == byte(34) {
			if r.pos+1 < len(r.data) && r.data[r.pos+1] == byte(34) {
				out = append(out, byte(34))
				r.pos = r.pos + 2
				continue
			}
			r.pos = r.pos + 1
			break
		}
		out = append(out, c)
		r.pos = r.pos + 1
	}
	return string(out), nil
}

func (r *Reader) parseField() (string, error) {
	if r.TrimLeadingSpace {
		for r.pos < len(r.data) && r.data[r.pos] == byte(32) {
			r.pos = r.pos + 1
		}
	}
	if r.pos < len(r.data) && r.data[r.pos] == byte(34) {
		return r.parseQuotedField()
	}
	return r.parseUnquotedField(), nil
}

// Read reads one record (a slice of fields), skipping blank lines and any
// comment lines. Returns io.EOF (as the error, with a nil record) once the
// input is exhausted.
func (r *Reader) Read() ([]string, error) {
	r.skipCommentsAndBlankLines()
	if r.pos >= len(r.data) {
		return nil, io.EOF
	}

	var fields []string
	for {
		field, err := r.parseField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)

		if r.pos >= len(r.data) {
			break
		}
		c := r.data[r.pos]
		if c == r.Comma {
			r.pos = r.pos + 1
			continue
		}
		if c == byte(13) {
			r.pos = r.pos + 1
			if r.pos < len(r.data) && r.data[r.pos] == byte(10) {
				r.pos = r.pos + 1
			}
			break
		}
		if c == byte(10) {
			r.pos = r.pos + 1
			break
		}
		return nil, errors.New("csv: extraneous or missing \" in quoted-field")
	}

	if r.FieldsPerRecord == 0 {
		r.FieldsPerRecord = len(fields)
	} else if r.FieldsPerRecord > 0 && len(fields) != r.FieldsPerRecord {
		return nil, errors.New("csv: wrong number of fields")
	}
	return fields, nil
}

// ReadAll reads every remaining record.
func (r *Reader) ReadAll() ([][]string, error) {
	var out [][]string
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				return out, nil
			}
			return nil, err
		}
		out = append(out, rec)
	}
}

type Writer struct {
	Comma   byte
	UseCRLF bool

	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{Comma: byte(44), w: w}
}

func (w *Writer) fieldNeedsQuotes(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == w.Comma || c == byte(34) || c == byte(10) || c == byte(13) {
			return true
		}
	}
	if s[0] == byte(32) || s[len(s)-1] == byte(32) {
		return true
	}
	return false
}

func (w *Writer) quoteField(s string) string {
	out := []byte{byte(34)}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == byte(34) {
			out = append(out, byte(34), byte(34))
		} else {
			out = append(out, c)
		}
	}
	out = append(out, byte(34))
	return string(out)
}

// Write writes one record, quoting fields that need it.
func (w *Writer) Write(record []string) error {
	for i := 0; i < len(record); i++ {
		if i > 0 {
			_, err := w.w.Write([]byte{w.Comma})
			if err != nil {
				return err
			}
		}
		s := record[i]
		if w.fieldNeedsQuotes(s) {
			s = w.quoteField(s)
		}
		_, err := io.WriteString(w.w, s)
		if err != nil {
			return err
		}
	}
	nl := "\n"
	if w.UseCRLF {
		nl = "\r\n"
	}
	_, err := io.WriteString(w.w, nl)
	return err
}

// WriteAll writes every record.
func (w *Writer) WriteAll(records [][]string) error {
	for i := 0; i < len(records); i++ {
		err := w.Write(records[i])
		if err != nil {
			return err
		}
	}
	return nil
}
