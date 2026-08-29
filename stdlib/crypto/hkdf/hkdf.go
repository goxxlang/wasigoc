// HKDF-SHA256 (RFC 5869). Extract+Expand as two concrete functions
// rather than a generic hash.Hash constructor -- same bound as this
// project's crypto/hmac (SumSHA256, no New(func() hash.Hash)).
package hkdf

import "crypto/hmac"

func Extract(salt []byte, ikm []byte) []byte {
	s := salt
	if len(s) == 0 {
		s = make([]byte, 32)
	}
	return hmac.SumSHA256(s, ikm)
}

func Expand(prk []byte, info []byte, length int) []byte {
	var out []byte
	var prev []byte
	i := byte(1)
	for len(out) < length {
		msg := append(append([]byte{}, prev...), info...)
		msg = append(msg, i)
		prev = hmac.SumSHA256(prk, msg)
		out = append(out, prev...)
		i = i + 1
	}
	return out[0:length]
}

func Sum(secret []byte, salt []byte, info []byte, length int) []byte {
	return Expand(Extract(salt, secret), info, length)
}
