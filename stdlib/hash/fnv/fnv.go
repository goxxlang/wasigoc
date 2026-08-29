// FNV-1 and FNV-1a, 32- and 64-bit (no 128-bit -- that needs a >64-bit
// integer type this compiler doesn't have). Each returns a concrete
// pointer type with the usual hash.Hash-shaped methods (Write/Sum/Reset/
// Size/BlockSize) plus Sum32()/Sum64() -- there's no hash.Hash/Hash32/
// Hash64 interface package here (untested territory: an interface
// embedding another interface), so callers use the concrete type
// directly, same as calling fnv.New32() and using its methods without
// ever naming hash.Hash32. The digest types are named Digest32/Digest32a/
// Digest64/Digest64a rather than Go's own unexported sum32/sum32a/sum64/
// sum64a (which would need real Go's hash.Hash32 interface to be usable
// outside this package) -- and specifically NOT "Sum32"/"Sum64", which
// would collide with the method of the same name and, under this
// compiler, get parsed as a same-named-as-its-class C++ member function
// (i.e. a constructor) instead of an ordinary method.
package fnv

const (
	offset32 = 2166136261
	prime32  = 16777619
)

type Digest32 struct {
	hash uint32
}

func New32() *Digest32 {
	return &Digest32{hash: offset32}
}

func (s *Digest32) Write(data []byte) (int, error) {
	h := s.hash
	for i := 0; i < len(data); i++ {
		h *= prime32
		h ^= uint32(data[i])
	}
	s.hash = h
	return len(data), nil
}

func (s *Digest32) Sum32() uint32  { return s.hash }
func (s *Digest32) Reset()         { s.hash = offset32 }
func (s *Digest32) Size() int      { return 4 }
func (s *Digest32) BlockSize() int { return 1 }
func (s *Digest32) Sum(b []byte) []byte {
	v := s.hash
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

type Digest32a struct {
	hash uint32
}

func New32a() *Digest32a {
	return &Digest32a{hash: offset32}
}

func (s *Digest32a) Write(data []byte) (int, error) {
	h := s.hash
	for i := 0; i < len(data); i++ {
		h ^= uint32(data[i])
		h *= prime32
	}
	s.hash = h
	return len(data), nil
}

func (s *Digest32a) Sum32() uint32  { return s.hash }
func (s *Digest32a) Reset()         { s.hash = offset32 }
func (s *Digest32a) Size() int      { return 4 }
func (s *Digest32a) BlockSize() int { return 1 }
func (s *Digest32a) Sum(b []byte) []byte {
	v := s.hash
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

const (
	offset64 = 14695981039346656037
	prime64  = 1099511628211
)

type Digest64 struct {
	hash uint64
}

func New64() *Digest64 {
	return &Digest64{hash: offset64}
}

func (s *Digest64) Write(data []byte) (int, error) {
	h := s.hash
	for i := 0; i < len(data); i++ {
		h *= prime64
		h ^= uint64(data[i])
	}
	s.hash = h
	return len(data), nil
}

func (s *Digest64) Sum64() uint64  { return s.hash }
func (s *Digest64) Reset()         { s.hash = offset64 }
func (s *Digest64) Size() int      { return 8 }
func (s *Digest64) BlockSize() int { return 1 }
func (s *Digest64) Sum(b []byte) []byte {
	v := s.hash
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

type Digest64a struct {
	hash uint64
}

func New64a() *Digest64a {
	return &Digest64a{hash: offset64}
}

func (s *Digest64a) Write(data []byte) (int, error) {
	h := s.hash
	for i := 0; i < len(data); i++ {
		h ^= uint64(data[i])
		h *= prime64
	}
	s.hash = h
	return len(data), nil
}

func (s *Digest64a) Sum64() uint64  { return s.hash }
func (s *Digest64a) Reset()         { s.hash = offset64 }
func (s *Digest64a) Size() int      { return 8 }
func (s *Digest64a) BlockSize() int { return 1 }
func (s *Digest64a) Sum(b []byte) []byte {
	v := s.hash
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
