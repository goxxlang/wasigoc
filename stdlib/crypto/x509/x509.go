// Bounded crypto/x509: ParseCertificate walks a DER-encoded
// Certificate SEQUENCE far enough to pull the serial INTEGER out of
// TBSCertificate -- the same "header reader" scope as debug/pe. No
// signature check, no SAN/AKID, no chain Verify (needs crypto/x509
// policies plus a time source and usually crypto/ecdsa over a parsed
// SPKI, a much larger surface). Subject is left empty.
package x509

import (
	"crypto/x509/pkix"
	"errors"
	"math/big"
)

var ErrFormat = errors.New("x509: not a valid DER certificate")

type Certificate struct {
	Raw          []byte
	SerialNumber *big.Int
	Subject      pkix.Name
}

func readTLV(b []byte, off int) (int, int, int, int, error) {
	if off >= len(b) {
		return 0, 0, 0, 0, ErrFormat
	}
	tag := int(b[off])
	off = off + 1
	if off >= len(b) {
		return 0, 0, 0, 0, ErrFormat
	}
	lbyte := int(b[off])
	off = off + 1
	length := 0
	if lbyte < 128 {
		length = lbyte
	} else {
		n := lbyte & 127
		if n == 0 || n > 4 || off+n > len(b) {
			return 0, 0, 0, 0, ErrFormat
		}
		i := 0
		for i < n {
			length = (length << 8) | int(b[off])
			off = off + 1
			i = i + 1
		}
	}
	if off+length > len(b) {
		return 0, 0, 0, 0, ErrFormat
	}
	return tag, length, off, off + length, nil
}

func ParseCertificate(der []byte) (*Certificate, error) {
	tag, _, val, next, err := readTLV(der, 0)
	if err != nil || tag != 0x30 || next != len(der) {
		return nil, ErrFormat
	}
	tag, _, tbs, _, err := readTLV(der, val)
	if err != nil || tag != 0x30 {
		return nil, ErrFormat
	}
	off := tbs
	if off < len(der) && der[off] == 0xa0 {
		_, _, _, off, err = readTLV(der, off)
		if err != nil {
			return nil, ErrFormat
		}
	}
	tag, ln, voff, _, err := readTLV(der, off)
	if err != nil || tag != 0x02 {
		return nil, ErrFormat
	}
	serial := new(big.Int).SetBytes(der[voff : voff+ln])
	return &Certificate{Raw: der, SerialNumber: serial}, nil
}
