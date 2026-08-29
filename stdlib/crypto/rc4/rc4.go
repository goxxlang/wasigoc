// RC4 (ARCFOUR) -- present for legacy interop only, like this project's
// crypto/sha1: it's a broken stream cipher by modern standards, never
// use it for anything that needs real confidentiality. `s` is a []byte
// slice, not Go's fixed-size [256]byte array type (this project prefers
// slices for exactly this shape everywhere else, see hash/crc32's own
// header comment). Verified against 3 standard RC4 test vectors
// (Key/Plaintext, Wiki/pedia, Secret/"Attack at dawn"), cross-checked
// independently via a from-scratch Python implementation first.
package rc4

import "errors"

type Cipher struct {
	s []byte
	i byte
	j byte
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) < 1 || len(key) > 256 {
		return nil, errors.New("crypto/rc4: invalid key size")
	}
	c := &Cipher{s: make([]byte, 256)}
	for i := 0; i < 256; i++ {
		c.s[i] = byte(i)
	}
	var j byte = 0
	for i := 0; i < 256; i++ {
		j = j + c.s[i] + key[i%len(key)]
		c.s[i], c.s[j] = c.s[j], c.s[i]
	}
	return c, nil
}

func (c *Cipher) XORKeyStream(dst []byte, src []byte) {
	i := c.i
	j := c.j
	for k := 0; k < len(src); k++ {
		i = i + 1
		j = j + c.s[i]
		c.s[i], c.s[j] = c.s[j], c.s[i]
		dst[k] = src[k] ^ c.s[byte(c.s[i]+c.s[j])]
	}
	c.i = i
	c.j = j
}

func (c *Cipher) Reset() {
	for i := 0; i < len(c.s); i++ {
		c.s[i] = 0
	}
	c.i = 0
	c.j = 0
}
