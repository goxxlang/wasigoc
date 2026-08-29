// Bounded crypto: the Hash identifier type real Go's own crypto package
// is built around -- MD5/SHA-1/SHA-256/SHA-512/SHA3-256 sizes and
// names -- without Hash.New() (that would import every concrete hash
// package from here and create an init-cycle with their RegisterHash
// calls). Callers construct a digest via crypto/sha256.New() etc.
// Hash is a struct, not `type Hash uint`, because methods here need a
// real struct receiver (same bound as time.Duration).
package crypto

type Hash struct {
	name string
	size int
}

var MD5 = Hash{name: "MD5", size: 16}
var SHA1 = Hash{name: "SHA-1", size: 20}
var SHA256 = Hash{name: "SHA-256", size: 32}
var SHA512 = Hash{name: "SHA-512", size: 64}
var SHA3_256 = Hash{name: "SHA3-256", size: 32}

func (h Hash) Size() int { return h.size }

func (h Hash) String() string { return h.name }

func (h Hash) Available() bool { return h.size > 0 }
