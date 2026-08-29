// hash.Hash/Hash32/Hash64: the interfaces the concrete hash packages
// (adler32, crc32, crc64, fnv, crypto/md5, crypto/sha1, crypto/sha256,
// crypto/sha512) already implement directly on their own Digest type.
// Hash32/Hash64 embedding Hash is this project's first interface-embeds-
// interface package -- previously untested (see README's stdlib tracker).
package hash

type Hash interface {
	Write(p []byte) (n int, err error)
	Sum(b []byte) []byte
	Reset()
	Size() int
	BlockSize() int
}

type Hash32 interface {
	Hash
	Sum32() uint32
}

type Hash64 interface {
	Hash
	Sum64() uint64
}
