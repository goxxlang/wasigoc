# Go++ language

Go++ is Go *syntax* with a C++/WASM *runtime*. The frontend is a real
(restricted) recursive-descent Go parser, including automatic semicolon
insertion. The subset is the set of constructs `wasigoc` accepts, not a
different grammar.

It is not `gc` and not a Go runtime port. wasm32-wasip1 is one thread
and has no growable stacks; wasi-sdk's noeh libc++ has no exceptions and
no RTTI. Forcing Go's M:N scheduler, tri-color GC, and racy memory model
onto that target would be a slow, unsafe Go-shaped C++ runtime.

The mapping is a **Rosetta**: keep the shape of the Go++ source, spell
each construct as the C++ feature that is actually strong on WASM.
`src/runtime.hpp` is that Rosetta. GC is Oilpan-lite (Blink/cppgc's C++
API), not a Go collector.

See `examples/rosetta/rosetta.go` for goroutines, channels,
`defer`/`panic`/`recover`, and `iota` compiled to `wasm32-wasip1`.

## Supported

```
package main
import "fmt"                 // also errors, os, reflect; everything else is stdlib/*.go
import alias "path"          // blank `_ "path"` runs init only; no `import .`

type Name struct {
  Field Type
  ...
}

func Name(param Type, ...) [Type | (name Type, ...)] { ... }
func (recv [*]Name) Method(param Type, ...) [Type | (Type, ...)] { ... }
func init() { ... }          // once per package, dependencies first, before main

type Name T                  // or `type Name = T` -- C++ `using` (not a distinct type)

var x [Type] [= expr]        // grouped: var ( x = 1; y = 2 )
const x [Type] [= expr]      // iota in a const group is folded to constexpr

if [SimpleStmt;] cond { ... } [else { ... } | else if ... ]
switch [SimpleStmt;] [tag] { case expr[, expr]: ... default: ... }
switch x := i.(type) { case T: ... case nil: ... default: ... }
select { case ch <- x: ... case v[, ok] := <-ch: ... default: ... }
for { ... }
for cond { ... }
for init; cond; post { ... }
for [k[, v]] := range expr { ... }   // slice, map, string (UTF-8 runes), chan
go expr
defer expr
ch <- x  /  <-ch  /  v, ok := <-ch
return [expr[, expr...]]
break [Label] / continue [Label] / goto Label
Label: stmt
x := expr                    // multi: a, b := expr, expr  /  a, b := f()  /  v, ok := m[k]  /  v, ok := i.(T)
x.(T)                        // must-form panics; comma-ok does not
append(s, t...)
x = expr  /  x += expr  (and -= *= /= %= &= |= ^= &^= <<= >>=)
x & y  /  x | y  /  x ^ y  /  x &^ y  /  x << y  /  x >> y  /  ^x
x++ / x--
```

Types: `bool`, `string`, `int`/`int8`/`int16`/`int32`/`int64`,
`uint`/`uint8`/`uint16`/`uint32`/`uint64`, `byte`, `rune`, `float32`,
`float64`, `error` (`wasigo::Error`), `any` / `interface{}`, a declared
`struct` or `interface`, `*T`, `[]T` (`wasigo::Slice<T>`), `map[K]V`
(`wasigo::Map<K,V>`), `chan T` (`wasigo::Chan<T>`), `[N]T`
(`std::array`), `func(...)`. `int`/`uint` are always 64-bit.

Builtins: `len`/`cap`/`append`/`copy`/`make`/`new`/`close`/`delete`/
`panic`/`recover`/`min`/`max`/`clear`, method values, array slicing
(copies into a `Slice`), numeric/`string` conversions.
`fmt.Print*`/`Sprint*`/`Printf`/`Sprintf` (format string must be a
**literal**; verbs `%d %s %f %v %t %c %%` only, no width/precision).
`errors.New` / `errors.Is` (`errors.New("")` is still non-nil).

Methods exist only on a real `struct` receiver. `type Duration int64` is
a C++ `using` — no method set. Value receivers are `const` and copy
`*this`. Pointer receivers mutate through `this`. Embedded interfaces
flatten into the outer vtable. Embedding is public C++ inheritance.

A function that does `<-` / `select` becomes a C++20 coroutine (`Task` /
`TaskT<T>`). `go` captures **by value**. Slice/map/chan nil and OOB
**panic** instead of UB.

## Rosetta

| Go++ | Not this | This |
| --- | --- | --- |
| `go f()` | pthread / wasi-threads | `wasigo::Task` / `TaskT<T>`, cooperative runqueue, data-race-free |
| `chan T` / `select` | condvars, nil-blocks-forever | `wasigo::Chan<T>`; nil send/recv **panics** |
| `defer` | try/finally | RAII `DeferList` (dtor LIFO) |
| `panic` / `recover` | C++ exceptions / setjmp | `PanicFrame` + `goto` the function epilogue. `recover()` only catches a panic in the **same** function. Runtime panics `abort`. |
| `error` | `std::string`, `""` is nil | `wasigo::Error` |
| `[]T` | `std::vector` (always copy) | `wasigo::Slice<T>`: shared backing, copy-on-grow, bounds-checked |
| `map[K]V` | ordered iteration | `wasigo::Map<K,V>` = `unordered_map`; nil vs empty preserved |
| `interface` / `any` | Go itables / RTTI | generated vtable + `adapt<T>` / `adapt_ptr`; `type_key_of<T>()` |
| func literals | `std::function` | `wasigo::Func`. Escaping literals that capture locals by reference can dangle — return a **method value** instead (see `stdlib/context`) |
| generics | Go 1.18 constraints | C++ templates (`[T any]` → `template<typename T>`) |
| `iota` | runtime enum | `constexpr`, folded at transpile time |
| GC | Go tri-color | Oilpan-lite: `GarbageCollected<T>`, `Member<T>`, `Persistent<T>`, STW mark-sweep |

## Modules

Each `.go` package keeps its `package` name as a C++ namespace
(`package geom` → `namespace geom`). `package main` stays at global
scope so `int main()` is the wasm start function. Generated headers
`#include` only their **direct** imports — never flattened.

Import resolution:

1. `./` / `../` relative to the importing file
2. `go.mod` `replace`
3. the importing file's directory
4. `go.mod` module prefix
5. each `--import-dir=DIR`
6. bundled `stdlib/`

`import "fmt"` / `"errors"` / `"os"` / `"reflect"` are builtins.
`internal/` is import-restricted like Go. Diamond imports parse once;
cycles are a hard error.

```
wasigoc examples/importpkg/main.go -o importpkg.cpp --out-dir gen/
wasigoc examples/modnest/main.go -o modnest.cpp --out-dir gen/
```

## Limits worth knowing

- No `complex128` (see `math/cmplx`: a `Complex` struct).
- No range-over-func, no `iter`.
- `recover()` does not unwind across calls.
- Ordinary func literals capture `[&]`; `go` captures by value.
- `fmt.Printf` format must be a string literal at the call site — no
  `log.Printf`-shaped wrappers.
- `uintptr` is not a builtin (`unsafe.Pointer` is `uint64`).
- `//go:embed` is not generated (`embed.FS` is an honest stub).
