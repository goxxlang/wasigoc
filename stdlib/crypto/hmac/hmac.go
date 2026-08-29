// HMAC (RFC 2104), built directly on the three concrete hash packages this
// project already has (crypto/md5, crypto/sha1, crypto/sha256) rather than a
// generic New(func() hash.Hash, key) constructor -- this project deliberately
// has no shared hash.Hash interface (see crypto/md5's package comment/the
// project's own notes: each digest type exposes Write/Sum/Reset directly,
// no interface). So HMAC is three concrete Sum functions, one per
// underlying hash, all sharing the same 64-byte block size (true for all
// three: MD5, SHA-1, and SHA-256 all use 64-byte blocks).
package hmac

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
)

func hmacCore(blockSize int, hashFn func([]byte) []byte, key []byte, message []byte) []byte {
	k := key
	if len(k) > blockSize {
		summed := hashFn(k)
		k = summed
	}
	padded := make([]byte, blockSize)
	for i := 0; i < len(k); i++ {
		padded[i] = k[i]
	}

	ipad := make([]byte, blockSize)
	opad := make([]byte, blockSize)
	for i := 0; i < blockSize; i++ {
		ipad[i] = padded[i] ^ 0x36
		opad[i] = padded[i] ^ 0x5c
	}

	inner := append(append([]byte{}, ipad...), message...)
	innerSum := hashFn(inner)
	outer := append(append([]byte{}, opad...), innerSum...)
	return hashFn(outer)
}

// SumMD5 computes HMAC-MD5(key, message).
func SumMD5(key []byte, message []byte) []byte {
	return hmacCore(64, func(b []byte) []byte { return md5.Sum(b) }, key, message)
}

// SumSHA1 computes HMAC-SHA1(key, message).
func SumSHA1(key []byte, message []byte) []byte {
	return hmacCore(64, func(b []byte) []byte { return sha1.Sum(b) }, key, message)
}

// SumSHA256 computes HMAC-SHA256(key, message).
func SumSHA256(key []byte, message []byte) []byte {
	return hmacCore(64, func(b []byte) []byte { return sha256.Sum(b) }, key, message)
}

// Equal reports whether two MACs are equal. NOT constant-time (this
// project has no crypto/subtle) -- fine for test/tooling use, not a
// substitute for a real constant-time comparison in a security-sensitive
// deployment.
func Equal(mac1 []byte, mac2 []byte) bool {
	if len(mac1) != len(mac2) {
		return false
	}
	for i := 0; i < len(mac1); i++ {
		if mac1[i] != mac2[i] {
			return false
		}
	}
	return true
}
