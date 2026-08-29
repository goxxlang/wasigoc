// Package structs: HostLayout marker type, matching real Go 1.23+'s
// own structs.HostLayout. A field of this type (conventionally named
// "_") marks the containing struct as using host memory layout. This
// compiler does not change layout based on it -- the type exists so
// source that names structs.HostLayout still compiles. The unexported
// hostLayout field prevents conversion from a plain struct{}.
package structs

type hostLayout struct{}

type HostLayout struct {
	unused hostLayout
}
