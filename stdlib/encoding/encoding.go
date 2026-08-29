// encoding: the small marker interfaces every codec package's concrete
// types can implement, matching real Go exactly (this package is tiny in
// real Go too -- just four one/two-method interfaces, no functions).
package encoding

type BinaryMarshaler interface {
	MarshalBinary() ([]byte, error)
}

type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) error
}

type TextMarshaler interface {
	MarshalText() ([]byte, error)
}

type TextUnmarshaler interface {
	UnmarshalText(text []byte) error
}
