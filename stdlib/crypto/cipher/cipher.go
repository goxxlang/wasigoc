// Bounded subset of crypto/cipher: just the `Block` interface (the shape
// every block cipher -- crypto/des, crypto/aes -- implements directly).
// No `Stream`/`AEAD`/`BlockMode` (CBC/CTR/GCM/etc. chaining modes) --
// callers work one block at a time, same bounded-scope precedent as
// `hash.Hash` not including a generic streaming wrapper either.
package cipher

type Block interface {
	BlockSize() int
	Encrypt(dst []byte, src []byte)
	Decrypt(dst []byte, src []byte)
}
