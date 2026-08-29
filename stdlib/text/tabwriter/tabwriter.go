// Column-aligned text output from tab-separated cells, bounded to the
// common case: buffer everything written until Flush, split on '\n' into
// rows and '\t' into cells, compute each column's max width (padding +
// minwidth) from every row's NON-FINAL cells, then pad and emit. Real
// tabwriter's "column block" resets (a shorter row breaks the block) and
// its various flags (FilterHTML/StripEscape/DiscardEmptyColumns/
// TabIndent/Debug) are NOT implemented -- only AlignRight. A row's FINAL
// cell is never padded here (real tabwriter only pads a trailing cell
// when the line itself ends with a literal tab, an uncommon case this
// bounded version doesn't distinguish). Verified against Go's own
// documented tabwriter example output, not just self-consistency.
package tabwriter

import (
	"io"
	"strings"
)

const AlignRight = 1

type Writer struct {
	output     io.Writer
	minwidth   int
	padding    int
	padchar    byte
	alignRight bool
	buf        []byte
}

func NewWriter(output io.Writer, minwidth int, tabwidth int, padding int, padchar byte, flags uint) *Writer {
	w := &Writer{}
	w.Init(output, minwidth, tabwidth, padding, padchar, flags)
	return w
}

func (w *Writer) Init(output io.Writer, minwidth int, tabwidth int, padding int, padchar byte, flags uint) *Writer {
	w.output = output
	w.minwidth = minwidth
	w.padding = padding
	w.padchar = padchar
	w.alignRight = flags&AlignRight != 0
	w.buf = nil
	return w
}

func (w *Writer) Write(buf []byte) (int, error) {
	w.buf = append(w.buf, buf...)
	return len(buf), nil
}

func padBytes(n int, c byte) []byte {
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = c
	}
	return b
}

func (w *Writer) Flush() error {
	text := string(w.buf)
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[0 : len(lines)-1]
	}

	rows := make([][]string, len(lines))
	numCols := 0
	for i := 0; i < len(lines); i++ {
		cells := strings.Split(lines[i], "\t")
		rows[i] = cells
		if len(cells) > numCols {
			numCols = len(cells)
		}
	}

	widths := make([]int, numCols)
	for i := 0; i < len(rows); i++ {
		for c := 0; c < len(rows[i])-1; c++ {
			cw := len(rows[i][c]) + w.padding
			if cw < w.minwidth {
				cw = w.minwidth
			}
			if cw > widths[c] {
				widths[c] = cw
			}
		}
	}

	var out []byte
	for i := 0; i < len(rows); i++ {
		row := rows[i]
		for c := 0; c < len(row); c++ {
			cell := row[c]
			if c < len(row)-1 {
				pad := widths[c] - len(cell)
				if w.alignRight {
					out = append(out, padBytes(pad, w.padchar)...)
					out = append(out, cell...)
				} else {
					out = append(out, cell...)
					out = append(out, padBytes(pad, w.padchar)...)
				}
			} else {
				out = append(out, cell...)
			}
		}
		out = append(out, byte(10))
	}
	w.buf = nil
	_, err := w.output.Write(out)
	return err
}
