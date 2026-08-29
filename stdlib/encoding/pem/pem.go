// RFC 1421-shaped PEM blocks ("-----BEGIN X-----" ... "-----END X-----"),
// bounded: header VALUES are single-line only (no RFC 1421 continuation-
// line folding for a long header value). Decode/Encode/EncodeToMemory.
package pem

import (
	"encoding/base64"
	"io"
	"strings"
)

type Block struct {
	Type    string
	Headers map[string]string
	Bytes   []byte
}

// Decode finds the first PEM block in data and returns it plus
// whatever comes after it; returns (nil, data) if no block is found.
func Decode(data []byte) (*Block, []byte) {
	s := string(data)
	beginMarker := "-----BEGIN "
	start := strings.Index(s, beginMarker)
	if start == -1 {
		return nil, data
	}
	afterBegin := start + len(beginMarker)
	endOfBeginLine := strings.Index(s[afterBegin:], "-----")
	if endOfBeginLine == -1 {
		return nil, data
	}
	blockType := s[afterBegin : afterBegin+endOfBeginLine]
	restStart := afterBegin + endOfBeginLine + len("-----")
	nl := strings.Index(s[restStart:], "\n")
	if nl == -1 {
		return nil, data
	}
	bodyStart := restStart + nl + 1

	endMarker := "-----END " + blockType + "-----"
	endIdx := strings.Index(s[bodyStart:], endMarker)
	if endIdx == -1 {
		return nil, data
	}
	body := s[bodyStart : bodyStart+endIdx]
	afterEnd := bodyStart + endIdx + len(endMarker)
	restOut := s[afterEnd:]
	restOut = strings.TrimPrefix(restOut, "\r\n")
	restOut = strings.TrimPrefix(restOut, "\n")

	lines := strings.Split(body, "\n")
	headers := make(map[string]string)
	var b64Lines []string
	inHeaders := true
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if inHeaders {
			if line == "" {
				inHeaders = false
				continue
			}
			colon := strings.Index(line, ":")
			if colon == -1 {
				inHeaders = false
				b64Lines = append(b64Lines, line)
				continue
			}
			key := strings.TrimSpace(line[0:colon])
			val := strings.TrimSpace(line[colon+1:])
			headers[key] = val
			continue
		}
		if line != "" {
			b64Lines = append(b64Lines, line)
		}
	}
	joined := strings.Join(b64Lines, "")
	decoded, err := base64.StdEncoding.DecodeString(joined)
	if err != nil {
		return nil, data
	}
	return &Block{Type: blockType, Headers: headers, Bytes: decoded}, []byte(restOut)
}

func EncodeToMemory(b *Block) []byte {
	var out []byte
	out = append(out, []byte("-----BEGIN "+b.Type+"-----\n")...)
	for k, v := range b.Headers {
		out = append(out, []byte(k+": "+v+"\n")...)
	}
	if len(b.Headers) > 0 {
		out = append(out, byte(10))
	}
	encoded := base64.StdEncoding.EncodeToString(b.Bytes)
	for i := 0; i < len(encoded); i = i + 64 {
		end := i + 64
		if end > len(encoded) {
			end = len(encoded)
		}
		out = append(out, []byte(encoded[i:end])...)
		out = append(out, byte(10))
	}
	out = append(out, []byte("-----END "+b.Type+"-----\n")...)
	return out
}

func Encode(out io.Writer, b *Block) error {
	_, err := out.Write(EncodeToMemory(b))
	return err
}
