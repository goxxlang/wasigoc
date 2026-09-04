# Design log (historical)

This is the original in-tree writeup: language notes, compiler-bug
diary, and the per-package stdlib tracker. The public docs start at
[`README.md`](../README.md).

---

# WASIGo++

**Go++** is the name for the language this project actually implements: Go
syntax and subset semantics, but with deliberate, deep departures from real
Go's own runtime model -- Oilpan (not a Go GC) for memory management, and an
interned, pointer-identity type representation (`go/types.Type`'s "Object
Type Identifier" design, see the Stdlib section) for type isolation -- so
it's its own thing, not a Go clone, compiled by **`wasigoc`**.

A subset-of-Go-to-C++ compiler (`wasigoc`) for [wasi-sdk](https://github.com/WebAssembly/wasi-sdk):
feed it a real (restricted) `.go` file and it emits a single, complete C++
translation unit -- struct definitions with their methods inlined, free
functions, a real `int main()`, and an inlined copy of `src/runtime.hpp` --
meant for wasi-sdk's **`wasm32-wasip1-clang++`** (see "Compile to wasm"
below; a bare `clang++ --target=wasm32-wasip1` is not enough on current
wasi-sdk). Unlike this repo's siblings [WASMBruja](../WASMBruja) (`.bruja`
Web IDL -> a quickjs binding *header*, for some larger hand-written program
to implement) and [WASMVoodooCompile](../WASMVoodooCompile) (`.voodoom`
mojom-like IDL -> Mojo `Proxy_`/`Stub_` *headers*): `wasigoc` compiles a
whole program, not a library.

```
Go source (.go)  ->  wasigoc  ->  generated C++ (.cpp)  ->  wasm32-wasip1-clang++  ->  .wasm
```

`wasigoc` itself is a host tool (MSVC/clang on the machine). Only the
generated C++ targets WASM.

Unlike WASMBruja/WASMVoodooCompile's own input languages -- both bespoke IDL
grammars invented for those tools -- `wasigoc`'s input is real Go source,
lexed and parsed by a real (if restricted) recursive-descent Go frontend,
including Go's automatic semicolon insertion (see `src/lexer.cc`'s
`EndsStatement`) so ordinary, un-doctored Go code lexes correctly. What makes
this a *subset* compiler rather than a full Go compiler is the set of
constructs it accepts -- see "What's supported" below -- not a departure from
Go's own syntax.

**What this is not.** It is not `gc` and not a Go runtime port. wasm32-wasip1
is one thread and has no growable stacks; wasi-sdk's noeh libc++ has no
exceptions, no RTTI, and `setjmp` needs wasm EH. Forcing Go's M:N scheduler,
tri-color GC, and racy memory model onto that target would be a slow, unsafe
Go-shaped C++ runtime. The design is a **Rosetta**: keep the *shape* of the
Go source, spell each construct as the C++ feature that is actually strong
on WASM (see the table below). GC is Oilpan (Blink/cppgc's C++ API), not a
Go collector. Slice/map/chan nil and OOB **panic** instead of UB; `go`
captures by value; cooperative scheduling means no data races and no
wasi-threads. The noeh include order (see "Compile to wasm") is load-bearing
-- a bare `clang++ --target=wasm32-wasip1` will not compile the output.

## What's supported

```
package main
import "fmt"                 // also errors, os; stdlib strings/strconv/bytes/sort/path/filepath/unicode/utf8/math/io/time
import alias "path"          // blank `_ "path"` runs init only; no `import .`

type Name struct {
  Field Type
  ...
}

func Name(param Type, ...) [Type | (name Type, ...)] { ... }  // named results: bare `return` works
func (recv [*]Name) Method(param Type, ...) [Type | (Type, ...)] { ... }
func init() { ... }          // runs once per package, dependencies first, before main

type Name T                  // or `type Name = T` -- C++ `using` (not a distinct defined type)

var x [Type] [= expr]        // and grouped: var ( x = 1; y = 2 )
const x [Type] [= expr]      // iota in a const group is folded to constexpr

if [SimpleStmt;] cond { ... } [else { ... } | else if ... ]
switch [SimpleStmt;] [tag] { case expr[, expr]: ... default: ... }
switch x := i.(type) { case T: ... case nil: ... default: ... }
select { case ch <- x: ... case v[, ok] := <-ch: ... default: ... }
for { ... }
for cond { ... }
for init; cond; post { ... }
for [k[, v]] := range expr { ... }   // slice, map, string (UTF-8 runes), chan (until closed)
go expr
defer expr
ch <- x  /  <-ch  /  v, ok := <-ch
return [expr[, expr...]]
break [Label] / continue [Label] / goto Label
Label: stmt
x := expr                    // and multi: a, b := expr, expr  /  a, b := f()  /  v, ok := m[k]  /  v, ok := i.(T)
x.(T)                        // type assert (must form panics; comma-ok does not)
append(s, t...)              // unpack a slice (or string into []byte)
x = expr  /  x += expr  (and -= *= /= %= &= |= ^= &^= <<= >>=)
x & y  /  x | y  /  x ^ y  /  x &^ y  /  x << y  /  x >> y  /  ^x
x++ / x--
```

Types: `bool`, `string`, `int`/`int8`/`int16`/`int32`/`int64`,
`uint`/`uint8`/`uint16`/`uint32`/`uint64`, `byte` (alias for `uint8`), `rune`
(alias for `int32`), `float32`, `float64`, `error` (-> `wasigo::Error`),
`any` / `interface{}`, a declared `struct` or `interface` name, `*T`,
`[]T` (-> `wasigo::Slice<T>`), `map[K]V` (-> `wasigo::Map<K,V>`),
`chan T` (-> `wasigo::Chan<T>`), `[N]T` (-> `std::array`), and `func(...)`.
`int`/`uint` are always emitted as 64-bit (`int64_t`/`uint64_t`), never the
platform-word-sized type real Go uses.

Builtins: `len`/`cap`/`append`/`copy`/`make`/`new`/`close`/`delete`/`panic`/
`recover`/`min`/`max`/`clear`, method values (`f := p.Sum`), array slicing
(`arr[1:3]` copies into a `Slice`), and the numeric/`string` conversions
(`int(x)`, `float64(x)`, `string(byteOrRuneOrBytes)`, ...).
`fmt.Print`/`Println`/`Sprint`/`Sprintln`/`Printf`/`Sprintf` (`Printf`/`Sprintf`
format string must be a string *literal* -- it's parsed at compile time --
and only `%d %s %f %v %t %c %%` are recognized, with no width/precision), and
`errors.New(msg)` / `errors.Is(err, target)` (`wasigo::Error`; empty string is
still non-nil). Composite literals work for structs (`Point{X: 1, Y: 2}` or
positional `Point{1, 2}`), slices (`[]int{1, 2, 3}`), and maps
(`map[string]int{"a": 1}`). Embedded interfaces flatten into the outer vtable.

Struct methods are emitted *inside* the C++ struct body. Pointer receivers
are ordinary non-`const` members (`T* recv = this`); value receivers are
`const` and copy `*this` so mutations stay on the copy, matching Go. Free
functions get a forward-declared prototype up front specifically so
mutual/forward calls between them (e.g. `main`, conventionally written
first, calling a helper defined later in the file) work without reordering
the source -- see `cpp_generator.cc`'s `Run()` for the emission order
(struct-forward-decls, then free-func-prototypes, then full struct bodies,
then multi-return result structs, then globals, then free-func bodies).

## Rosetta: awkward Go → idiomatic WASM C++

wasm32-wasip1 is one thread and has no growable stacks. Forcing Go's
preemptive M:N scheduler, a Go collector, and racy memory model onto that
target is how you get a slow, unsafe Go-shaped C++ runtime. `src/runtime.hpp`
is the Rosetta instead: keep the *shape* of the Go source, spell each
construct as the C++ feature that is actually strong on WASM. GC is **Oilpan**
(Blink/cppgc's C++ API), not a Go GC clone.

See `examples/rosetta/rosetta.go` for a program that uses goroutines,
channels, `defer`/`panic`/`recover`, and `iota` and still compiles to
`wasm32-wasip1`.

| Go | Not this (awkward parity) | This (C++ / WASM strength) |
| --- | --- | --- |
| `go f()` | pthread / wasi-threads / growable stacks | `wasigo::Task` (void) / `wasigo::TaskT<T>` (returns T). C++20 coroutine + cooperative runqueue. Data-race-free by construction. |
| `chan T` / `select` | condvars, nil-chan-blocks-forever | `wasigo::Chan<T>`, `co_await ch.send/recv`, `GSelect` returns the ready case index. Nil send/recv **panics** (a forever-block cannot be preempted on WASI). |
| `defer f()` | try/finally sugar | RAII `DeferList` (dtor LIFO). |
| `panic` / `recover` | C++ exceptions / setjmp (wasi-sdk noeh has neither) | `PanicFrame` + `goto` the function epilogue so `defer`/`recover` run. Runtime panics (OOB, nil map) still `abort`. |
| `error` | `std::string`, `""` means nil | `wasigo::Error` (`errors.New("")` is **non-nil**). |
| `[]T`, `cap`, slicing | `std::vector` (always copy) | `wasigo::Slice<T>`: shared backing, copy-on-grow, bounds-checked. |
| `map[K]V` | ordered iteration | `wasigo::Map<K,V>` = `unordered_map` (Go's order is unspecified anyway). Nil vs empty preserved; assign-to-nil panics. |
| `interface` / `interface{}` / `any` | Go itables / RTTI | generated vtable + `adapt<T>` (boxes a copy) / `adapt_ptr(T*)`; `any` is `wasigo::Any`. `type_key_of<T>()` (no RTTI, wasi-sdk noeh has none). `x.(T)` is `must_cast` / comma-ok is `try_cast`. `recover()` is `wasigo::Recovered`, not `Any`. Embedded interfaces flatten methods into the outer vtable. |
| func literals / method values | heap escape analysis / `std::function` | `wasigo::Func` (virtual erasure; `<functional>` pulls a broken `<cctype>` on noeh). `go` captures **by value** so a task cannot dangle on the spawner's stack. Method values (`p.Sum`) are capturing lambdas. |
| generics | Go 1.18 constraints | C++ templates (`[T any]` → `template<typename T>`). C++ is strictly stronger here. |
| `iota` | runtime enum | `constexpr` ints, folded at transpile time. |
| embedding | hidden field + promotion | public C++ inheritance; promotion falls out. |
| value receiver | mutating `this` | `const` method + **copy** of `*this` (Go mutation isolation). |
| grouped params, `...T` | rejected | parsed; variadic becomes `Slice<T>`. |
| `make` / `new` / `close` / `copy` / `delete` | missing | `make_slice` / `make_map` / `make_chan`, `New`, `close`, `copy`, `del`. |
| GC | Go tri-color / write barriers / growable stacks | C++ Oilpan-lite (`wasigo::gc`): `GarbageCollected<T>`, `Member<T>` (on-heap edges), `Persistent<T>` (off-heap roots), `Trace(Visitor&)`. Stop-the-world mark-sweep on an explicit grey stack (WASM has no growable stacks, one thread so no concurrent mark). Slice/Map stay `shared_ptr` until the generator emits `Trace`. |

A function that does `<-` / `select` becomes a C++20 coroutine: `void` maps
to `wasigo::Task`, a result `T` maps to `wasigo::TaskT<T>` (`co_await` the
call). See `examples/asyncval`. `string(x)` still only accepts a string, a
byte/rune, or `[]byte`; use `fmt.Sprintf("%d", x)` to format a number.

## Imports and modules (WASMVoodooCompile shape)

Same rule as `voodoomc`: **do not flatten the graph into the entry file's
namespace, and do not flatten the include graph either.** Each `.go` package
keeps its `package` name; that name becomes a C++ namespace (`package geom`
→ `namespace geom { ... }`; a dotted `package a.b` would nest
`namespace a { namespace b { ... } }`, the same way voodoo's
`module a.b.c;` nests). The entry file (`package main`) stays at global
scope so `int main()` is the wasm start function.

voodoo's per-file `module` statement is two Go things:

| WASMVoodooCompile | WASIGo++ |
| --- | --- |
| `module a.b.c;` (file-level, **is** the C++ namespace) | `package` clause → C++ namespace; `go.mod`'s `module` path → **import resolution** |
| nested enum/const inside a struct/interface (mangled `Foo_Status`, not a true C++ nested type -- emission order across kinds) | Go has no nested type decls; embedding is public C++ inheritance. Nested **packages** (`shape/palette`) are separate namespaces + headers |
| each header `#include`s only its **direct** imports | same: `palette_gen.hpp` includes `shape_gen.hpp`; the entry includes only *its* direct imports |

`import "fmt"` / `import "errors"` / `import "os"` are builtins (not files).
Any other `import "path"` is resolved in this order:

1. `./` / `../` relative to the importing file (Go relative imports)
2. `go.mod` `replace` (`old => ./rel` is filesystem-relative to the module root)
3. the importing file's directory (voodoo `#include "..."` search, so
   `import "../geom"` and a sibling `"geom"` still work)
4. `go.mod` module prefix: walking up from the entry finds `go.mod`;
   `import "example.com/app/shape"` with `module example.com/app` is
   `<module-root>/shape`
5. each `--import-dir=DIR`, in order
6. the bundled `stdlib/` (compiled into `wasigoc` as `WASIGO_STDLIB_PATH`)

A `go.mod` is optional. `replace (` blocks and one-line `replace A => B` are
parsed; `require` is ignored (no network fetch). A package under
`.../internal/...` is only importable from the parent of `internal`, same as
Go.

If the path names a `.go` file, that file is the package. Otherwise `.go` is
tried as a suffix, then a **directory** of `*.go` files (skipping `*_test.go`)
is merged into one package / one C++ namespace / one generated header -- Go's
"all files in a directory are one package". A diamond import is parsed once.
An import cycle is a hard error naming the chain.

Generated C++: one `#pragma once` header per imported package, each
`#include`-ing only its direct imports (voodoo v15). Cross-file names in Go
source stay package-qualified (`shape.NewPoint`); codegen emits
`shape::NewPoint` -- never `using namespace`. See `examples/geom/` +
`examples/importpkg/main.go`, `examples/modnest/` (`go.mod` + nested
`shape/palette`), and `stdlib/strings` etc.

```
wasigoc examples/importpkg/main.go -o importpkg.cpp --out-dir gen/
# writes gen/geom_gen.hpp (namespace geom; point.go + origin.go merged)
# writes importpkg.cpp (#include "geom_gen.hpp", calls geom::NewPoint)

wasigoc examples/modnest/main.go -o modnest.cpp --out-dir gen/
# walks to examples/modnest/go.mod (`module example.com/app`)
# writes gen/shape_gen.hpp, gen/palette_gen.hpp (palette includes shape)
# writes modnest.cpp (#include "shape_gen.hpp" and "palette_gen.hpp")
```

Bundled stdlib: see **Stdlib** below (pickup toward the full Go standard
library). `fmt`/`errors`/`os` are builtins; everything else under `stdlib/`
is ordinary `.go`. `func init()` in an imported package runs (once, in
dependency order) before `main`.

Struct-by-value fields, and any struct referenced from another function's
multi-return result, must already be fully declared earlier in the file --
see `Run()`'s pass ordering. A pointer/slice/map/chan field can reference a
struct declared later (forward declaration covers that).

## Stdlib (pickup: the full Go standard library)

**Goal:** the public Go standard library (`go list std`, on the order of 150
packages), compiled as ordinary `.go` under `stdlib/` the same way
`strings` already is. Do **not** add compiler builtins unless the package
must touch WASI or the Rosetta runtime (`os` file fds, `time.Now`, clocks,
entropy). One thread / no growable stacks still applies: `sync` is a no-op
or cooperative, `time.Sleep` does not block the runqueue, `net` waits on
WASI sockets, `plugin`/`cgo`/`race` do not map.

**Where we are (2026-08-29):** 4 builtins (fmt/errors/os/reflect) + the
public `go list std` minus `internal/`/`vendor/`/`n/a` packages, including
the last 26 (crypto family fill-in, debug/dwarf+gosym, embed, asn1/gob,
go/build+importer, math/cmplx, testing/quick+slogtest, structs, unsafe,
syscall). See the tracker. Enough for the goldens (print, Atoi, Join, sort, Builder,
`io.Writer`/`Copy`/`ReadAll`, generics-based `slices`/`maps`/`cmp`,
`container/list`/`ring`/`heap`, `sync`+`sync/atomic`, `unicode`+`utf16`,
`math/bits`+`rand`, `encoding/hex`/`base64`/`binary`/`json`, `log`, `bufio`,
`flag`, cooperative `context`, `net/url`, wrapped errors, a real `os.File`,
`time.Time`, a real backtracking `regexp` engine, a stubbed `os/exec`) plus
much deeper `strings`/`strconv`/`bytes`/`path`/`sort`/`math`/`unicode/utf8`.
Real `strings` alone is still larger than this entire tree. `net`/
`net/http` cannot exist here at all (WASI preview 1 has no sockets --
verified directly in wasi-libc's own header, see the writeup below); `net`
is a deliberate stub. `reflect` and arbitrary-struct `encoding/json.Marshal`
are both done (Unmarshal into a struct still isn't -- that needs
settable/addressable reflect Values, a bigger feature than read-only
field access). A real Go source pipeline exists too now: `go/token` +
`go/scanner` (a genuine tokenizer, including automatic semicolon
insertion) + `go/ast` + `go/parser` (real recursive-descent parsing with
correct operator precedence -- expressions, statements including
`switch`/range-for, `var`/`const` decls, pointer/slice/map types,
composite literals) + `go/printer`/`go/format` (re-emit a Node tree as
Go source, gofmt-style round trip) + `go/types` (an expression AND
statement type checker -- `var`/assign/if/for, catches real mismatches
-- over an *interned* type representation, see the compiler-bugs
writeup) + `go/version` -- a bounded but real subset of Go's own grammar
and semantics, not the whole language, plus `go/build/constraint` (build
tag expression parsing/evaluation). `go/build`/`go/constant`/`go/doc`/
`go/importer` are still todo.

Along the way, exercising patterns no earlier stdlib code had (generics,
`&T{...}` pointer literals, struct fields typed `any`, methods on a
defined-slice type, a package literally named `log`) found and fixed several
previously-latent core `wasigoc` bugs -- each verified with a golden that
compiles through the real wasi-sdk clang++ to a well-formed wasm module, and
(as of 2026-08-28, `wasmtime` is installed and wired into every `_golden`
test -- see "Build" below) actually *run* under `wasmtime` with real stdout
checked against the expected output, not just compiled and run against a
host g++ by hand as a stand-in:

- A method whose body declares a local var from a receiver-derived
  expression (`z := r.field`) crashed codegen: the async-detection pre-pass
  never put the receiver in scope before analyzing the body.
- A generic function's return type wasn't substituted at the call site
  (`slices.Max`, `maps.Keys`) -- fixed by unifying the call's argument types
  against the declared param types to recover a `{T: concrete-type}` map.
- `for x := a; cond; x = y.f {` could misparse its post-clause as a
  composite literal (`y.f{`) swallowing the loop body -- composite literals
  need suppressing across the *whole* classic-for header, not just `cond`.
- `&T{...}` took the address of a C++ temporary -- ill-formed C++, and (for
  the field-setting form, which a permissive compiler did accept) a
  dangling pointer once the full expression ended. Now heap-allocates via
  `new`, the way the `new()` builtin already did.
- A cross-package `func New() *T` / a cross-package call passing an argument
  through an interface parameter both emitted the bare unqualified type
  name instead of `pkg::Name`.
- A struct with an `any` field got an auto-generated `operator==` that
  doesn't compile (`wasigo::Any` has none).
- `fmt.Print*`/`any` both printed a `bool` as C++'s `1`/`0` instead of Go's
  `true`/`false` (`Any` now carries a per-adapted-type print function, since
  there's no RTTI to ask "does my boxed value support `<<`" later).
- An interface's generated vtable-adapter lambda always named its self
  pointer `s`, colliding with a real Go method parameter also named `s`.
- `type X []T` / `type X map[K]V` (a C++ `using` alias, not a distinct
  type) didn't support indexing, composite literals, or -- silently, no
  error -- methods; the last is now a hard error instead of the method
  quietly never being emitted anywhere in the program.
- `package log` as a bare `namespace log {` collides with global `::log`
  from `<cmath>`; MSVC hard-errors where a permissive compiler only warns.
- Multi-return brace-init narrowing (`byte` arithmetic promoted to
  `int64_t` then narrowed back) is a hard error under MSVC where a
  permissive compiler only warns -- a stdlib-source issue, not a compiler
  one, but worth knowing: cast explicitly (`byte(x - 48)`) rather than
  relying on implicit narrowing when a multi-return's declared type is
  smaller than the expression's.
- `err.Error()` on a plain builtin `error` (not a struct implementing the
  interface) failed to resolve -- `error` maps to `wasigo::Error`, which
  isn't a `StructDecl` reachable through the normal method-lookup path.
  Both type inference and codegen for method calls now special-case
  `error.Error()` directly to `.str()`.
- Elided nested composite-literal element types (`[]classRange{{48, 57}}`)
  don't parse ("expected an expression but found '{'") -- a real parser
  gap, left as-is (low priority, self-contained); write the element type
  explicitly instead (`[]classRange{classRange{48, 57}}`), which gopls
  flags as a redundant-type lint hint but is otherwise harmless.
- **General bug, not stdlib-specific:** `map[K]V{}` (a composite literal
  with zero entries -- `MIMEHeader{}`, or any empty map literal) produced a
  *nil* map, not an empty one: `wasigo::Map<K,V>{}` with empty braces binds
  to the default constructor rather than the initializer_list one (C++'s
  "empty braces prefer the default constructor" overload-resolution rule),
  leaving the backing `shared_ptr` null. No earlier stdlib code had
  happened to write a zero-entry map literal (empty maps were always built
  via `make(map[K]V)` instead) so this was latent until `net/textproto`
  used `MIMEHeader{}`. Fixed in `EmitCompositeLit`: the zero-entry map case
  now routes through `wasigo::Map<K,V>::make()`.

- `QualifyResultType` (namespace-qualifies an unqualified cross-package
  result type at a `:=` unpack site) recursed through `Pointer` but not
  `Slice`/`Map` -- `func ParseAddressList(s string) ([]*Address, error)`
  called from outside package `mail` emitted `wasigo::Slice<Address*>`
  instead of `wasigo::Slice<mail::Address*>`, an unresolvable bare name.
  Now recurses through both.
- **Two distinct bugs, only found once real `wasmtime` execution (see
  below) could catch a runtime bug that only affects `wasm32-wasip1`
  specifically, not a host build:** a stateless lambda used as a
  `std::shared_ptr<FILE>` custom deleter, once its
  `__shared_ptr_pointer::__on_zero_shared()` got inlined at two or more
  call sites (e.g. `os.WriteFile`'s implicit close plus a later explicit
  `File.Close()`), could produce a `call_indirect` to an empty wasm
  function-table slot -- `wasm trap: uninitialized element`, deep inside
  libc's own `fclose`. Host g++ never saw this (it's wasm32-backend/table
  codegen specific), and it only showed up once `wasmtime` could actually
  execute the module instead of just checking it compiled. Worked around
  by replacing the custom-deleter-lambda `std::shared_ptr<FILE>` with a
  `std::shared_ptr<FileHandle>` where `FileHandle` is a plain struct with
  an ordinary (non-template, non-closure) destructor -- see `os_open`/
  `os_create`/`File` in `runtime.hpp`.
- **MSVC-specific, not caught by g++:** a struct field named the same as
  its own struct (Go allows this freely -- real `net/mail.Address` is
  `struct { Name string; Address string }`) breaks two different ways
  under MSVC (host build's actual compiler) that g++ accepts fine: (1)
  the auto-generated `operator==`/receiver-rebinding lines that spell the
  bare struct name as a *type* inside its own member function hit
  "`Address` does not name a type" (the field hides the injected-class-name
  in that context) -- fixed by namespace-qualifying the type name whenever
  this collision exists (`SelfTypeName`); (2) *any* `ptr->Address`/
  `obj.Address` field access anywhere in the program hits "type name
  cannot appear on the right side of a class member access expression" --
  MSVC parses the identifier after `->`/`.` as the type, not the field, in
  this situation, even though standard class-member lookup should prefer
  the field. Since qualifying the type doesn't fix a member-access
  expression, this one is fixed by mangling the C++-only identifier for
  just that field (`Address` -> `Address_` in the emitted C++, invisible
  to Go source and to `LookupField`'s matching) everywhere a struct field
  is declared, read, or written (`FieldCppName`).
- The lexer only supported decimal integer literals -- no `0x`/`0o`/`0b`.
  This had been a standing limitation the whole project, worked around by
  spelling constants via shifts/arithmetic, but crypto/hash work makes
  every constant a hex magic number from a spec (MD5's K table, SHA-256's
  round constants, CRC polynomials), where hand-converting 64 values to
  decimal is exactly the kind of transcription work that quietly
  introduces silent, hard-to-notice bugs. Added real `0x`/`0X`/`0o`/`0O`/
  `0b`/`0B` literal scanning (with `_` digit separators, matching decimal)
  -- integer literals only, no hex floats. Also switched the decimal-
  literal parse from `stoll` to `stoull` (bit-pattern-reinterpreted back
  into the token's `int64_t` field): a decimal literal past `INT64_MAX`
  but within `uint64` range (FNV's `14695981039346656037` offset basis)
  previously threw "stoll argument out of range" outright.
- **Not a compiler bug, a naming gotcha worth remembering:** a Go method
  named identically to its own receiver struct (`func (s *Sum32) Sum32()
  uint32`, an easy name to reach for on a hash-digest type) becomes,
  post-codegen, a C++ member function with the same name as its class --
  i.e. a constructor declaration, not an ordinary method ("return type
  specification for constructor invalid"). Unlike the field-name
  collision above, this one was avoided by simply renaming the struct
  (`hash/fnv`'s digest types are `Digest32`/`Digest32a`/`Digest64`/
  `Digest64a`, not `Sum32`/`Sum64`) rather than teaching the compiler to
  mangle same-named methods -- cheaper, and arguably better Go style
  anyway (real Go's own `fnv` package keeps its digest types unexported
  for exactly this kind of reason).
- **`reflect`, a genuinely new compiler feature, not a stdlib addition:**
  added as a 4th builtin package (`BuildReflectBuiltinFile`, mirroring
  `os`), backed by real per-type metadata `Any` didn't carry before --
  `kind` (an `RKind` enum classifying the boxed C++ type), `type_name`,
  and (for structs only) `reflect_fields_fn`, a function pointer set via
  a `has_reflect_describe<T>` SFINAE trait (same shape as the existing
  `has_ostream_op<T>` trait `print_fn` already used) detecting an
  ADL-findable `wasigo_reflect_describe(const T*, vector<FieldInfo>&)` --
  which `EmitStructDefs` now emits for every struct, right after its
  definition, listing its exported fields by name as their own Any-boxed
  values. `reflect.Value`/`reflect.Type` are both literally `wasigo::Any`
  (the `os.File`-style trick: `NamedCppType` maps the Go type straight to
  a C++ type with real methods already on it, so a Go method call routes
  through the ordinary struct-method path with no further special-casing)
  -- `Kind`/`Name`/`NumField`/`Field`/`FieldName`/`Interface`/`Int`/
  `Float`/`Bool`/`String` are real `Any` member functions. Only
  `TypeOf`/`ValueOf` (free functions, no natural receiver to hang a real
  method on) needed `EmitCall` special-casing, the same as `os.Getenv`.
  Kind constants (`reflect.Struct` etc.) have no real C++ symbol to
  reference (no generated `reflect.hpp` exists, `reflect` is a builtin)
  so they're resolved directly against `wasigo::RKind` in both
  `InferType`'s Selector case and `EmitSelector`, rather than declared as
  package consts. Two bugs found building this:
  - A parameter/field literally named `__out` silently corrupted its own
    function's parameter list under MSVC -- `__out` is a legacy Windows
    SDK SAL annotation macro (`sal.h`/`specstrings.h`, pulled in
    transitively by MSVC's own standard headers), and macro-expands away,
    so every later use of the (now-undeclared) parameter reported
    nonsensical "syntax error '.'" at the *use* site, not the macro
    collision itself -- took a moment to place. Renamed to `outFields`;
    worth remembering that any `__`-prefixed identifier is reserved to
    the implementation per the C++ standard anyway, not just a SAL trap.
  - `std::vector<Param>` (holds a `unique_ptr<TypeNode>`, non-copyable)
    can't be built from a brace-init list like `{param(...)}` -- that
    always copy-constructs elements. Needed a `params1(Param)` helper
    that `push_back(std::move(...))`s instead, mirroring `results1`.

  Scope, deliberately: read-only (no `Set*`, no addressable/settable
  Values -- real field mutation needs pointer-based access, a bigger
  feature); no Slice/Map/Chan/Func Kind classification (reports
  `Invalid`); Go's same-width type pairs (`int`/`int64`, `byte`/`uint8`,
  `rune`/`int32`) share one Kind since `NamedCppType` already collapses
  them to the same C++ type, with no per-declaration metadata to tell
  them apart. Built specifically to unlock arbitrary-struct
  `encoding/json.Marshal` (now real, including recursive nested structs,
  verified against hand-computed JSON output) -- extend further as real
  needs come up, not ahead of them.
- **`AnalyzeAsync()` only ever scanned the current file's own functions**
  for channel-blocking operations needing `co_await` -- never any
  *imported* package's functions. A method defined in a different file
  that itself blocks on a channel internally (`net.Conn.Read` doing
  `<-c.recv`, added for `net.Pipe()`) never got flagged as async by the
  calling file's own analysis pass, so the call site emitted a plain
  (non-`co_await`ed) call against what's actually a `TaskT<Result>`
  coroutine handle -- "no member named r0/r1" at the call site, not at
  the method definition, since the method's *own* file correctly
  generated it as a coroutine; only the *caller's* view of it was wrong.
  Same shape blind spot `FileNeedsChan`/`NeedCoro` were already fixed for
  (imported channel-typed *fields*, not calls) -- fixed the same way:
  `AnalyzeAsync` now iterates every imported file too, to the same
  fixpoint, not just `file_`.
- **A multi-return function whose result list contains a qualified type**
  (`func scanNumber() (token.Token, string)`) failed to parse: "expected
  an identifier but found '.'". `ParseOneResult`'s named-vs-unnamed
  disambiguation only checked whether the identifier it just consumed was
  followed by a comma or `)` (meaning "that identifier was a complete
  unnamed type by itself") -- it never considered a following `.`, so
  `token` got misread as a *result name* and `ParseType()` was then asked
  to parse a type starting at `.`, which isn't one. A result name is
  always a single bare identifier (Go doesn't allow a dotted name), so
  IDENT-then-`.` unambiguously means "that identifier was a package
  qualifier for an unnamed type," never a name -- fixed by checking for
  `.` alongside comma/`)` and building the qualified type directly.
- **`defer func(){ ... }()` (an immediately-invoked func literal) whose
  body assigns to the enclosing function's named return value** (the
  standard Go idiom for a deferred `recover()` that overrides the return
  on panic) failed to compile: "passing const wasigo::Error as 'this'
  argument". `EmitDefer` always wraps the deferred closure in `[=]`
  (capture by value) -- correct and necessary for a *named/selector*
  defer call's precomputed argument temps (Go evaluates defer args
  immediately, and those temps are destroyed before the deferred closure
  runs, so they need real copies, not references) -- but wrong for an
  immediately-invoked func literal, which doesn't use those temps at all
  and instead re-emits its whole body inline, already itself captured
  `[&]` by the ordinary func-literal rule. Nesting a `[&]`-capturing
  literal inside an outer `[=]`-capturing wrapper makes the literal see
  the *outer lambda's own const copy* of any outer variable (a by-value
  lambda capture is const by default) instead of the function's real
  variable -- so `err = ...` inside the deferred recover handler failed
  to compile (and would have silently written to a throwaway copy even
  if some other pattern let it compile). Reference capture is safe for
  this case specifically: `DeferList` runs during the *same* function's
  own stack unwind, strictly before any variable declared earlier in
  that function (a named return included) is itself destroyed. Fixed by
  switching only the immediately-invoked-func-literal case to `[&]`.
- **A struct field's own type is written unqualified from inside its
  defining package** (`List []*Node` inside package `ast`, not `[]*ast.
  Node`) -- accessed from a different package (`f.List[0]` in package
  `main`), the inferred element type stayed unqualified too:
  `InferType`'s `Selector` case returned `fd->type.get()` straight from
  the `FieldDecl`, with none of the `QualifyResultType` treatment a
  cross-package function's *return* type already got. Same failure shape
  as the earlier `net/mail` slice-of-pointer fix, just for a struct
  field's declared type instead of a call's declared result type -- fixed
  identically, wrapping the field lookup in
  `QualifyResultType(fd->type.get(), st->pkg)`.
- **Go's `recover()` cannot catch a `panic()` more than one call deep on
  this compiler -- a real, permanent constraint, not a bug to fix.**
  `recover()` here is implemented as "stash the panic message on a
  per-function `PanicFrame` and `goto` that same function's epilogue,"
  not real stack unwinding (wasm32-wasip1 has no exception-handling
  proposal support to unwind on, and this compiler doesn't build a
  setjmp/longjmp-based one either) -- so it only catches a panic raised
  within the exact function that owns the `defer`. The classic Go
  recursive-descent-parser shape (panic deep in the call tree, recover
  once at the very top) silently aborts the whole program instead of
  being caught here. `go/parser` uses sticky parser state instead (an
  `err` field checked at the top of every parse method, set once, never
  a panic) -- see its package comment. Keep this in mind for any future
  parser/interpreter-shaped code in this compiler: reach for sticky
  state or explicit `(value, error)` threading, not panic/recover, for
  anything beyond a single function's own body.

See `git log` for the fixes.

## Real wasm execution (wasmtime)

As of 2026-08-28, `wasmtime` is installed on this machine and wired into
every `_golden` test (see "Build" below for the exact setup) -- so
`_golden` no longer just proves the wasi-sdk compile succeeds, it proves
the compiled module's *actual output* matches, under the real
`wasm32-wasip1` runtime. This immediately caught real problems the
host-g++-by-hand method structurally could not:
- The wasm32-specific `shared_ptr`-deleter table-slot bug above (a wasm
  backend/runtime bug, invisible on a host build).
- Three legacy goldens (`fib`, `structs`, `collections`) that predated the
  `EXPECTED_OUTPUT` convention and had never had real expected output
  filled in -- their output was already correct, just never actually
  checked.
- Two of *my own* `EXPECTED_OUTPUT` strings (`regexppkg`, `mailpkg`) had
  transcription mistakes (a missing trailing space from a two-argument
  `Println`, an extra duplicated `"true"`) that eyeballing a host g++ run
  had missed -- exact byte comparison doesn't.

Also since then: `fmt.Errorf`'s `%w` and `errors.Unwrap`/`Join` (real
generator/runtime work, not `.go` -- `fmt`/`errors` are builtins). `Error`
now optionally carries a wrapped `Error`, set by `%w`; `errors.Is`/`Unwrap`
walk that chain. No `errors.As` (needs a runtime type-comparison feature
this compiler doesn't have yet); `errors.Join` concatenates messages but
doesn't multi-chain for `Is`. See `examples/errwrap`.

`fmt.Fprint`/`Fprintln`/`Fprintf` (see `examples/fprint`): `os.Stdout`/
`os.Stderr` map to `std::cout`/`std::cerr` when used directly and unaliased
as the writer argument (real Go defines `fmt.Print*` as
`Fprint*(os.Stdout, ...)`, so this is the same feature both ways); any
other writer argument must be a concrete value with its own `.Write`.
`bytes.Reader`/`strings.Reader` (`NewReader` + `Len`/`Read`/`ReadByte`)
round out `io.Copy`/`io.ReadAll`'s Reader side.

A real `os.File` (see `examples/osfile`): `os.Open`/`Create`/`ReadFile`/
`WriteFile`, and `os.Stdout`/`Stdin`/`Stderr` as genuine `os.File` values
(not just Fprint's special-cased argument -- `var w io.Writer = os.Stdout`
now works). `File.Read`/`Write`/`Close` are plain `<cstdio>`
fread/fwrite/fclose under the hood -- wasi-libc already implements those
via WASI's `path_open`/`fd_read`/`fd_write`, so there's no hand-rolled WASI
syscall code (same reasoning as `fmt.Print` already using `std::cout`).
`os` is one of the four builtin packages (fmt/errors/os/reflect -- no real
`stdlib/os/*.go` is ever loaded), so making `os.File` behave like an
ordinary struct with real methods needed synthesizing `StructDecl`/
`FuncDecl` entries by hand (`BuildOsBuiltinFile` in cpp_generator.cc) so the
same Lookup*/multi-return machinery every other package's methods use works
unmodified. No `Seek`, no `os.Remove`/`Mkdir`/`Stat`, no directories.

Found and fixed along the way: `fmt.Print*`/`Printf`/`Sprintf`/`Errorf`
printed a `byte`/`uint8`/`int8` value as a C++ character (e.g. `88` came
out `X`) instead of Go's always-numeric formatting -- those C++ types are
`char` under the hood, so `std::cout <<` picks the character overload;
fixed by widening to a >1-byte int before printing, except for `%c`, which
now explicitly casts to `char` (previously broken the other way: `%c` on a
`rune`/`int32_t` printed the numeric codepoint instead of the character,
since `int32_t` has no character overload to (accidentally) get this
right). Also: a `Write`/`.Write(...)` call on a pointer-typed writer (e.g.
`fmt.Fprintln(&b, ...)` for a `*bytes.Buffer`) always emitted `.Write`
instead of `->Write` for a pointer. And the async-detection pre-pass (see
the receiver-scoping bug above) had a second, related gap: it never bound
`a, b := f()`'s locals at all (only `a := f()` and `a, b := f(), g()`), so
any later statement needing either name's type inside that same pre-pass
crashed the whole compile; fixed properly for the common call-based
multi-return case, and the pass overall now catches its own inference
errors and degrades gracefully (an unbindable name just stays untyped for
async-detection purposes) instead of ever being able to abort compilation
on its own account again.

`time.Now()` needed the same "no Go source can read this, special-case the
call like os.Args" treatment as os.File's pieces, but simpler: `time` is an
ordinary loaded package (unlike os/fmt/errors), so only the one call needed
intercepting, not a whole synthetic file. While wiring it, found and fixed
another real gap: `pkg.Type(x)` -- a cross-package conversion to a named
type, e.g. `time.Duration(n)` from outside package `time` -- fell through
to "call to undefined function", since the generic imported-package-call
path only ever checked whether `Type` was a *function*, never a type name,
in that package.

**A limitation surfaced, not fixed:** wasigoc only supports methods on a
real `struct` receiver (see the "methods on a defined-slice type" fix
above) -- `type Duration int64` is a `using` alias with nowhere to attach a
C++ member function, so real Go's `Duration.String()` (the standard-library
example of a non-struct method set) can't be a method here either;
stdlib/time spells it `FormatDuration(d)` instead. Properly supporting
methods on any defined type, not just struct, is real future generator
work (a wrapper type with an implicit conversion back to the underlying
type, roughly) -- flagged here rather than attempted under time pressure,
since a half-correct version risks subtle arithmetic/overload bugs across
a wide surface for something day-to-day code can route around.

`bufio` (`Scanner`/`Writer`, see `examples/bufiopkg`), `sync/atomic`
(function-based + Go 1.19+ typed API, see `examples/atomicpkg`),
`encoding/binary` and `math/rand` (self-seeded from `time.Now()`, see
`examples/binrand`) surfaced two more bugs:

- A package-scope global (`const bufSize = 4096`; later, generalized:
  `var Canceled = errors.New("context canceled")`) wasn't visible inside a
  *method* body referencing it, only inside free functions. Cause: Run()'s
  emission order is struct bodies (methods included, inline) before
  globals -- globals need to come after structs in general (an initializer
  can reference a struct type, `var p = Point{1, 2}`), but that left a
  same-file global undeclared yet at the point an earlier inline method
  body used it, since C++'s "complete-class context" early member-
  visibility rule doesn't reach enclosing-namespace declarations written
  later in the file. Fixed in two steps: consts that fold to a compile-time
  integer (`bufSize`) get a full `constexpr` definition moved before struct
  bodies; everything else gets an `extern` forward-declaration there
  instead (its real definition, with its initializer, stays after struct
  bodies where it already was -- `extern T x;` only needs `T` forward-
  declared, which struct types already are by this point, not complete).
- `package rand` (`math/rand`) collided with global `::rand()` from
  `<cstdlib>` -- the same class of bug as `package log` colliding with
  `::log` from `<cmath>` (see above), same fix (added to the same
  libc-name-escaping list in `CppIdent`).

`flag` and `context` (cooperative, over `wasigo::Chan` -- see
`examples/contextpkg`) surfaced three more bugs, found via a segfault, not
a compile error, in the trickiest of this whole push:

- A local variable of a *named function-typed alias* (`type CancelFunc
  func()`, then `var cancel CancelFunc = ...`) wasn't recognized as
  callable -- `cancel()` errored "call to undefined function". Same root
  cause as the earlier `type IntHeap []int` indexing/composite-literal
  fixes: `type X T` is a `using` alias (not a distinct type), so the
  callable-value check needs to resolve through it (`ResolveUnderlying`)
  the same way those did.
- `select { case <-expr: }` where `expr` is anything other than a plain
  channel variable (a method call returning a channel, e.g.
  `ctx.Done()`) failed to compile: `GSelect::recv`/`send` take a `Chan<T>&`
  (non-const lvalue reference), which a call's return value -- a C++
  rvalue -- can't bind to. Fixed by binding each case's channel expression
  to a named local first (`Chan<T>` is a cheap shared_ptr-backed handle, so
  the copy still refers to the same channel).
- **The real bug, found by crash, not compile error:** a package that
  itself never writes `go`/`chan`/`<-` but *imports* one that does (context
  has a `chan bool` struct field) still needs `WASIGO_NEED_CORO` defined --
  the imported package's struct/function definitions land in the same
  translation unit regardless of whether the importing file's own code
  touches a channel, and without that macro, `runtime.hpp` never includes
  `<coroutine>`/the channel machinery at all, so referencing `wasigo::
  Chan`/`make_chan`/`close` from the imported package's code should have
  been a compile error -- except the importing test's *own* `main` also
  used `go`/`select` directly, which independently set the flag and masked
  the gap. `NeedCoro()` was only scanning the current file for a
  channel-shaped global/struct-field/param; now it scans every imported
  file too.

**A second limitation surfaced while chasing that crash, not fixed:** a
plain (non-goroutine) func literal that captures a local variable and then
*escapes* the function that created it (returned, stored somewhere that
outlives the call) captures **by reference** -- `[&]` -- which means the
capture is a reference to a stack slot that's gone the moment the defining
function returns. This is a real, reproduced segfault (a `func()
CancelFunc` factory returning a closure over a local `*Context`), not a
theoretical one. `go`'s closures already capture by value specifically to
avoid this (see the Rosetta table above); ordinary func literals don't,
and fixing that in general needs real escape analysis (which locals does a
literal capture that live past this scope?) -- more than this pass could
safely take on. The workaround used in `stdlib/context` (and worth knowing
for any future stdlib code, or user code, hitting the same shape): don't
return a `func() { ... captures a local ... }` literal -- define a real
method and return a **method value** instead (`return child,
child.cancel`, not `return child, func() { ...child... }`). A method
value's receiver is captured by value (a plain pointer copy -- see
`EmitMethodValue`), which safely outlives the defining function; a
literal's captures are not.

`net/url`, and especially `encoding/json` (a real recursive-descent
parser/serializer with a long internal call chain -- see
`examples/jsonpkg`), found two more general bugs, both about the same
"every multi-return function gets its own uniquely-named result struct"
design (see `ResultStructName`) reaching a case it hadn't before:

- `return otherFunc(...)` -- forwarding another multi-return call's result
  directly as this function's own return, when the two just happen to have
  matching result *types* -- didn't compile: the two results structs are
  still unrelated C++ types with no conversion between them (the earlier
  io.WriteString/bufio.WriteString fix pages above worked around this one
  call at a time by decomposing into named locals first; `json.go`'s
  `parseValue` delegating to `parseObject`/`parseArray`/`parseString`/
  `parseNumber`/`parseLiteral`, and `marshalValue` delegating to
  `marshalArray`/`marshalObject`, made that untenable to keep doing by
  hand). Now fixed generally in `EmitReturn`: `return otherFunc(...)` for
  a multi-result function decomposes the callee's result struct into a
  temporary and reconstructs *this* function's own result struct from that,
  instead of trying to pass the callee's struct value through directly.
- Go string-literal concatenation across a line wrap (`"a very long " +
  "error message"`, both sides literals) failed to compile: each side
  still emits as its own raw C string literal (`EmitExpr(StringLit)` never
  wraps in `std::string`), and `"a" + "b"` has no `operator+` in C++ at all
  (only `std::string + const char*` does). Fixed by wrapping just the left
  operand in `std::string(...)` when both sides of a `+` are string
  literals.

Adding real directory listing (`os.Stat`/`os.ReadDir`, driven by
NewBrowser's goxxlang port of `bundle.Collect` needing it -- see that
repo's README) found one more general bug, in `EmitCall`'s "call a local
variable of `func` type" branch (the one that handles calling a callback
parameter like `filepath.WalkDir`'s own `fn`, as opposed to a package
function or method, which go through `EmitArgsFor` and already get this
right): it emitted every argument with plain `EmitExpr`, never
`EmitExprAs`, so an untyped `nil` argument for a non-pointer parameter
(`error`, calling `fn(path, d, nil)` from `WalkDir`'s own body) came out
as literal C++ `nullptr` -- correct for a pointer parameter, but
`wasigo::Error` has no `nullptr_t` constructor, only the `==`/`!=`
comparison overloads `NilSpellingFor` already exists to get right
(`{}` for `error`/`any`/slice/map/chan/func, `""` for `string`, `nullptr`
only for a real pointer). Fixed by resolving the call's `TypeKind::Func`
underlying type and running each argument through `EmitExprAs` against
that parameter's declared type, the same as `EmitArgsFor` already did for
every other kind of call.

### How to grow it

1. Write Go under `stdlib/<path>/*.go` (skip `*_test.go`; directory = one
   package). `wasigoc` already searches `WASIGO_STDLIB_PATH`.
2. Add a golden (or extend `stdlibpkg` / `anyval`) that imports it and
   checks stdout. CMake `wasigo_add_golden` `DEPENDS` the new `.go` files.
3. Prefer implementing in Go (insertion sort, Newton sqrt, UTF-8 via
   `range`). Reach for `src/runtime.hpp` / generator builtins only for
   WASI (`fd_*`, clocks) or for `fmt`/`errors`/`os` as they exist today.
4. Tick the tracker below. Partial packages: fill the "missing" column
   before starting a new tree when those names are everyday (`TrimSpace`,
   `io.Copy`, `fmt.Errorf`, `ParseInt`).

### Builtins (generator / runtime, not `.go`)

| Package | In | Missing |
| --- | --- | --- |
| `fmt` | `Print`/`Println`/`Sprint`/`Sprintln`/`Printf`/`Sprintf`/`Errorf`/`Fprint`/`Fprintln`/`Fprintf` | `Fprint*`'s writer must be `os.Stdout`/`os.Stderr` written directly (unaliased) or a concrete `.Write`-having value -- a variable holding `os.Stdout` works as an `io.Writer` (real `os.File`) but not as `Fprint*`'s special-cased fast path. No `Scan*`. Format must be a **string literal**; verbs only `%d %s %f %v %t %c %w(Errorf only) %%`, no width/precision |
| `errors` | `New`, `Is` (chain-walking string-equal), `Unwrap`, `Join` | `As` (needs a runtime type-comparison feature this compiler doesn't have) |
| `os` | `Args`, `Exit`, `Getenv`, `File` (`Open`/`Create`/`ReadFile`/`WriteFile`, `Read`/`Write`/`Close`), `Stdout`/`Stdin`/`Stderr`, `Stat`/`FileInfo` (`Name`/`Size`/`IsDir`, via real `stat(2)`), `ReadDir`/`DirEntry` (`Name`/`IsDir`, via real `opendir`/`readdir`/`closedir` -- genuine WASI `fd_readdir`, not a stub) | `Setenv`, process, `Remove`/`Mkdir`; `FileInfo` has no `Mode`/`ModTime`/`Sys` |
| `reflect` | `TypeOf`/`ValueOf`; `Value`/`Type` (both literally `wasigo::Any`) with `Kind`/`Name`/`NumField`/`Field`/`FieldName`/`Interface`/`Int`/`Float`/`Bool`/`String`; Kind constants (`Invalid`/`Bool`/`Int8`.../`Struct`) | No `Set*` (read-only, no addressable values); no Slice/Map/Chan/Func Kind (reports `Invalid`); same-width Go type pairs (`int`/`int64`, `byte`/`uint8`, ...) share one Kind, can't tell them apart |

### Compiled Go under `stdlib/`

| Package | In | vs real package |
| --- | --- | --- |
| `strings` | Prefix/suffix, Index/Contains/Count, Repeat/Join/Split/Replace/ReplaceAll, ToUpper/ToLower, Trim/TrimLeft/TrimRight/TrimSpace/TrimFunc/TrimPrefix/Suffix, Fields, Cut, Map, EqualFold, LastIndex, IndexByte/IndexAny/ContainsAny/ContainsRune, `Builder`, `Reader`+`NewReader` (Len/Read/ReadByte) | rune-aware ToUpper (ASCII-range case only); Reader has no Seek/ReadRune |
| `bytes` | Equal/Index/Contains/Repeat/IndexByte, Trim/TrimLeft/TrimRight/TrimSpace, Clone, Cut, Replace/ReplaceAll, Split, Join, ToUpper/ToLower, HasPrefix/HasSuffix, `Buffer` (+ `Read`), `Reader`+`NewReader` (Len/Size/Read/ReadByte) | Reader has no Seek/ReadRune |
| `strconv` | `Itoa`/`Atoi`, `FormatBool`/`ParseBool`, `ParseFloat` (no exponent), `FormatInt`/`FormatUint` (any base 2-36), `ParseInt`/`ParseUint` (any base, base 0 auto-detects `0x`/`0o`/`0b`/leading-0 octal), `Quote` | No `FormatFloat`/`Append*`, `Quote` has no `\u` escaping |
| `path` | `Base`/`Dir`/`Ext`/`Join`/`Clean`/`Split`/`IsAbs` | No `Match` |
| `path/filepath` | Same as `path`, plus no-op `ToSlash`/`FromSlash`, `Rel`, `WalkDir`/`SkipDir` (built on `os.ReadDir`) | Slash-only on WASI; no `Walk` (the pre-`fs.DirEntry` variant), `Abs` (no `os.Getwd`), `Glob`, `EvalSymlinks`; `WalkDir` never visits `root` itself, only descendants |
| `io` | `Writer`/`Reader`/`Closer`/`StringWriter`, `EOF`, `ErrUnexpectedEOF`, `Copy`, `ReadAll`, `WriteString` | No `MultiWriter`, `LimitReader`, `Seeker`, `Pipe` |
| `math` | Abs/Min/Max/Sqrt(Newton)/Floor/Ceil/Trunc/Mod/Copysign/Signbit/Inf/NaN/IsNaN/IsInf/Exp/Log/Log2/Log10/Pow/Hypot (Exp/Log via range-reduction + series, no libm) | No trig (Sin/Cos/Tan); no `math/rand` |
| `sort` | Insertion Ints/Strings/Float64s + `*AreSorted`, `Interface`+`Sort`+`IsSorted`, generic `Slice`/`SliceStable`/`SliceIsSorted`, `Search`+`SearchInts`/`SearchStrings`/`SearchFloat64s` | `Slice` is generic (`[]T`), not reflection-based like real `sort.Slice(x any, ...)` -- see stdlib/sort |
| `unicode/utf8` | `RuneCountInString`, `DecodeRuneInString`, `RuneLen`, `EncodeRune`, `Valid`/`ValidString`/`ValidRune`, `RuneError`/`RuneSelf` | No `DecodeLastRune*`, `AppendRune` |
| `time` | `Duration` + ns…hour, `Time` (UTC-only: `Unix`/`UnixNano`/`UnixMilli`/`Add`/`Sub`/`Before`/`After`/`Equal`/`IsZero`/`Date`/`Year`/`Month`/`Day`/`Hour`/`Minute`/`Second`/`Weekday`/`String`), `Now`, `FormatDuration`; **`Sleep` is a no-op** | No `Parse`/reference-layout `Format`, no timers/location. `Duration` has no methods (`Nanoseconds`/`String`/... are free functions `DurationNanoseconds`/`FormatDuration`/etc instead) -- methods need a real `struct` receiver and `Duration` is `int64`, see stdlib/time. Sleep must not block the runqueue |
| `cmp` | `Compare`/`Less`/`Or` | matches real package |
| `slices` | Contains/Index/Equal/Reverse/Sort/IsSorted/Max/Min/Clone/Insert/Delete/Concat | No `SortFunc`/`BinarySearch`/`Compact`; `Sort` is insertion sort |
| `maps` | Keys/Values/Clone/Copy/Equal/DeleteFunc, **returning `[]K`/`[]V`** | Real package returns `iter.Seq[K]` since 1.23; no range-over-func here |
| `unicode` | Is{Digit,Upper,Lower,Letter,Space,Punct,Control,Print,Number}, To{Upper,Lower,Title} | ASCII + Latin-1 only, no category tables; no `unicode/utf16` |
| `math/bits` | OnesCount/LeadingZeros/TrailingZeros/Len/Reverse/RotateLeft (32+64-bit), `UintSize` | No `Mul64`/`Div64`/`Add64`/`Sub64` carry ops |
| `container/list` | `List`/`Element`: New/PushBack/PushFront/Remove/InsertBefore/InsertAfter/Front/Back/Len/Init, `Next()`/`Prev()` | Nil-terminated head/tail internally, not Go's circular sentinel ring -- same public behavior |
| `container/ring` | `Ring`: New/Next/Prev/Len/Do/Move/Link/Unlink | matches real package |
| `sync` | `Mutex`/`RWMutex` (no-op Lock/Unlock, real TryLock), `Once` (real), `WaitGroup` (counts but `Wait()` cannot block -- no Go-level scheduler yield outside a channel coroutine) | No `Map` (needs `any` equality/hash; `wasigo::Any` has neither) |
| `container/heap` | `Interface` (Len/Less/Swap/Push/Pop, not embedding `sort.Interface`), `Init`/`Push`/`Pop`/`Remove`/`Fix` | An implementer must be a struct wrapping a slice, not a method on a bare `type X []T` -- see stdlib/container/heap |
| `encoding/hex` | `Encode`/`EncodeToString`/`Decode`/`DecodeString`/`EncodedLen`/`DecodedLen` | matches real package |
| `encoding/base64` | `StdEncoding`/`URLEncoding`, `EncodeToString`/`DecodeString` | Standard padding only, no raw/no-padding variants |
| `encoding/binary` | `LittleEndian`/`BigEndian`: Uint16/32/64 get/put/append | No varint, no `Read`/`Write` (io-based) |
| `log` | `Print`/`Println`/`Fatal`/`Fatalln`/`Panic`/`Panicln` | No `Printf`/`Fatalf`/`Panicf` (fmt.Printf's format string must be a literal *at the fmt.Printf call site* -- a forwarded `format string` parameter can never satisfy that, so no wrapper is possible at all, not just unwritten); no timestamp/file-line prefix, no `*Logger`, writes to stdout not stderr |
| `unicode/utf16` | `Encode`/`Decode`/`EncodeRune`/`DecodeRune`/`IsSurrogate`/`RuneLen` | matches real package |
| `bufio` | `Scanner` (default line-splitting only, no custom `Split`), `Writer` | No `Reader` |
| `sync/atomic` | `Add`/`Load`/`Store`/`Swap`/`CompareAndSwap` (Int32/Int64/Uint32/Uint64 function-based), Go 1.19+ `Int32`/`Int64`/`Uint32`/`Uint64`/`Bool`/`Value` types | One thread: every op is a plain load/store/compare, not a real CPU atomic |
| `math/rand` | `Source`/`Rand` (xorshift64*), package-level funcs, self-seeded from `time.Now()` | Not cryptographically secure (same caveat real `math/rand` always had); no `NormFloat64`/`ExpFloat64` |
| `flag` | `String`/`Int`/`Bool` + `Var` forms, `Parse`, `Args`/`NArg`/`Arg`/`Parsed` | No `FlagSet`, `Usage`/`PrintDefaults`, `Float64`/`Duration` flags |
| `context` | `Context` (a concrete struct, not real Go's interface), `Background`/`TODO`/`WithCancel`/`WithValue`, `Done`/`Err`/`Value` | `Value` keys are `string`, not `any` (`wasigo::Any` has no equality); no `WithTimeout`/`WithDeadline` |
| `net/url` | `QueryEscape`/`QueryUnescape`/`PathEscape`/`PathUnescape`, `URL` struct + `Parse`/`String`, `ParseQuery` | No userinfo/port split; `ParseQuery` returns a flat `map[string]string`, not real Go's multi-value `Values` |
| `encoding/json` | `Marshal`/`Unmarshal` for the *generic* decoded-JSON shape (`any`/`map[string]any`/`[]any`/`string`/`float64`/`bool`/`nil`), PLUS `Marshal` for arbitrary structs via `reflect` (exported fields, declaration order, recursive nested structs) | `Unmarshal` still can't decode into an arbitrary struct pointer (needs settable/addressable reflect Values); no slice-of-struct/map-of-struct field marshaling yet; numbers in scientific notation parse-error (`strconv.ParseFloat` here has no exponent support) |

### Next slices (everyday holes first)

Do these before opening a new package tree -- they are what a ported `.go`
file hits first:

Done: `io.Copy`/`ReadAll`, `strings.Fields`/`TrimSpace`/`Cut`/`ReplaceAll`,
`strconv.ParseInt` with bases, `bytes`/`path`/`sort`/`math`/`unicode/utf8`
fill-in, `cmp`/`slices`/`maps`/`unicode`/`math/bits`/`container/list`
/`container/ring`/`container/heap`/`sync`/`sync/atomic`/`encoding/hex`
/`encoding/base64`/`encoding/binary`/`log`/`unicode/utf16`/`bufio`/
`math/rand`/`flag`/`context`/`net/url`/`encoding/json` (generic value
shape only -- see the tables above), `fmt.Errorf`'s `%w` +
`errors.Unwrap`/`Join`, `fmt.Fprint`/`Fprintln`/`Fprintf` (to
`os.Stdout`/`os.Stderr` or a concrete Writer), `bytes.Reader`/
`strings.Reader`, a real `os.File`
(`Open`/`Create`/`ReadFile`/`WriteFile`/`Stdout`/`Stdin`/`Stderr`),
`time.Time`/`Now` (clocks are `std::chrono`, portable across host+wasi-sdk),
the byte-prints-as-a-character `fmt` fix, a real backtracking `regexp`
engine, `os/exec` stubbed with clear "not supported" errors, `net/textproto`
(MIME header parsing -- groundwork for `net/http`, verifiable without
sockets), the nil-vs-empty `map[K]V{}` fix, `net/mail`, the wasm32
`shared_ptr`-deleter fix, the two MSVC same-name-field fixes, `wasmtime`
wired into the golden tests for real output verification, hex/octal/
binary integer literal support, `hash/fnv`+`hash/crc32`+`hash/adler32`,
and `crypto/md5`+`crypto/sha1`+`crypto/sha256` (each verified against 2-3
standard published test vectors, not just self-consistency); a real
(read-only) `reflect` package (4th compiler builtin) plus arbitrary-struct
`encoding/json.Marshal` built on it, including recursive nested structs;
`net` as a deliberate, verified-necessary stub for `Dial`/`Listen`, plus a
real, working `net.Pipe()` (in-memory, channel-backed) and the
cross-package async-detection fix it needed; a real Go source pipeline
(`go/token`+`go/scanner`+`go/ast`+`go/parser`) and the three general
compiler bugs it surfaced (a qualified-type-in-multi-return-list parse
bug, a defer-closure capture-mode bug, a cross-package struct-field-type
qualification gap); `go/printer`+`go/format` (Node tree back to source,
gofmt-style round trip) and a bounded `go/types` (an interned,
pointer-identity type representation plus expression AND statement type
checking) built cleanly on top with zero new compiler bugs -- the four
fixes above plus the whole `reflect`/`net.Pipe` foundation from earlier
the same day carried the rest of the way without needing anything new;
then extended `go/parser`/`go/ast`/`go/printer` with `var`/`const`
declarations, pointer/slice/map types, composite literals, `switch`,
range-for, and address-of/dereference (`&x`/`*x`) -- all verified
together in one combined program (declare, take an address, build a
slice and map literal, range over the slice, switch on a value, print it
back out correctly indented) -- plus `go/types.CheckStmt`, extending
type checking from one expression to a whole statement tree, and
`go/version`; then extended `go/types` again with map types (`MapOf`,
interned like every other type shape), slice/map indexing and slice
composite-literal element checking in `CheckExpr`, and range-for plus
tagged/bare `switch` support in `CheckStmt` -- including a fix for the
blank identifier (`_ = i`) in plain assignment, which had been
incorrectly type-checked as a real variable reference. Verified with a
15-check program covering interning identity for maps, indexing into a
`[]int`, a well-formed and a mismatched slice literal, a full function
body (`range` binding, tagged `switch`, bare `switch{}`) type-checking
clean end to end, and a deliberately mismatched `switch` case
correctly rejected -- zero new compiler bugs.

Still open: real `net`/`net/http` (see the "not possible on this target"
note below -- `net` itself is now a deliberate stub, not a todo).

**`net`/`net/http` status: real sockets are NOT possible on this
project's actual compile target, verified directly (2026-08-28), not
assumed.** wasi-libc's own `sys/socket.h` compiles `socket`/`connect`/
`bind`/`listen`/`accept` out entirely whenever `__wasip1__` is defined:

```c
#if (defined __wasilibc_unmodified_upstream) || !(defined __wasip1__)
int socket (int, int, int);
#endif
```

and `wasm32-wasip1-clang++` (this project's compiler) does define
`__wasip1__`. A minimal probe program calling `::socket(...)` through the
exact same `-nostdinc++` + noeh `-isystem` order `run_golden.cmake` uses
fails to compile with "no member named 'socket'" -- confirmed empirically,
not inferred from the header alone. This is WASI Preview 1's own
by-design capability model (no ambient network authority, pre-opened fds
only) -- not a missing wasmtime flag, not a missing SDK component, and
not fixable from stdlib or even from `cpp_generator.cc`: the syscall
surface itself doesn't exist for this target. A real `wasi:sockets`
interface exists only for the wasm32-wasip2 Component Model target
(`wasm32-wasip2-clang++` is present in this SDK) -- a fundamentally
different ABI (components, not linear-memory modules) that `wasigoc`
isn't built around; migrating would be a separate, large undertaking, not
a stdlib addition.

Given that, `Dial`/`Listen` and `Listener`'s methods stay a deliberate
stub (same shape as `os/exec`): a clear "not supported on wasm32-wasip1"
error rather than silently pretending to work. `net/http` was not built
on top of them -- a request/response layer over a transport that can
never actually connect would be dead weight providing false confidence,
not a useful stub. If real cross-process networking is ever needed, the
wasip2 migration is the prerequisite question to answer first, not
something to route around from within stdlib.

**`net.Pipe()` (2026-08-28, later the same day) is the one part of this
package that's real, not a stub** -- an in-memory, synchronous,
full-duplex `Conn` pair, backed by two unbuffered Go channels (one per
direction) instead of OS sockets, so it needs nothing WASI doesn't
already have. `~/WASMHolePunch` (a sibling C++ project, pointed at
mid-session as a "this opens a UDP port from WASM" counter-example) was
checked directly and turned out to *confirm* the sockets finding instead
of contradicting it: its own `CMakeLists.txt` links a completely
different, no-op `wasi_sockets_stub.cc` for its WASI build
(`WhpResult UdpSocket::Bind(...) { return WHP_RESULT_UNIMPLEMENTED; }`)
and only links the real socket code for the native build -- the exact
same stub-under-WASI shape this project already had. Building `net.Pipe`
found and fixed one more general compiler bug: `AnalyzeAsync()` only ever
scanned `file_`'s own functions for channel-blocking operations that need
`co_await`, never any *imported* package's functions -- so a method
defined in another file that itself blocks on a channel (`Conn.Read`
doing `<-c.recv`, called from a completely different file) never got
detected as async by the calling file, and its call site emitted a plain
(non-`co_await`ed) call against what's actually a `TaskT<Result>`
coroutine handle, not the result struct itself -- "no member named
r0/r1", the same category of blind spot `FileNeedsChan`/`NeedCoro` had
already been fixed for (a channel-*typed field*, not a call, not scanning
imports). Fixed the same way: `AnalyzeAsync` now iterates every imported
file too, to the same fixpoint. Verified with a 16-check golden
(`netpkg`) exercising a real goroutine writing through a `Pipe()` while
`main` reads, partial-buffer reads across multiple `Write` calls, EOF
after `Close()`, and both ends of the closed-pipe error path -- all
correct under both host g++ and real `wasmtime` execution.

**Building `encoding/csv` (2026-08-28) surfaced three more general compiler
bugs, none about CSV itself** -- found because `csv.Reader`/`csv.Writer`
were the first stdlib code to combine an unqualified cross-package type
collision, a bare interface-typed struct field, and a no-type-annotation
global var all at once, none of which any earlier package had exercised
together:

- **The real one, found by a "call has 1 argument(s) but 0 expected"
  error nowhere near the actual call:** `LookupStruct`/`LookupInterface`/
  `LookupAlias`, when asked to resolve an *unqualified* name (the normal
  case for any reference inside a type's own defining file), fell through
  to searching `opt_.imported_files` -- every OTHER package in the whole
  program, not just the current one -- if the name wasn't found in the
  current file. Go's own rule is that an unqualified identifier always
  means the current package, full stop (this project has no dot imports
  to complicate that -- see the parser's explicit rejection of `import .
  "path"`), so this was simply wrong, and a real, reproducible, silent
  cross-package name collision: two unrelated packages each declaring a
  type with the same bare name (here, `io.Reader` the interface and both
  `encoding/csv` and `strings` each declaring their OWN unrelated
  `Reader` struct) could resolve a same-package reference to the WRONG
  package's type entirely silently. Concretely: inside `io.ReadAll`'s own
  body, `r.Read(buf)` (where `r`'s real type is the `io.Reader`
  interface) resolved against `csv.Reader`'s own *unrelated* `Read()`
  method (a same-named coincidence, zero params) instead of correctly
  falling through to interface-method dispatch, because the buggy
  fallback let an empty-`pkg` struct lookup match ANY imported package's
  struct by bare name. Fixed by restricting all three lookups' `pkg`-
  empty case to `file_` only, matching Go's actual scoping rule.
- **Immediately surfaced a real regression from that fix, not a new bug:**
  `NamedCppType(const TypeNode*)`'s cross-package branch had been
  delegating its "does this type exist at all" check to the *bare-name*
  overload -- which used to silently succeed via the exact same buggy
  imported-files fallback the fix above just closed, so cross-package
  types like `*strings.Reader` stopped resolving at all. Fixed by making
  that existence check itself take the type's actual `pkg` and query
  `LookupStruct`/`LookupInterface`/`LookupAlias` with it directly, instead
  of asking an unscoped "does ANY package have this name" question.
- **A second, independent, previously-latent bug the same investigation
  turned up:** a package-level global var declared with NO explicit type
  (`var LittleEndian = byteOrder{...}` in `encoding/binary`,
  `var StdEncoding = Encoding{...}` in `encoding/base64` -- both real,
  pre-existing stdlib code, not new) infers its type from the initializer
  expression, which is naturally unqualified relative to ITS OWN defining
  file -- but that inferred type was returned as-is to a caller in a
  DIFFERENT package (`binary.LittleEndian.PutUint32(...)` from outside
  package `binary`) with no re-qualification, unlike the sibling branch
  three lines above it (an explicit `var X T = ...` annotation) which
  already re-qualified correctly. This had been silently masked by the
  same buggy imported-files fallback the whole time -- fixed by
  qualifying the inferred type with the declaring package the same way
  the explicit-type branch already does.
- **A separate, unrelated struct-comparability bug, found once the above
  three were fixed and `csv.Writer` finally reached actual C++
  compilation:** the auto-generated `operator==` for a Go struct exists
  only when every field is "comparable" (Slice/Map/Chan/Func fields
  disqualify it, matching those types' `wasigo::` runtime shapes having
  no `operator==`) -- but a *named interface* field (`w io.Writer`) was
  missing from that disqualifying list, even though the generated
  interface-adapter struct (`self`/`vt`/`type_key`, see
  `EmitInterfaceDefs`) has no `operator==` either. Every earlier stdlib
  struct holding an interface field (e.g. `bufio.Writer`'s own `w
  io.Writer`) also happened to hold a Slice/Map field that already
  disqualified the whole struct first, so this stayed latent until
  `csv.Writer` (fields: two plain scalars plus `w io.Writer`, nothing
  else) became the first struct where an interface field was the ONLY
  reason to disqualify it. Fixed by routing the check through the
  existing `IsInterfaceType` helper (which also subsumed the older
  bespoke `any`-only check it already had).

All four fixes verified via the full test suite (109/109, zero
regressions) before and after, plus `csv.Reader`/`csv.Writer` themselves
verified with 23 checks (quoted fields, embedded commas/newlines,
doubled-quote escaping, comment/blank-line skipping, `FieldsPerRecord`
mismatch, a `Write`\-then-`Read` round trip through a real `bytes.Buffer`)
under both host g++ and real `wasmtime` execution.

**Building `crypto/sha512` surfaced one more general compiler bug, found
by an actual hard compile error (a narrowing-conversion error), not a
silent wrong answer.** SHA-512's own constants (8 init values, 80 round
constants) include several 64-bit hex literals above `INT64_MAX` (e.g.
`0xa54ff53a5f1d36f1`) -- the lexer's own decimal/hex literal parse
already stores these bit-pattern-preserving in an `int64_t` field (the
same design as the earlier FNV-64 constant fix), so such a literal reads
back as a *negative* `int64_t` even though the Go source is an ordinary
positive hex constant. `EmitExpr`'s `IntLit` case printed that value
via `std::to_string(e.intval) + "LL"` unconditionally -- correct BIT
PATTERN, wrong C++ token: a negative signed literal narrowing-converts
into a `uint64_t` brace-init list (`wasigo::Slice<uint64_t>{...}`,
exactly how every hash/crc table in this project is built) illegally
under real narrowing-conversion rules, not just a warning -- "narrowing
conversion... from 'long long int' to 'long long unsigned int'" from
g++, at the exact literal. A Go int LITERAL (not a negated one --
`-5` parses as `UnaryExpr(Minus, IntLit(5))`, a separate node) is never
actually negative at the source level, so a negative `intval` at this
point in the pipeline can only mean "the top half of the uint64 range,
bit-reinterpreted" -- fixed by printing the *unsigned* decimal value
with a `ULL` suffix whenever `intval < 0`, which is correct either way
(the ordinary small/positive case never takes this branch at all).
Verified with 7 checks (3 standard FIPS 180-4 test vectors including the
"quick brown fox" one, streaming `Digest` split across two `Write`
calls, `Reset`, `Size`, `BlockSize`) -- all correct on both host g++ and
real `wasmtime`, plus the FULL test suite re-run (119/119, zero
regressions) since this touches shared literal-emission code used by
every package.

**Building `crypto/subtle` surfaced one more general compiler bug --
same root cause family (integer literals always emitting as 64-bit),
but a different failure shape: silently WRONG arithmetic, not a compile
error this time.** `ConstantTimeByteEq(x, y byte) int { return
int((uint32(x^y) - 1) >> 31) }` returned -1 instead of 1 for equal
bytes. Root cause: every Go int literal emits as a 64-bit `LL` C++
token regardless of its actual Go-level type, so C++'s usual arithmetic
conversions promote the WHOLE subexpression to signed 64-bit arithmetic
the moment a narrower operand (`uint32(x^y)`, a genuinely 32-bit value)
combines with a literal -- `0 - 1` becomes a plain signed `-1`, not the
`uint32` wraparound value `0xFFFFFFFF` Go's own semantics require, and
the following `>> 31` then sign-extends a negative 64-bit value instead
of shifting zeros into a wrapped-around unsigned one. This ONLY breaks
when a narrower-than-64-bit result feeds directly into another operator
without an intervening assignment to a same-width variable (an
assignment's own implicit narrowing conversion happens to correct it,
which is why no earlier package -- none had done arithmetic quite this
way -- ever tripped over it). Fixed generally in `EmitExpr`'s
`BinaryExpr` case: after computing a non-comparison binary op's C++
expression, if the expression's real Go-level type is `int8`/`uint8`/
`byte`/`int16`/`uint16`/`int32`/`uint32`/`rune` (this compiler's
`int`/`uint`/`int64`/`uint64` are already 64-bit, so untouched), wrap
the WHOLE result in an explicit `static_cast` back to that width. This
is safe unconditionally, not just for the buggy case: casting an
already-correctly-computed value to its own real width is a no-op --
it only changes behavior for the previously-silently-wrong widened
case. Verified with 14 checks (`ConstantTimeCompare`/
`ConstantTimeByteEq`/`ConstantTimeEq`/`ConstantTimeSelect`/
`ConstantTimeLessOrEq`/`ConstantTimeCopy`, byte-identical between host
g++ and real wasmtime) plus the FULL suite re-run (137/137, zero
regressions) since this touches the single most shared code path in
the whole generator -- every binary operator in every package goes
through it.

**Building `encoding/pem` surfaced a THIRD instance of the same "type
qualification missing for a function that belongs to a different
package than the one being generated" bug family the `encoding/csv`
round found twice already -- this time in `AnalyzeAsync`'s own
receiver/param declarations, not `LookupStruct`/`NamedCppType`.** A
same-package method as ordinary as `for k, v := range b.Headers` (`b
*Block`, `Headers map[string]string`) made an UNRELATED file's
composite literal (`pem.Block{...Headers: ...}`) fail with "unknown
field 'Headers' on type 'Block'" -- bisected down from the full test to
this exact minimal repro (a plain field read on `b` worked fine; only
the `range` over a map FIELD tripped it). Root cause: `AnalyzeAsync`'s
scanning loop iterates every function in `file_` AND every imported
file to determine which functions need `co_await`-wrapping, and for
each one `Declare`s its receiver/param types into a scratch scope so
statements can be type-checked -- but `SynthNamed(fn.receiver_type)`
(and each param's raw `ParamGoType(p)`) never attached a package
qualifier, even when the function being scanned came from an IMPORTED
file, not `file_`. A receiver type like `*Block`, written bare inside
`pem.go` itself (correctly, relative to that file), stayed bare when
declared into the CURRENT Generator's scope (which might be generating
a completely different file) -- so the later `range b.Headers` field-
type lookup searched `file_`'s own package instead of `pem`'s, and (as
of the `LookupStruct` fix earlier this session) that search is now
correctly scoped to fail rather than silently falling through to
whatever package happened to have a matching struct name. Exactly the
same masking relationship as the `binary.LittleEndian`/
`base64.StdEncoding` regression: pre-existing, latent, invisible until
the broader unqualified-lookup bug was closed. Fixed by tracking each
scanned file's own package name (`fptr == &file_ ? "" : fptr->
package_name`) and qualifying the receiver type directly, and each
param type via the existing `QualifyResultType` helper, whenever the
function being scanned belongs to a different package. Verified with
14 checks (`EncodeToMemory`→`Decode` round trip, a hand-written PEM
block with one header, `Encode` into a `bytes.Buffer`, no-PEM-found and
trailing-content-after-block edge cases) -- byte-identical between host
g++ and real wasmtime -- plus the FULL suite re-run (141/141, zero
regressions).

**Three more general compiler bugs, all found the same way as the very
first entries in this diary: real downstream Go source hitting the
parser, not a package built to exercise a known gap on purpose.**
`~/project_lovelace` (a sibling project vendoring the real
`internal/wasmbin`/`internal/wasmdecomp` decompiler packages through
this compiler to build a standalone WASI guest) had worked around
several `wasigoc` limitations directly in its own vendored source rather
than here, exactly the kind of note this project's own docs ask to be
followed up on. Revisiting that list surfaced three that were real,
general parser/codegen bugs, not narrow enough to leave as a workaround:

- **`type Name [N]T` (a named array or slice type) was completely
  broken**, both loudly and silently. `ParseOneTypeSpec` treated ANY `[`
  right after a type name as the start of a generic type-parameter list
  (`type Set[T any] struct{...}`) unconditionally, calling
  `ParseTypeParamNames` (which immediately does `ExpectIdent()`) no
  matter what actually followed. `type block [64]int32` (found in this
  project's OWN `stdlib/image/jpeg/jpeg.go`, apparently never actually
  exercised end to end -- the `jpegpkg` golden was building generated
  C++ that never got this far) failed outright: "expected an identifier
  but found int literal", `64` obviously not being an identifier. Worse,
  `type Named [maxSize]byte` (a named constant as the array length, a
  perfectly ordinary Go pattern) didn't error at all -- `maxSize` IS an
  identifier, so it silently got consumed as a bogus one-element generic
  parameter list instead, producing a nonsense type. A named SLICE type
  (`type ByteSlice []byte`) was equally broken (`[]` immediately calls
  `ExpectIdent()` against `]`) and, grepped across the entire project,
  had never once been written in this project's own stdlib or examples
  -- a real, total coverage gap hiding behind "nobody happened to need
  it yet." Fixed with a 2-token lookahead (`LooksLikeTypeParamList`) at
  the one call site: a `[` is only ever a type-parameter list when it's
  followed by an identifier AND that identifier is followed by something
  other than `]` (Go's own generic constraint syntax always needs a
  constraint after the name; `[N]` alone can never be valid type params,
  only an array length). Otherwise the `[` is left unconsumed for
  `ParseType()`'s own already-correct slice/array handling to pick up
  normally, the same code path `var` declarations already used
  correctly.
- **Grouped struct field names (`A, B T`, Go's ordinary shorthand for
  two same-typed fields) weren't supported at all** -- exactly the
  `Module, Name string` shape `project_lovelace` had hand-expanded to
  one field per line to work around. `ParseParamList` already handled
  the identical ambiguity correctly for function parameters (collect
  names in a loop, then parse ONE shared type), but `ParseStructFields`
  had never grown the same handling -- it read one identifier, and
  either treated it as a whole embedded field or immediately called
  `ParseType()`, with no comma-loop in between. Fixed by giving struct
  fields the same shape ParamList already has: collect one or more
  comma-separated names, parse the shared type and optional tag once,
  then emit one `FieldDecl` per name (`CloneType` per field, matching
  how `ParseParamList` already clones a shared type across grouped
  params).
- **A generic struct instantiated directly as an expression-position
  composite literal (`Pair[int]{A: 1, B: 2}`, as opposed to a `var s
  Set[int]` declaration -- the only shape any earlier stdlib/example
  code had ever used) failed two different ways in sequence.** First a
  parse failure: `ParsePostfix`'s `[` handling has no concept of "this
  could be a type," so `Pair[int]` parsed as an ordinary `Index`
  expression (`Pair` indexed by `int`), and the `{` immediately after
  had nowhere to go, surfacing nowhere near the real cause ("expected
  ':=' or '=' after a comma-separated expression list", from statement
  parsing much further up the call stack). Fixed by recognizing the
  shape in `ParsePostfix` itself: a single bracketed entry immediately
  followed by `{` (only possible for a type, since a real Go index
  expression can never be directly followed by an unparenthesized `{`)
  or 2+ comma-separated entries in `[...]` (never valid indexing syntax
  at all -- indexing takes exactly one expression) both get reinterpreted
  as type arguments via a new small `TypeArgExprToType` helper (handling
  the identifier/qualified-name/pointer shapes an already-parsed
  expression can actually take), and the whole `Name[Args]{...}` becomes
  a composite literal the same way a plain `Name{...}` already does.
  Second, once parsing was fixed, a codegen failure: `EmitCompositeLit`
  built the struct's synthesized IIFE as `Pair __s{}; ...`, relying on
  C++ class template argument deduction to pick up `T` -- but CTAD has
  no constructor argument to deduce anything FROM at that point (the
  struct is default-constructed, then fields assigned one by one
  afterward), so it fails outright ("no matching function for call to
  'Pair()'"). Fixed by using the already-correct `CppType`/`NamedCppType`
  (which already knows how to append `<Args>` from `type_args`, used
  everywhere else a generic type name is spelled) instead of a bare
  `QualName` call, so the IIFE declares the fully-instantiated
  `Pair<int64_t> __s{};` up front.

New golden `examples/typedecl` (`type Name []T`/`[N]T` with both a
literal and a named-constant length, grouped struct field names on a
2-field and a 3-field struct, and `Pair[int]{...}`/`Map2[string,
int]{...}` generic composite-literal instantiation with one and two
type arguments) -- 7 checks, matching between host g++ and real
`wasmtime`. Full suite re-run after all three fixes: 277/278 (the one
failure, `runtime_smoketest`, is a pre-existing, unrelated host
panic/recover-on-closed-channel test that never touches
`parser.cc`/`cpp_generator.cc`, the only two files this round changed).

**GocVM (2026-09-03): one compiler-known dispatch gate, not per-package
FFI.** Sibling project `~/shim_sandbox` made its 8 "extra G++" stub
topics (net/os.exec/os.user/syscall/tls -- see its own
docs/architecture.md) real (Winsock/Win32 backends), but `os/exec`,
`os/user`, `syscall`, `net`, and `crypto/tls` here are ordinary parsed
Go++ source with no way to reach hand-written C++ at all -- only `os`
gets that treatment, as one of the compiler's few special-cased builtin
packages (`BuildOsBuiltinFile`, `module_loader.cc`'s
`IsBuiltinImport`). Rather than repeating that whole-package-builtin
treatment four more times, or inventing a generic bodyless-extern
mechanism, added **one** new builtin function following the exact same
precedent `os.Getenv` already set: `gocvm.Call(topic, payload string)
(string, error)` (`BuildGocvmBuiltinFile` in `cpp_generator.cc`, one
`EmitCall` branch, one `ResolveCalledFunc` branch so `s, err :=
gocvm.Call(...)` unpacks through the ordinary multi-return path like
`os.Open` already does -- `wasigo::gocvm::CallResult{r0, r1}` matches
`OsOpenResult`'s exact `rN`-field shape on purpose).

The dispatch primitive itself (`wasigo::gocvm::Call`, `src/runtime.hpp`)
is a small pluggable-bridge pattern: a `HostBridge` interface (default:
`kNoBridge`, the same honest "not supported" shape every existing stub
already had) plus an `AbacHook` interface (default: allow-all -- richer
per-caller ABAC is future work, not needed yet). Registration is
explicit, not static-init-across-a-static-library-archive (a linker can
silently drop an unreferenced `.o` from a `.a` with no
`--whole-archive`): `wasigo::set_os_args`, already called identically by
both of `cpp_generator.cc`'s `main()` shapes, calls
`wasigo_gocvm_install_bridge()` when built with `-DWASIGO_GOCVM_BRIDGE=1`
-- `goclang++.bat --shim-sandbox` now passes that flag, and
`shim_sandbox/src/gocvm_bridge.cc` supplies the real bridge, a thin
adapter over its existing `W2gSapiHandle` (zero duplicated logic).

Per the user's explicit request, GocVM also doubles as the virtual-
goroutine registry, reusing this project's existing Oilpan-lite cppgc
(`gc::GarbageCollected`/`Persistent`, not a new allocator): a
`gocvm::VThread` is allocated and stored as a `gc::Persistent<VThread>`
field directly on `Task`/`TaskT<T>`'s `promise_type`, set only in the
three `go()` overloads (not on every `co_await`-helper coroutine --
same distinction Go's own `go` statement draws) -- so a spawned
goroutine's registry entry is rooted for exactly its coroutine frame's
lifetime with no separate list to keep in sync by hand, and no
`UnregisterThread` call site needed at all.

**Real bug hit along the way**: a block-scope `extern "C" void f();`
inside a function body is not valid C++ (a linkage-specification is
namespace-scope only) -- clang rejects it with a confusing "expected
unqualified-id" pointing at the linkage string itself, not an "extern
not allowed here" message. Moved the forward declaration up to namespace
scope, next to (not inside) `set_os_args`.

**Also found while wiring this in**: `shim_sandbox`'s own CMake sibling
search (`../wasigoc`, `../WASIGo++`) never looked for `../go++` -- this
workspace's actual fork -- so it had been silently building against the
*unforked* `~/WASIGo++/src/runtime.hpp` the entire session (harmless
until now, since nothing shim_sandbox used had actually diverged yet).
Fixed by adding `../go++` to that search list, checked before
`../WASIGo++`.

Wired 3 of the now-real shim_sandbox topics through `gocvm.Call` into
real Go++ stdlib source -- `os/exec` (`Run`/`Output`/`CombinedOutput`,
parsing real_win.cc::Exec's `"exit=<n>\n<output>"` reply), `os/user`
(`Current`), `syscall` (`Getpid`/`Getppid`/`Getenv`/`Environ`) --
chosen because gocvm's existing single-request/single-reply shape is
their *complete, correct* semantic. `net.Dial`/`net.Listen`/`tls.Dial`
are deliberately NOT wired this round: they conceptually want a live,
usable `Conn` handed back, and shim_sandbox's 8 topics today only prove
reachability (connect-then-close) -- wiring those for real needs a
session/handle protocol extension first, not attempted here.
Hand-verified end to end outside the golden-test harness (compiled a
`.go` using `gocvm.Call` directly through wasigoc, then native clang++
against a rebuilt `libw2g.a` with `-DWASIGO_GOCVM_BRIDGE=1`): real
`syscall.Getpid()`-equivalent traffic returned this session's actual
process ID, not a canned value. Every existing golden test for these
three packages (`execpkg`/`userpkg`/`syscallpkg`) still passes unchanged
under plain `compile.bat` (wasm32-wasip1, no bridge linked) -- confirmed
their exact expected-output strings are still produced when
`gocvm.Call` reports no bridge registered, since that's the same
`errNotSupported`/canned-value fallback these packages always returned.
Full go++ `ctest` suite re-run: 277/278 (the one failure,
`runtime_smoketest`, is the same pre-existing host-only panic/recover
test noted above, unrelated to this round's `runtime.hpp`/
`module_loader.cc`/`cpp_generator.cc`/stdlib changes).

**GocVM, later still (2026-09-03): live sockets/processes, real TLS --
"complete 1:1 with go, not the limitations of wasi 1."** The 3 packages
above had single-request/single-reply as their *complete* semantic;
`net`, `crypto/tls`, and the rest of `os/exec`/`os/user`/`syscall` need
a *live, stateful* resource instead. Added a handle table to
shim_sandbox (`src/sapi/handles.h`/`.cc` -- open sockets/processes/TLS
sessions, referenced by an opaque id) and ~12 new gocvm topics
(`net.accept`, `net.io.read`/`write`/`readfrom`/`writeto`/`close`,
`os.exec.start`/`wait`/`stdout.read`, `tls.io.read`/`write`/`close`,
plus extended ops on the existing `os.user`/`syscall` topics) dispatched
from the same single choke point (`src/sapi/handle.cc`). `net.dial`/
`net.listen` now leave the socket **open** (a real behavior change from
earlier this session's reachability-probe-only version) and hand back a
handle instead of closing it.

Real TLS (`shim_sandbox/src/sapi/tls_win.cc`, ~300 lines): Schannel/
SSPI, the standard `AcquireCredentialsHandleW` + `InitializeSecurityContextW`
client handshake loop, automatic certificate chain + hostname validation
always on (`SCH_CRED_MANUAL_CRED_VALIDATION`/`SCH_CRED_NO_SERVERNAME_CHECK`
never set) -- chosen over vcpkg's already-available OpenSSL specifically
to keep `goclang++.bat`'s consumer `.exe` dependency-free (Schannel is
part of Windows itself, `-lsecur32 -lcrypt32`, no DLL to ship). `os/user`
`Lookup`/`LookupId` are real too (`LookupAccountNameW`/`LookupAccountSidW`
+ `NetUserGetInfo` for home dir and primary-group SID, the same approach
real Go's own Windows `os/user` implementation uses). `syscall.Chdir`/
`Kill` are real (`SetCurrentDirectoryW`, `OpenProcess`+`TerminateProcess`).

Rewired `stdlib/net/net.go` (`Conn`/`Listener`/`PacketConn` gain a
`real bool`/`handle string`, every op tries `gocvm.Call` first, falls
back to the original local-loopback-only logic on `err != nil`),
`stdlib/crypto/tls/tls.go` (`Dial` does connect+handshake in one call,
`Read`/`Write`/`Close` map to `tls.io.*`), `stdlib/os/exec/exec.go`
(`Start`/`Wait` -- `Start` launches an output-pump goroutine when
`Stdout`/`Stderr` is set, `Wait` joins it via a `pumpDone` channel before
confirming the exit code, matching real Go's Wait semantics), `stdlib/
os/user/user.go` (`Lookup`/`LookupId`), `stdlib/syscall/syscall.go`
(`Chdir`/`Kill`).

**A modeling mistake caught before it shipped broadly**: `gocvm.Call`'s
`(string, error)` only signals `err != nil` when there is *no real
answer at all* (no bridge) -- a real bridge's own failure (a real
`connect()` refused, a real `CreateProcess` error, ...) still comes back
`err == nil` with the payload starting `"error: "` (every real_win.cc
handler's existing convention). The first draft of several of these
wired functions only checked `err`, so a genuine operational failure
would have been silently misread as valid data (an "error: ..." string
handed back as a username, or spliced into output bytes with a generic
"exit status -1"). Fixed by adding an `isRealError(reply)` check
(`strings.HasPrefix(reply, "error:")`) in every newly-wired function
before treating a reply as data -- `err != nil` still means "no bridge,
fall back to the old stub"; `err == nil && isRealError(reply)` now means
"a real bridge gave a real, definitive failure," surfaced as a real Go
error instead.

**Two more real, general bugs found (not source-level workarounds)**:
- `src/sapi/tls_win.cc`: `QueryContextAttributesW` returns a
  `SECURITY_STATUS` (0 == success), not a `BOOL` -- `if
  (!QueryContextAttributesW(...))` treated success (0, "falsy" in C++)
  as failure, so the handshake completed but every dial then failed
  with a confusing "QueryContextAttributes(STREAM_SIZES) failed".
  Fixed by comparing the returned status against `SEC_E_OK` explicitly.
- **wasigoc, `src/cpp_generator.cc`'s `EmitGo`**: `go recv.AsyncMethod(...)`
  (a method that itself uses channels, so it returns `wasigo::Task`/
  `TaskT<T>`) fell through to the generic "ordinary callable" fallback --
  `wasigo::go([=]{ recv.AsyncMethod(...); })` -- which calls the method as
  a plain statement, constructing but never starting-and-scheduling the
  returned `Task` (never passed to `go()` or `co_await`ed), so `~Task`
  destroys the coroutine frame before it ever runs a single line of the
  method body. Anything waiting on a signal from that goroutine (here,
  `os/exec.Cmd.Wait()` parked on `<-pumpDone`) deadlocks forever --
  `Scheduler::run()`'s own "all goroutines are asleep" panic, not a hang.
  This is exactly the kind of bug the established methodology looks
  for, except here it was this session's *own* new code (`os/exec`'s
  `go c.pump()`) that exercised the gap for the first time --
  confirmed by grepping the entire `stdlib/`+`examples/` tree: no other
  `go recv.method(...)` call exists anywhere else in the codebase, so
  this fix changes no other package's already-golden-verified output.
  Fixed the same way the existing free-function/package-function cases
  in `EmitGo` already work: detect `IsAsyncMethod` on the resolved
  method and pass the call straight to `wasigo::go(...)`.

Verified end to end with real Go++ source (wasigoc -> native clang++ ->
`libw2g.a` with `-DWASIGO_GOCVM_BRIDGE=1`, since `.bat` still can't run
through this session's tools): a two-*process* real TCP server/client
(a real `Listen`/`Accept` and a real `Dial`, exchanging real bytes both
ways -- deliberately two processes, not two goroutines in one, since
wasigo's cooperative one-OS-thread scheduler means a blocking `Accept()`
and a blocking `Dial()` in the *same* process can't rendezvous: whichever
runs first never yields back for the other to run, a genuine pre-existing
limit of the model, not new to this round); and a single program doing
real `exec.Start()`/`Wait()` with streamed `Stdout`, a real
`user.Lookup()` matching `user.Current()`, a real `syscall.Chdir()`, and
a real TLS handshake + HTTPS GET to a live host (`example.com`) that came
back an actual `HTTP/1.1 200` with a real decrypted body. Full go++
`ctest`: 277/278 (same pre-existing unrelated failure). shim_sandbox's
own `ctest`: 2/2, extended with a real loopback socket round trip, real
`exec.Start`/`stdout.read`/`Wait`, a real `os.user` lookup of the current
user, and a real TLS handshake + HTTPS GET against a live host.

**Deferred, not attempted**: `Cmd.Stdin` (a child's stdin is always
wired to `NUL` -- no interactive input); separating a child's stdout
from its stderr (the real backend combines them into one pipe, so
`Cmd.Stderr` alone falls back to receiving the combined stream);
`os/user.Lookup`'s home-directory guess when `NetUserGetInfo` reports
none (`C:\Users\<name>`, the common local-account convention, not
verified against every possible account configuration); TLS session
renegotiation (surfaced as an honest error, not attempted); a POSIX
`HostBridge` (still Windows-only, matching `real_posix.cc`'s existing
honesty).

**GocVM, later still (2026-09-03): tests for the ErrorState state
machine, plus a real toolchain-level linker bug it exposed.** The
`gocvm::Call` ErrorState machine (`kClear`/`kBridgeActive`/`kPanic`,
`src/runtime.hpp`) had zero test coverage: every compiled golden test
runs via `compile.bat` with no bridge linked at all (only ever exercises
the `kNoBridge` branch), and the real `shim_sandbox` bridge never panics
in practice, so the "a bridge call panics and must surface as a
`wasigo::Error` instead of aborting" contract -- the entire reason
`ErrorState`/`BridgeScope`/`panic_or_stash` exist -- had never actually
been run. Added `tests/runtime_smoketest.cc::gocvm_error_state()`: five
fake `HostBridge`/`AbacHook` implementations (success, a real bridge
failure via `ok=false`, an internal panic via a `PanicFrame` unwound
inside the bridge call exactly like compiler-emitted `goto
__wasigo_end` does, a reentrant nested `gocvm::Call`, and an ABAC deny)
driving `gocvm::Call` directly and asserting on both the returned
`wasigo::Error` text and that `g_error_state` always ends back at
`kClear` afterward -- including immediately re-using the bridge slot
after a stashed panic, which is the case most likely to leak state if
`BridgeScope`'s RAII guarantee ever regressed.

Building this test (the first thing in the repo to link a second,
different `HostBridge` against `runtime.hpp` while `shim_sandbox`'s
`libw2g.a` *also* links a bridge against the same header) surfaced a
real, general, and previously-latent linker bug, not a test bug:
`inline thread_local ErrorState g_error_state;` (a C++17 inline
variable) fails `shim_sandbox`'s `w2g_bridge` build with "multiple
definition of TLS init function for wasigo::g_error_state" on this
machine's toolchain (WinLibs mingw GCC 16.1.0 / bundled binutils) --
every object file that includes `runtime.hpp` (each `libw2g.a` member,
plus the executable linking against it) emits its own copy of the
compiler-generated TLS guard/init function for the inline variable, and
this GCC/binutils combination doesn't COMDAT-fold it away on a PE/COFF
target, unlike ELF. `g_error_state` had existed since the GocVM entries
above but nothing before this round ever linked two separate
`runtime.hpp`-including binaries together in one build graph the way
`shim_sandbox`'s own executables (`w2g_bridge`, `w2g_tests`, ...)
already always have -- so this was latent, not new, and this round's
test file was incidental to surfacing it, not a special trigger. Fixed
by replacing the bare inline variable with `inline ErrorState&
g_error_state()` wrapping a function-local `thread_local` static (the
standard Meyer's-singleton shape, using the *function's* COMDAT folding
rather than a bare inline-variable's TLS-init-function folding, which is
the part this toolchain doesn't support) and updating every call site
from `g_error_state.foo()` to `g_error_state().foo()`. Verified: `ctest`
in both `~/go++/build-fork` (278/278) and `~/shim_sandbox/build` (2/2,
including a full from-scratch rebuild of `w2g_bridge`/`w2g_tests` that
previously failed to link at all) after the fix.

**GocVM, later still (2026-09-03): the `err != nil` half of `isRealError`
was never wired -- fixed across all 5 packages, then shim_sandbox+ABAC
made the default.** Auditing every `gocvm.Call` site while adding the
`ErrorState` test coverage above surfaced a second, more consequential
gap than the linker bug: `isRealError`'s own doc comment, present since
the very first GocVM entry, already correctly says `gocvm.Call`'s
`err != nil` should mean "no bridge, fall back" -- but every call site
in `os/exec`, `net`, `crypto/tls`, `os/user`, `syscall` actually treated
*every* `err != nil` that way, with no code anywhere checking *why* `err`
was non-nil. Before this round that distinction was moot (the only
`err != nil` case in practice was genuinely no bridge), but the
`ErrorState` machine above means a real `--shim-sandbox` build can now
also produce `err != nil` for an ABAC deny, a bridge-internal panic, or
(pathologically) a reentrant call -- all three would have been silently
misreported as `errNotSupported`/`ErrNotSupported`/a canned fallback
value, i.e. "this platform doesn't support X" on a build where it
genuinely does and something real actually broke. `net.go`'s four
connection-establishing functions (`Listen`/`Dial`/`ListenPacket`/
`DialPacket`) had the same bug in inverted form (`if err == nil { real
path }`, else silently fall through to the local-only `Pipe`-backed
stack) -- worse there, since it produces a *successful* `Conn`/`Listener`
that can never actually reach the requested address, not even a visible
error.

Fixed generally in all 5 packages: added `isNoBridge(err error) bool`
(checks `err.Error()` for the exact `"no host bridge registered"`
substring `wasigo::gocvm::kNoBridge` always includes) and changed every
site from `if err != nil { return fallback }` to `if err != nil { if
isNoBridge(err) { return fallback }; return err }` (net.go's four
`err == nil`-gated sites: `if !isNoBridge(err) { return nil, err }`
before falling through). Left `syscall.Getpid`/`Getppid`/`Environ`/
`Getenv` unchanged -- they match real Go's own infallible signatures (no
`error` return at all to surface anything through, same documented bound
as real Go's `Getpid`, which also cannot fail). Verified: rebuilt +
`ctest` for `execpkg`/`netpkg`/`tlspkg`/`userpkg`/`syscallpkg`
individually (all pass, byte-identical output — these packages' golden
tests all run bridge-less, so `isNoBridge` still routes every one of
them to the exact same fallback path as before) plus the full suite
(278/278).

With that fixed, a real bridge failure can no longer be mistaken for a
platform limitation, so the user asked to flip `goclang++.bat`'s
`--shim-sandbox --abac` from opt-in to the default (confirmed explicitly
over two narrower options: default just the bridge, or leave it opt-in
and only discuss the tradeoff). Changed `goclang++.bat`: `USE_SHIM`/
`USE_ABAC` now default to `1`; `--shim-sandbox` still exists but now
only changes *how strictly* a missing/unbuilt shim_sandbox is treated --
by default it's a silent fallback to a bridge-less build (identical to
today's `wasigoc`-only behavior), `--shim-sandbox` (or any future CI
invocation wanting a hard failure instead of a silent downgrade) makes
it `exit /b 1` the same way it always has. Added `--no-shim-sandbox`
(skip the bridge entirely) and `--no-abac` (bridge, no
`-DW2G_ABAC_SYSTEM=1`). Updated `docs/build.md` and `README.md` to
match. **Not verified by actually running the `.bat`** (`.bat` files
still can't execute through this session's tools, see the standing
gotcha above) -- the new branches were written to the same
`EnableDelayedExpansion`/`!VAR!`-inside-blocks discipline as the rest of
the file (including the exact bug class a previous round of this diary
already found and fixed in this same script), but ask the user to
smoke-test `goclang++.bat` themselves before relying on it.

**2026-09-03, same day, much later still -- a real full cutover, two
real UB bugs in `go func(){...}()`, and GocVM goes non-blocking.**
Asked "are we ready for a release," the honest answer was no for a
reason with nothing to do with code quality: `~/go++` had never been a
git repository at all -- this entire day's work (everything above,
across many sessions) lived only as loose files on one machine, and
`~/WASIGo++` (the real `goxxlang/wasigoc` checkout, already tagged
`v0.2.1` that same morning, before any of this existed) had no idea any
of it existed either. Per explicit direction, did a full cutover
instead of a selective merge: mirrored `~/go++` over `~/WASIGo++`
wholesale (keeping `.git`), dropping the two packages (`go/build`,
`go/build/constraint`) that only ever existed in `WASIGo++` -- "don't
care about the old code." Committed. `shim_sandbox` had the identical
problem (~2000 lines of real Winsock/Win32 backend work, `git log`
showed exactly one commit) and got the same treatment once it became
clear the async bridge below depends on it.

Investigating a since-fixed `runtime_smoketest` crash (real root cause:
`CMAKE_BUILD_TYPE=Release`'s `-DNDEBUG` had been silently compiling out
every `assert()` in that file all along -- "278/278 passing" was never
a real check of it) surfaced a genuine, general, previously-latent
compiler bug: `EmitGo` compiled `go func(){ <uses channels> }()` to
`wasigo::go((<closure>)());` -- immediately invoking the closure and
handing the resulting (initially suspended) Task to `go()`. A lambda
coroutine's frame stores only a pointer back to its own closure
("this"); capture mode doesn't change that. The temporary closure is
destroyed at the end of that expression, but the Task is only initially
suspended -- the scheduler resumes it later, reading through a by-then-
dangling pointer. `grep` confirmed zero existing stdlib/example source
used this pattern, so -- same shape as the `go recv.AsyncMethod(...)`
bug earlier in this diary -- it was simply never exercised before.
Fixed with two new `runtime.hpp` helpers, `GoAsyncLit`/`GoAsyncLitT`,
that take the closure itself (uninvoked) as a genuine by-value coroutine
parameter -- frame-owned, survives suspension -- then invoke and
`co_await` it internally; `EmitGo` routes an async go-target literal
through these, and defers a non-async one via the existing `[=]{...;}`
wrap-and-call-later shape every other synchronous go target already
uses (which also happened to fix a real "invalid use of void
expression" compile error the old form hit for `go func(){ <no
channels> }()`, likewise never previously exercised). Second bug found
fixing the first: `EmitFuncLit`'s async branch captures by value (`[=]`)
but was never marked `mutable`, so any non-const method on a captured-
by-value object (`Chan::send` -- called by every channel-using async
literal) failed to compile with "discards qualifiers." `tests/
runtime_smoketest.cc` had the hand-written version of the same dangling-
closure pattern in `pingpong()`/`returning_task()` -- fixed the same
way -- plus its own `boom_local()` bug: `assert(order == "R01")` was
checked before `DeferList`'s destructor (what actually runs the
deferred closures) had fired. All verified for real this time: built
directly with asserts live (no `NDEBUG`) at both `-O0` and `-O2`,
repeated runs, clean.

With those real bugs out of the way, tackled a real architectural one:
`gocvm::Call` is a synchronous, blocking call into `HostBridge::Call` --
since wasigo's scheduler is single-threaded cooperative with no OS
threads backing it, one goroutine's slow or indefinitely-blocking host
call (a socket `recv()` with no data yet, a subprocess that hasn't
exited) stalled *every other* ready goroutine too, not just its own.
Built the non-blocking path the original `VThread` comment ("not a
currently-live state machine") had left for later: a new
`AsyncHostBridge` (`Submit`/`PollOne`/`WaitOne`) and `gocvm::CallAsync`,
a real awaitable -- suspending it registers a genuine `VThread`
(`State::kAwaitingHost`) via the same registry every `go()`-spawned
goroutine already uses, and `Scheduler::run()` drains completions after
every resume, blocking on the bridge for one completion (rather than
declaring deadlock or busy-spinning) only once the ready queue is truly
empty but async work is outstanding. `cpp_generator.cc`: `gocvm.Call` is
now a recognized await point (`ExprNeedsAwait` didn't know about it at
all before -- `IsImportedPackage` explicitly excludes the `gocvm`
builtin, so the existing `pkg.Func` async-inference branch could never
have caught it), and compiles unconditionally to `co_await
wasigo::gocvm::CallAsync(...)`, propagating transitively through the
existing async-inference machinery exactly like any other await-needing
call with no other compiler changes needed -- verified: `syscall.
Getpid()` becomes `TaskT<int64_t>`, its caller becomes a coroutine
automatically, all the way up to `main()`. `shim_sandbox`'s
`gocvm_bridge.cc` got a matching `AsyncSapiBridge`, backed by exactly
ONE worker thread (deliberately not a pool): preserves the same
serialized access to shim_sandbox's own internals (the handle table
especially) the old synchronous single-cooperative-thread model already
relied on, just moved onto its own OS thread so the scheduler's own
thread is never blocked by it.

Verified the ordering claim directly, not just "it compiles": a new
`gocvm_async_ordering()` test uses a fake bridge that only ever answers
from the scheduler's *blocking* `WaitOne` path, and asserts a second,
unrelated goroutine actually runs to completion *before* the awaiting
goroutine's request is answered (`order == "BA"`, never `"AB"`) --
impossible to produce under the old blocking `Call`. Verified with real
compiled programs too: `syscall.Getpid()` round-trips a real PID through
the real worker-thread dispatch end to end; a goroutine running `exec.
Command("ping", "-n", "3", "127.0.0.1").CombinedOutput()` (a real ~2.1s
subprocess, real exit code, real output) does not block `main()`, which
prints its own next line immediately and only blocks on the result
channel -- the whole program still takes the real ~2.1s (nothing
faked), but that time overlaps with `main` instead of preceding it.
Full `ctest` both repos: `go++`/`WASIGo++` 278/278 (including every
existing gocvm-wired package's golden test on the bridge-less
wasm32-wasip1 path, confirming `CallAsync`'s immediate no-bridge/ABAC-
deny branch still produces byte-identical fallback output), `shim_sandbox`
2/2.

**Same day, one more round -- real thread-safety for Chan/Map/gc::Heap,
and a genuine GC concurrency bug found and fixed.** Asked why GocVM
couldn't just use real OS threads throughout (not just the one bridge
worker thread): wasm32-wasip1 has no threading at all in this project's
target configuration (no wasi-threads dependency, by design -- see
Scheduler's own comment), and even on native, `Chan`/`Slice`/`Map`/the
GC heap have zero synchronization today because exactly one thread ever
touched them. Asked to build that synchronization out as prerequisite
work (not something today's behavior depends on).

Scoped deliberately per-type, matching what real Go actually
guarantees rather than blanket-locking everything: **Chan\<T\>** gets
real locking (Go guarantees channel safety across goroutines) --
`State::mu`, each plain awaiter re-checking readiness under the lock
inside `await_suspend` (C++20's "return `false`, don't actually
suspend" signal closes the `await_ready`-releases/`await_suspend`-
reacquires race window), `GSelect` locking every participating
channel together for its whole check-then-park sequence, address-
sorted the same way real Go's own `selectgo` orders its channel locks
so two concurrent selects sharing channels can never deadlock each
other. **Map\<K,V\>** gets NO locking -- real Go doesn't protect maps
either (concurrent access is documented UB there too) -- instead the
same *failure mode*: a `hashWriting`-style atomic flag that panics
("fatal error: concurrent map writes"/"...read and map write") on
detected contention instead of silently corrupting, matching real Go's
own detect-and-crash contract rather than being more forgiving than
the language it's modeling. **Slice\<T\>** gets nothing at all,
deliberately: real Go slices have zero runtime protection either, and
adding locking here would be a parity regression (Go++ programs
racing a slice would silently "work" where the equivalent real Go
program is genuine UB). **`gc::Heap`/`Persistent<T>`**: mutex-protected
`Make`/`AddRoot`/`RemoveRoot`/`Collect`, `Collect()` holding the lock
for its entire mark-sweep pass, `Persistent<T>`'s every mutator taking
the lock for its whole detach+reassign+attach sequence (not two
separate acquisitions) so a concurrent `Collect()` can never observe a
root mid-update.

That locking alone was not sufficient. A real multi-threaded stress
test (`tests/sync/sync_stress_test.cc`, new -- 8 producer/4 consumer
threads hammering a `Chan` via `try_send`/`try_recv`, 8 threads doing
concurrent `Make`+`Persistent`+`Collect`, 8 threads hammering the
`Map` guard's atomic flag) found `Chan` and the `Map` detector
genuinely solid (160,000 real items with matching sums; 1.6M
contention attempts correctly detected) but the GC heap crashed
roughly 1 run in 8-15 at real scale (8 threads x 5000 allocations,
`Collect()` running while other threads actively allocate). Root-caused
via `gdb` across a sequence of narrowing repros (a fully standalone
reproduction using plain classes with no dependency on any real
`gc::`/`Persistent` code reproduced the identical crash, ruling out
anything specific to this project's templates or inheritance depth;
the crash only appeared once a virtual dispatch was added to the mark
loop, which turned out to matter only because it widens the timing
window, not because virtual calls are special): **`Heap::Make<T>()`
publishes its result into `objects_` -- visible to *any* thread's
`Collect()` -- before the caller has any chance to root it.** In the
original single-cooperative-thread model this window was categorically
unreachable (nothing else could ever run between `Make()` returning and
the caller rooting its result), so the bug is genuinely new to real
concurrency, not a pre-existing latent one exposed by testing harder.
A concurrent `Collect()` on another thread can legitimately see the
freshly-made object as unreached-from-any-root during that window and
sweep it, hanging the allocating thread a dangling pointer the instant
it tries to root or use it -- exactly the "vtable for the wrong type"
symptom `gdb` showed, from memory freed out from under a still-in-use
pointer and reused for something else.

Fixed with `Heap::MakeRooted<T>()`: allocates and roots in the SAME
critical section (a new private `Persistent<T>(T*, AlreadyLockedTag)`
constructor, `friend`ed to `Heap`, that skips `Persistent`'s own
locking since the caller -- `MakeRooted` -- already holds it), so
there is now no instant where an object exists in `objects_` without
also already being reachable through the `Persistent<T>` handed back.
`gocvm::RegisterThread()` (the one production caller of the old
`Make<T>()`-then-root-separately shape) switched to it; `Make()` itself
is kept, with the hazard documented prominently on it, for the
single-scheduler-thread case where the window can't be hit -- not
removed, since removing it would be scope creep on top of prerequisite
work nothing yet exploits. Verified: the exact 8-thread/5000-allocation
scenario that reproduced the crash ~1 run in 8-15 went to 30/30 clean
after the fix, then 10/10 on the full three-part stress test.

Full `ctest` both repos (279 = the existing 278 + the new
`sync_stress_test`): 279/279. `runtime_smoketest` reconfirmed with real
asserts (no `-DNDEBUG`) at both `-O0` and `-O2`, repeated runs, clean.

**Same day, one more small round -- map a VThread to the real OS thread
that served it.** `VThread` gains `os_thread` (a `std::thread::id`),
and there's a new global, mutex-protected `gocvm::OSThreadFor(vthread_id)`
lookup by plain numeric id (useful when a caller -- logging, metrics --
only has the id, not the VThread pointer, and the mapping needs to
outlive the VThread's own collection). `AsyncHostBridge::Completion`
gained `worker_thread`; `apply_completion()` records it on both the
VThread and the global map whenever a bridge sets it (default
`std::thread::id()`, "none", for a bridge that never does). shim_sandbox's
`AsyncSapiBridge` sets it via `std::this_thread::get_id()` in its worker
thread before handing a completion back -- trivial with exactly one
worker thread, but the plumbing doesn't assume that stays true.

Verified two ways: a new `tests/runtime_smoketest.cc` case
(`gocvm_vthread_maps_to_real_os_thread`) uses a fake bridge backed by a
REAL `std::thread` (unlike the existing `FakeAsyncBridge`, which
answers synchronously from the scheduler thread) and confirms
`OSThreadFor` returns a real, non-null id that is neither the calling
thread's nor a placeholder -- specifically the fake's own worker
thread. Then end to end for real: a hand-built program linking the
actual `shim_sandbox` `AsyncSapiBridge`, doing a real `syscall.getpid`
gocvm call, confirming the serving VThread maps to a real, distinct OS
thread, not a test double. `CallAsyncAwaiter` gained a `vthread_id`
field (populated in `await_suspend`) so a caller keeping a named
awaiter (`auto a = CallAsync(...); r = co_await a;`) can look this up
afterward -- generated code never needs it, only introspection like
this.

**Same day, last round -- reflect catches up with generic structs and
named array/slice/map types.** Asked to make `reflect` recognize this
project's own recent type-system work. Found: `EmitReflectDescribe`
returned immediately for any struct with type parameters, so a generic
struct (`Pair[int]{...}`) got zero reflection metadata at all --
`reflect.TypeOf(p).Name()`/`.NumField()`/`.Field(i)` silently reported
"not a struct" rather than erroring, since `has_reflect_describe<T>` is
a compile-time trait and a missing overload just makes the checking
branch take the false path. Fixed by emitting `wasigo_reflect_describe`/
`wasigo_reflect_typename` as templates over the struct's own type
parameters instead of skipping them -- the existing ADL + `void_t`
trait detection in `runtime.hpp` finds a template overload exactly the
same way it finds a concrete one, no changes needed there.

A named type wrapping `[]T`/`[N]T`/`map[K]V` with at least one method
(`type IntList []int; func (l IntList) Sum() int {...}`) never got
this treatment either -- it goes through a completely separate code
path (`EmitAliases`' wrapper-struct branch), so `Name()` came back `""`
for every such type. Fixed the same way, plus a new
`static constexpr int wasigo_reflect_kind` member (a new
`has_reflect_kind_override` trait reads it, checked before the
existing struct check in `kind_of<T>`) so `Kind()` reports the real
Slice/Array/Map kind instead of falling through to Invalid -- `RKind`
gained `Array`/`Map` values for this, appended at the very end since
the existing values are matched by POSITION against `reflect`'s
Go-visible constants and inserting anywhere else would silently
renumber everything after it.

Investigating this surfaced two real, more fundamental bugs in the
same wrapper-struct machinery, independent of `reflect` entirely:
indexing (`l[0]`) and ranging (`for _, v := range l`) over a named
slice/array/map type with methods didn't compile at all. The wrapper
struct only exposes an implicit conversion to the underlying type;
`operator[]` and range-for's own `.size()`/`begin()`/`end()` lookup
don't consider a class's conversion operators the way arithmetic/
comparison operators do, so neither was ever found. First fix attempt
cast through the conversion operator (`Slice<T>(x)`) to make these
resolve -- compiled, and worked for the `Slice`-backed case (shared
storage under a `shared_ptr`, so a copy of the handle still points at
the same data) -- but a conversion operator returns BY VALUE, and for
the plain no-method named-array case (`type Block [64]int32`, already
a transparent `using` alias with no wrapper struct at all, fine before
touching anything) the fix applied the SAME cast anyway, silently
writing an indexed assignment into a throwaway copy: `b[0] = 42`
no-op'd `b`. `typedecl_golden` and `jpegpkg_golden` (named byte arrays
without methods, used heavily for pixel/DCT data) both caught this
immediately as a full `ctest` regression -- exactly the methodology
this diary keeps using, just against this session's own change instead
of a fresh compile target. Fixed properly by gating on `HasMethodsOn`
(the exact predicate `EmitAliases` itself uses to pick the wrapper-
struct path over a transparent alias) and indexing/ranging through the
wrapper's own `v` member directly instead of the conversion operator --
a real reference into the actual storage, correct for both the
`shared_ptr`-backed and value-array-backed cases, verified explicitly
with a named ARRAY type that has a method (write-through indexing then
ranging over the same variable), the one case that would have caught
the copy-vs-reference distinction on its own.

Verified: `reflect.TypeOf`/`NumField`/`Field`/`FieldName` on a real
`Pair[int]` instance; `Name`/`Kind`/indexing/ranging on a real
`IntList`; write-through indexing and ranging together on a named
array type with a method. Full `ctest` (both repos): 279/279, including
`typedecl_golden`/`jpegpkg_golden` themselves confirming the earlier
copy-vs-reference regression is gone.

**Same day, one more round -- `encoding/json` can Unmarshal into a
slice-typed struct field.** `Marshal` of a struct with a slice or
named-slice field (`Wrapper{Items IntList, Names []string}`) already
worked, via the existing `finish_any_kind<T>`/`is_wasigo_slice<Slice<T>>`
mechanism. `Unmarshal` back into the same struct did not: `decodeReflect`
in `stdlib/encoding/json/json.go` had no `reflect.Slice` case at all, so
it fell through to the generic "unsupported Unmarshal target" error --
and because a struct's field loop returns on the FIRST field error, one
unsupported slice field killed decoding of every other field in the
struct too, not just its own.

Fixed with the same shape `SetInt`/`SetString`/etc already use: a new
`Any::slice_set_fn` function pointer (populated in `finish_any_kind<T>`
alongside the existing `slice_len_fn`/`slice_index_fn`, using
`typename T::value_type` as the element type -- `Slice<T>` gained a
`value_type` alias for this) and a public `Any::SetSlice(const
Slice<Any>&)` wired to it, exposed to Go as `reflect.Value.SetSlice(elems
[]any) bool`. Unlike `SetInt`/etc, it returns `false` instead of
panicking on failure, since `encoding/json` needs to turn "an element
wasn't a scalar this slice's type accepts" into a real Go error for one
field deep inside a larger struct, not an abort of the whole program.
Element coercion (`try_coerce_json_any<Elem>`) mirrors `decodeReflect`'s
own per-kind checks, just done once per slice element at the C++ level:
JSON's generic decode shape always boxes a number as `float64`, so an
`int`/`int64`/`float32` slice element all coerce from the same
`RKind::Float64` box. Deliberately out of scope: a slice of structs or a
slice of slices -- `try_coerce_json_any` returns `false` for both,
surfacing as a real `json: unsupported slice element type` error rather
than a crash.

Verified: a `Wrapper{Items IntList, Names []string}` round trip through
`Marshal`+`Unmarshal` -- previously `Unmarshal` returned a non-nil error
and left both fields at zero, now returns `nil` with both fields
correctly populated (compiled and run directly, not via a golden test).
Full `ctest` re-run for regressions.

**Same day, later still -- two claimed-permanent language restrictions
actually fixed for real: a non-literal `fmt.Printf`/`Sprintf`/`Fprintf`/
`Errorf` format string, and `return` inside range-over-func.** Both had
stood as documented walls (see this file's "Supported"/limits section
and `docs/language.md`) for long enough that a claim they'd been fixed
was checked directly against the running `src/` source (parser.cc:1649,
cpp_generator.cc's `EmitPrintf`/`EmitReturn`, `runtime.hpp`) before doing
anything else -- both restrictions were still genuinely enforced in the
code, not just stale docs. Implemented both:

*Non-literal format string*: `EmitPrintf`/`EmitFprintf`/`EmitErrorf`
already parse a literal format string's own verbs at compile time
(unchanged, still the fast/checked path). When the format expression
isn't a `StringLit`, they now emit a call to a new runtime
`wasigo::FormatPrintf(fmt, std::vector<Any>)` instead of erroring --
each remaining arg gets boxed via the existing `EmitAdapt`/`Any::adapt`
machinery, and `FormatPrintf` walks `%d %s %f %v %t %c %w %%` at runtime
the same way the compile-time loop does, streaming each `Any` through
its own `operator<<`. `Errorf`'s `%w` in this path renders as plain text
(no wrap tracking -- which arg is `%w` depends on the runtime string's
own content, not something codegen can see) rather than erroring, a
real documented narrowing not silently dropped. First attempt missed the
actual motivating case entirely: a `log.Printf`-shaped wrapper
(`func Logf(format string, v ...any) { fmt.Printf(format, v...) }`)
compiled and RAN, but silently produced wrong output (`a=<any> b=%s`
instead of `a=5 b=hi`) -- the `v...` spread was being boxed as ONE `any`
(the whole `Slice<Any>`) instead of being expanded into its own
elements, since the per-arg loop had no special case for `ellipsis`.
Caught by testing that exact scenario, not by review. Fixed with a new
`wasigo::AnyVectorFromSlice(const Slice<Any>&)` and an explicit
`args.back()->ellipsis` check in the new `EmitDynamicFormatCall`,
mirroring the existing ordinary-variadic-call spread handling. Landed
`log.Printf`/`Fatalf`/`Panicf` in `stdlib/log` on top of this, the
concrete case that had been cited as categorically impossible.

*`return` inside range-over-func*: a range-over-func loop's body compiles
to code running inside the yield lambda passed to the sequence function
(`EmitRangeOverFunc`) -- a literal `return` there would return from that
lambda (wrong type, wrong effect), which is exactly why this used to be
a hard `Error()`. Fixed the way real Go's own compiler desugars this:
`JumpFrame` gained `rf_ret_var`/`rf_val_var` string fields; the
OUTERMOST range-over-func loop reachable declares a fresh `bool` flag
(and, unless the enclosing function returns nothing, a value slot typed
via `ReturnCppType`) right before its own call; a `return` inside (found
by `EmitReturn` walking `jump_stack_` for the nearest `range_func`
frame) stashes its value into that slot and does `return false;` to stop
iterating instead of a real C++ return; right after each range-over-func
call, a check sees whether the flag got set and, if this loop is itself
nested inside another one, propagates by doing the SAME `return false;`
(we're still lexically inside that outer yield lambda) -- only the
actual outermost level does the real C++ `return`/`co_return`. One
shared flag/value pair per outermost loop, inherited (not
re-declared) by anything nested inside it, so arbitrarily deep nesting
escapes one level at a time. `main()`'s implicit `int` return (despite
declaring zero Go results) is special-cased the same way `EmitReturn`'s
own bare-`return` branch already does. A real lifetime hazard avoided
during implementation: the post-call check originally held a
`JumpFrame&`/pointer taken before emitting the loop body, but
`EmitStmtList(s.body)` can push/pop `jump_stack_` (a `std::vector`) for
nested loops, which can reallocate it -- switched to copying the two
variable-name strings into locals before emitting the body, used after.

Verified end to end, not just "compiles": `fmt.Sprintf`/`Printf`/
`Errorf` with a variable format string; the exact `log.Printf`-shaped
wrapper scenario (both the broken-before-the-ellipsis-fix output and the
corrected `a=5 b=hi` after); a single-level range-over-func `return`
(`for v := range Seq { if v > n { return v } }`); and a NESTED
range-over-func `return` (`for a := range Seq { for b := range Seq { if
a+b==7 { return a*100+b } } }`, confirming propagation through two
levels lands the right value, not just stops the inner loop). Full
`ctest`: 283/283, both before and after the `ellipsis` fix (the broken
intermediate state was caught by manual testing, never by `ctest` --
existing goldens don't happen to exercise this exact spread shape,
worth remembering if this code is touched again).

**Same day, last round -- panic/recover actually works in async
functions now, four real bugs deep, plus a severe regression found and
fixed before it ever got committed.** Started from a specific, correct
observation: `recover()`'s pending-panic chain (`g_panic_frame`, a flat
`thread_local`) is exactly the kind of state the VThread/goroutine
thread-safety work earlier this session was about, and it had never
actually been made safe. Two real goroutines each holding a live,
un-popped `PanicFrame` (possible any time a function with `defer`
suspends mid-body -- a channel op, an async gocvm call) could scramble
each other's `prev` links across a scheduler switch, or hand `recover()`
one goroutine's pending panic while running as a completely different
one.

Fixed for real: each `gocvm::VThread` now owns its own `panic_head`
chain instead of sharing one flat per-OS-thread chain.
`gocvm::current_vthread()` (Meyer's-singleton `thread_local`, from the
start this time -- see `g_error_state`'s own linker-bug history) tracks
whichever goroutine is actually executing, saved/restored around every
`Scheduler::run()` resume and captured at every point a goroutine parks
itself (`Chan`'s three awaiters, `CallAsyncAwaiter`, `Task`/`TaskT`'s own
`await_suspend`) so the right chain gets restored on resume regardless
of what ran in between. `PanicFrame`'s ctor/dtor and `recover()` route
through `current_panic_head()` (the current VThread's own chain, or the
old flat chain as a fallback for code that isn't running as any spawned
goroutine -- `main()` itself, in particular).

Checking whether this bug was even reachable surfaced the real, bigger
gap: `panic("literal")` inside a function with `defer` only ever got the
`__pf.has_pending = true; goto __wasigo_end` treatment (letting a
deferred `recover()` catch it) when the function was **not** async --
gated that way since the mechanism was originally setjmp-based, and
"setjmp cannot land in a C++20 coroutine frame." The actual mechanism
today is a plain structured `goto`, not setjmp, and a goto within one
function is exactly as valid inside a coroutine body as outside one --
so panic/recover inside a goroutine's own body had simply never worked
at all, independent of any threading concern, since only a non-async
function could ever hold a `PanicFrame`. Opened the gate.

Doing that surfaced two more real, previously-unreachable bugs (unreachable
because nothing could get a `PanicFrame` in an async function before):
a `goto` firing before a function's own trailing `return` skips that
return's `co_return`/`return` entirely, and if the function's *textual*
last statement WAS a return (a common shape: a function ending in
`panic(...)` needs no explicit trailing return in real Go, since `panic`
is itself a spec "terminating statement"), nothing else supplied a
fallback -- for a `TaskT<T>` coroutine (no `return_void()`), that's a
hard "fell off the end of a non-void coroutine" compile error; for a
plain sync function, it was a real (if less loud) "no return statement"
UB gap that happened to work by luck before now, via whatever the
optimizer's NRVO did or didn't do. Fixed both (async and sync) with an
explicit fallback after `__wasigo_end:`, always present whenever `wrap`
(has defer) is true regardless of whether the last statement looks like
a return, since a panic earlier in the body can bypass it either way.

That, in turn, surfaced a THIRD, more fundamental bug, present even with
no panic involved at all: a named result a `defer` modifies (the
standard `func f() (result T) { defer func(){ recover(); result = ... }
() ; ... }` idiom -- or just as often a defer plainly appending to a
result) silently discarded the defer's change in an async function.
`co_return result` copies `result`'s value into the promise via
`return_value(T)` **before** any local's destructor runs (`return`/
`co_return` are specified that way) -- `~DeferList()`'s own defer-running
happens strictly after, too late. The sync case happened to still work,
but only because NRVO (not guaranteed by the standard) makes `result`
and the actual return slot the same storage the whole time -- fragile,
not actually correct. Fixed generally: `DeferList::RunAll()`/new
`RunAllAwait()` are called EXPLICITLY, before the named result is read,
at every return site with named results (`EmitReturnWithDefers` in
cpp_generator.cc) -- for a non-bare `return expr`, that means assigning
into the named result first, THEN running defers, THEN returning it,
matching real Go's actual defer/named-result order for both bare and
explicit-value returns, sync and async alike.

A FOURTH bug, also panic-independent: `defer func(){ <-ch }()` --a
deferred closure whose own body uses channels -- silently never ran its
body at all. Same root cause class as this session's earlier `go
func(){...}()` dangling-closure bug: the closure is itself a
Task-returning coroutine, always suspended at `initial_suspend()` until
something resumes it, and the old codegen just called it and discarded
the result (`~DeferList()`'s plain `n->run()`), abandoning it suspended
forever. Fixed with `DeferList::AsyncImpl`/`push_async` (mirrors
`Impl`/`push` but the closure is `co_await`ed, not called-and-dropped)
and two driving paths: an ASYNC enclosing function `co_await`s
`__defers.RunAllAwait()` directly (added at every exit point that
previously relied on the destructor); a SYNC enclosing function (which
cannot `co_await` at all -- Go allows deferring an async closure from a
plain function with no channels of its own) spawns the closure as a real
goroutine and drives just that one to completion via a new
`wasigo::RunUntil(bool* done)`, which pumps the scheduler's own
resume/vthread-restore loop but stops as soon as that one flag flips --
deliberately NOT `wasigo::run()`, which drains every pending goroutine
in the whole program, not just this defer's.

**Before any of this landed, a severe regression in the fix itself, found by full-suite testing, not review.** `DeferList`'s full definition had been moved to right after `struct Task` (needed complete for the async pieces) -- which put its ENTIRELY UNRELATED synchronous core (`Node`/`Impl`/`push`/`RunAll`/`~DeferList()`, no `Task` involved at all) inside the pre-existing `#ifdef WASIGO_NEED_CORO` block, a real, load-bearing optimization gate: wasigoc only compiles Task/Chan/Scheduler in for a program whose transitive source actually uses channels/goroutines somewhere, and plain `defer` has always been completely independent of that. Broke `database/sql` first (`sql.go` uses ordinary `defer`, no channels anywhere in the whole `sqlpkg` example's transitive closure) with a baffling "aggregate has incomplete type" error deep inside a generated header, despite `DeferList` being textually complete earlier in the very same translation unit. Root-caused via bisection: an isolated `runtime.hpp` + the failing header compiled FINE on its own; only the full ~3400-line prefix reproduced it; `-E` preprocessor output confirmed the ENTIRE coroutine section -- Task, TaskT, Scheduler, DeferList, all of it -- was silently absent whenever the compiling package didn't need `WASIGO_NEED_CORO` itself. Fixed by splitting `DeferList` back apart: `Node`/`Impl`/`push`/`RunAll`/`~DeferList()` (needs nothing from Task) stayed at DeferList's ORIGINAL early location, unconditionally compiled; `run_await()` (the one non-template virtual with a body, needed on `Node` so `RunAllAwait()` can call it polymorphically regardless of whether a given defer is sync or async) is declared there but its body defined out-of-line, later, once Task is real -- same declare-early/define-late split this session already used for `PanicFrame`'s ctor/dtor; `AsyncImpl`/`push_async`/`RunAllAwait()` (needs Task by name in their own signatures) are wrapped in their own `#ifdef WASIGO_NEED_CORO` inside the early class body, matching the exact condition under which Task exists at all -- always true wherever an async defer could exist in the first place, since the deferred closure's own channel use is what turns WASIGO_NEED_CORO on for that whole program.

Verified with six real compiled-and-run Go++ programs, not just `ctest`
(which never happened to exercise this shape before either): a single
goroutine parking on a channel then panicking, caught by its own
deferred `recover()`; the ORIGINAL two-goroutine scenario that started
this whole investigation, each with its own defer/panic/recover,
interleaved through the scheduler, confirming real isolation (`worker1
recovered: boom from worker1` / `worker2 no panic`, never crossed); a
named-result function ending in bare `panic()` (no trailing return) with
a synchronous defer, both async and sync; a plain async function (no
panic at all) whose defer appends to a named result; and a SYNC function
deferring a channel-using closure with no async anything else involved.
Full `ctest`, both repos: 283/283 -- including, after the regression fix,
`database/sql` and every other plain-`defer`-no-channels package that
briefly stopped linking.

### Tracker (`go list std` minus `internal/`)

Status: **in** = present (see tables above; still partial), **todo** = not
started, **rt** = needs runtime/WASI not just Go source, **n/a** = does not
map (no dynamic load, no cgo, no race detector, no host syslog).

| Package | Status |
| --- | --- |
| `archive/tar` | **in** (USTAR format, bounded: regular files only, `Header{Name, Mode, Size, Typeflag}` -- no uid/gid/mtime/uname/gname/devmajor/devminor/prefix, written as zero/empty, still a valid real USTAR header. `Writer`/`Reader`. Verified bidirectionally against Python's own `tarfile` module -- an archive written here read correctly by `tarfile`, AND an archive written by `tarfile` read correctly here -- real interop, not just self-consistency) |
| `archive/zip` | **in** (bounded: local file header + central directory + EOCD, entries always DEFLATE-compressed via `compress/flate` (method 8) -- no Store method, no directories, no ZIP64. `Writer.Create`/`Close` match real Go's actual shape (`Create` returns `io.Writer`, buffered until the next `Create`/`Close` -- same "buffer, compress once" bound as flate/zlib/gzip). `NewReader` takes a `[]byte` directly rather than real Go's `io.ReaderAt`+size (no ReaderAt interface here), scanning backward for the EOCD signature exactly like a real zip parser does. `File.Open` verifies the entry's CRC-32 at EOF, same bound as gzip/zlib's Reader. Verified genuinely bidirectionally against Python's own `zipfile` module: this package's own archive opened by `zipfile.ZipFile`, `testzip()` confirming both entries' CRCs are genuinely valid (not just self-consistent), and both files' content read back correctly; and a real archive written by `zipfile.ZipFile(..., ZIP_DEFLATED)` (two entries, one with real repetition) opened and both entries decompressed byte-for-byte correctly by this package's `Reader`) |
| `bufio` | **in** (line-oriented `Scanner`, small buffered `Writer`) |
| `bytes` | **in** (partial) |
| `cmp` | **in** |
| `compress/bzip2` | **in** (decompression only, same as real Go's own package -- bzip2 has never had a Go encoder in the standard library either. Bounded further than real Go: single-member streams only (no concatenated continuation members), matching a normal one-shot compressor call's output. Ported from real Go's own algorithm (there's no RFC for bzip2, same "guessing plus the Wikipedia page" precedent real Go's own package comment states): MSB-first bit reader, canonical-Huffman-tree-by-recursive-bisection matching real Go's `newHuffmanTree`/`buildHuffmanNode` (using a plain insertion sort in place of real Go's `slices.SortFunc`, which this project's bounded `slices` doesn't have), move-to-front decoding, RUNA/RUNB run-length decoding of the MTF zero-symbol, the "single array" inverse Burrows-Wheeler transform, and the initial RLE1 pass -- reimplemented as one direct pass over the fully un-BWT'd block rather than real Go's incremental per-Read-call state machine, since this port decodes the whole stream eagerly at `NewReader` time (same "buffer, don't stream" bound as `compress/flate`/`gzip`/`zlib`). The block/file CRC-32 variant (bit-reversed shift direction) is checked, not skipped. Verified against real bzip2 data produced by Python's own `bz2` module: a small 2,360-byte input (this package's checked-in golden test) plus, separately, a larger 362,749-byte input at a smaller block size forcing 4 real blocks with a live Huffman-tree-selector switch mid-block -- both decoded byte-for-byte identical to the original. Found two real, general, previously-latent gaps building this: (1) `wasigo::Slice<bool>` backs onto `std::vector<bool>`, whose `operator[]` returns a bit-packed proxy instead of a real `bool&`, breaking this project's generic `Slice<T>::operator[]` -- `[]bool` is rare enough in Go that no earlier package had exercised it; worked around here with `[]byte` (0/1) instead of changing the shared `Slice<T>` template for this one rare case; (2) `io.ErrUnexpectedEOF` was simply missing from this project's `io` package (ordinary stdlib source, not compiler-level) -- added it alongside `io.EOF`) |
| `compress/flate` | **in** (real DEFLATE, RFC 1951: `Reader`'s `inflate` handles all three block types -- stored, fixed Huffman, AND dynamic Huffman -- via one shared canonical-Huffman-construction function (`buildHuffman`, RFC 1951 3.2.2's bl_count/next_code algorithm) used for both the fixed tables and per-block dynamic tables, not two separate implementations. `Writer`'s `deflate` always emits ONE final block using real greedy LZ77 (hash-bucketed by 3-byte prefix, up to 32 most-recent candidates per bucket, window 32768, min match 3/max match 258) PLUS fixed Huffman coding only -- never dynamic tables, so compression ratio trails real Go's own flate, but the bitstream is fully standard. `level` is accepted (`NewWriter(w, level)`, for gzip/zlib call-site compatibility) but has no effect, same bound as `time.Sleep`. `Writer` buffers the whole input and compresses once at `Close`, not real streaming, same bound as `encoding/csv`'s `Reader`. Verified genuinely bidirectionally against Python's own `zlib` module, not just self-consistency: (1) round trip through this package's own Writer+Reader across empty/single-byte/repetitive/mixed inputs; (2) this package's OWN compressed output, fed to Python's `zlib.decompressobj(-15)`, decoded byte-for-byte correctly -- proves the encoder emits a genuinely standard bitstream; (3) real dynamic-Huffman deflate data PRODUCED by Python's `zlib.compressobj` (confirmed via the block-type bits: `btype=2`) on a 12,513-byte randomized-word input, decoded byte-for-byte correctly by this package's `Reader` -- proves the decoder's dynamic-Huffman path is genuinely correct against an independent real implementation, not just internally consistent; (4) a Python-produced stored block (`btype=0`, `level=0`) also decoded correctly) |
| `compress/gzip` | **in** (RFC 1952: 10-byte fixed header (magic/CM/FLG/MTIME/XFL/OS) wrapping a raw `compress/flate` stream, trailed by an 8-byte little-endian CRC-32 + ISIZE. `Writer` always emits the minimal header (no `Header.Name`/`Comment`/`Extra`/`ModTime` support) -- but `Reader` DOES parse and skip FEXTRA/FNAME/FCOMMENT/FHCRC when present, so it reads a real gzip file with any of those set, not just this package's own output. Same eager-decode trailer-positioning approach as `compress/zlib` (see its comment). Verified genuinely bidirectionally against Python's own `gzip` module: this package's own output decoded correctly by `gzip.decompress`; and a real gzip file written by Python's `gzip.GzipFile` (`FLG=8`, FNAME set to the temp filename -- confirmed by inspecting the flag byte before testing, not assumed) decoded byte-for-byte correctly by this package's `Reader`, proving the FNAME-skip path works against a real producer, not just a hand-crafted test) |
| `compress/zlib` | **in** (RFC 1950: 2-byte CMF/FLG header, mod-31 checksummed, wrapping a raw `compress/flate` stream, trailed by a big-endian Adler-32 over the uncompressed bytes. No preset dictionary (`FDICT`), no `NewWriterLevel`/`NewReaderDict`. `Reader` reads the 4-byte trailer immediately after constructing the underlying `flate.Reader` -- safe because `flate.Reader` decodes its whole body eagerly at construction (see flate's own bound), so by the time it returns the underlying stream is already positioned exactly at the trailer, with any sub-byte padding from the last deflate block correctly left behind. Verified genuinely bidirectionally against Python's own `zlib` module: (1) round trip through this package's own Writer+Reader, plus a deliberately corrupted trailing byte correctly caught as a checksum mismatch; (2) this package's own compressed output decoded correctly by Python's `zlib.decompress` (which itself validates the header checksum AND the Adler-32 trailer -- not a bounded/partial check); (3) a real header+dynamic-Huffman-body+trailer stream produced by Python's `zlib.compress` (1,263 bytes compressing a 10,691-byte randomized-word input) decoded byte-for-byte correctly by this package's `Reader`) |
| `compress/lzw` | **in** (classic variable-width LZW -- clear/EOF control codes, growing up to 12-bit codes -- but NOT the GIF-specific "early change" code-width-bump-one-code-early quirk; real Go's own package doesn't implement that either, so this isn't a deviation, the same documented boundary. `NewReader`/`NewWriter` match real Go's actual `io.Reader`/`io.Writer` + `Order` + litWidth signature exactly -- LZW is pure byte-stream compression, no arbitrary-type marshaling, so unlike `encoding/gob`/`encoding/asn1` there's no reflection wall here. `Writer` streams one byte at a time through the real incremental-parse dictionary (not slurp-then-process). Verified by round trip (encode then decode recovers the original bytes exactly) across every litWidth 2-8 in both `LSB`/`MSB` order, plus a 20,000-byte pseudo-random stress case that forces the dictionary all the way to its 4096-entry cap and a mid-stream clear-code reset -- not byte-for-byte compared against a real GIF/TIFF file, since real Go's own docs don't promise interop with either format's LZW variant precisely. Found and fixed a real, general algorithmic bug (in this stdlib source itself, not `wasigoc`): the decoder's dictionary fill permanently lags the encoder's by exactly one code position -- an inherent, correct property of the standard LZW decoder algorithm (it skips adding a dictionary entry for the very first code after start/clear, since there's no previous entry yet to combine with) -- so triggering the code-WIDTH bump on the same `nextCode == 1<<codeWidth` threshold the encoder uses widens the decoder's read width one code too late, corrupting every code from that point on. Fixed by bumping the decoder's width one threshold earlier (`nextCode == (1<<codeWidth)-1`) to compensate for that lag. Found by comparing an emit-by-emit encoder trace against a read-by-read decoder trace side by side after a small-litWidth round trip started failing partway through) |
| `container/heap` | **in** (`Interface` implementer must be a struct wrapping a slice -- see stdlib/container/heap) |
| `container/list` `container/ring` | **in** |
| `context` | **in** (`Context` is a concrete struct, not real Go's interface; `Background`/`TODO`/`WithCancel`/`WithValue`, cooperative `Done()`/`Err()`/`Value()`; `Value` keys are `string` not `any` -- see stdlib/context) |
| `crypto/rc4` | **in** (present for legacy interop only, like `crypto/sha1` -- broken by modern standards. `Cipher`/`NewCipher`/`XORKeyStream`/`Reset`; verified against 3 standard RC4 test vectors, cross-checked independently first) |
| `crypto/subtle` | **in** (same bitwise-trick algorithms real Go's own implementation uses -- no branches on secret data in the Go source -- with an honest caveat real Go doesn't need: no way to verify from here that an optimizing C++ compiler never reintroduces a branch. `ConstantTimeCompare`/`ConstantTimeByteEq`/`ConstantTimeEq`/`ConstantTimeSelect`/`ConstantTimeLessOrEq`/`ConstantTimeCopy`. Found and fixed a real, general compiler bug building this -- see the compiler-bugs writeup) |
| `crypto` | **in** (Hash identifier struct -- Size/String/Available for MD5/SHA-1/SHA-256/SHA-512/SHA3-256. No Hash.New(); construct a digest via the concrete package. Hash is a struct, not `type Hash uint`, because methods need a struct receiver) |
| `crypto/dsa` | **in** (bounded textbook SignRaw/VerifyRaw with a caller-supplied k, same "no GenerateKey" shape as crypto/rsa. Verified with a tiny p=23/q=11/g=2 vector) |
| `crypto/ecdh` | **in** (P-256 SharedSecret = x(priv*peer); Alice/Bob symmetry verified with small scalars) |
| `crypto/ecdsa` | **in** (P-256 SignRaw with caller-supplied k + Verify. Sign/Verify round trip with small d/k/hash) |
| `crypto/ed25519` | **in** (RFC 8032 Sign/Verify via math/big field arithmetic -- slow, correct. `ed25519pkg`'s golden only covers sizes and reject-short-sig, since a full scalar-mult round trip was believed too expensive for the decimal-limb Int at this size -- that belief hid a real bug: the `dEd` twisted-Edwards curve constant had a transcription error in its low digits (`...085989429717` instead of the correct `...085940283555`, i.e. `-121665/121666 mod p`), so the compiled-in base point never actually satisfied the curve equation. `ptAdd`/`scalarMult` were internally self-consistent (Sign and Verify each independently correct relative to the wrong curve), which is exactly why the discrepancy stayed invisible without an external Sign-then-Verify check. Found while building `unil` (below), which needed real signing; fixed by correcting the constant. A genuine full keygen/sign/verify/tamper-reject round trip is now exercised by `unilpkg`'s golden and does complete, just slowly (tens of seconds at `-O2` native; budget accordingly for anything that calls `Sign`/`Verify`/`PublicKey` more than a couple of times) |
| `unil` (not `go list std` -- project extension) | **in** (`stdlib/unil`: the "unil" bill-of-materials format, a Go++ port of `~/WASMUniLoader/cpp/src/sbom.cc`'s C++ core -- File/Component/Signature/Document, canonical JSON matching that C++ writer byte-for-byte including its partial-indent quirk, SHA-256 digest, Ed25519 sign/verify of documents and detached bytes, JSON parse via `encoding/json`'s generic `map[string]any` tree (its struct-Unmarshal path doesn't walk slice fields, so `Document` is assembled by hand from that tree -- same shape as sbom.cc's own tiny `J` parser), and an `Execute(cmd string)` JSON-in/JSON-out dispatcher mirroring sbom.cc's `execute()` (sandbox/bundle/canonical/digest/sign/verify/keygen/signbytes/verifybytes -- no `embeddedSandbox`, since this package has no baked-in WASMUniLoader wasm hashes to fall back to). Private keys are 64 bytes everywhere (32-byte seed \|\| 32-byte public, matching the C++ core and real Go's own `ed25519.PrivateKey` layout); only the seed half is passed to `ed25519.Sign`. `unilpkg`'s golden is a full round trip: build a sandbox+bundle document, digest it, generate a keypair, sign and verify the document, stringify then re-parse it and verify again, sign and verify detached bytes, and confirm tampered bytes are rejected) |
| `guac` (not `go list std` -- project extension) | **in**, on-disk shape only (`stdlib/guac`: the on-disk shape of a distributable Go++ wasm package, built on `unil` above -- a directory holding compiled wasm file(s) plus a `guac.json` manifest, which is an ordinary `unil.Document` with no schema changes (byte-compatible, every `unil.*` function applies unchanged), just two conventions: `Scope` is `"package"` (unil itself only ever writes `"sandbox"`/`"bundle"`), and `Name` is `"<import path>@<version>"` (`PackageName` splits it back), `Capabilities` holds exactly the build target triple (`Target`), and `Runtime` lists dependencies as `unil.Component{Role: "depends"}` (`Depend`) -- name, fetch origin, and the dependency's own manifest digest for pinning, the same name+hash-pinning shape `unil.Component` already had, just a new `Role`. `HashFile`/`BuildManifest` hash files already on disk (SHA-256 via `crypto/sha256`, CRC-32 via `hash/crc32.ChecksumIEEE` formatted big-endian-hex to match `crc32Hex` in WASMUniLoader/cpp/src/crc32.cc exactly, not byte-order hex-encoded); `WriteManifest`/`ReadManifest` are `os.WriteFile`/`os.ReadFile` plus `unil.Stringify`/`unil.ParseDocument`; `Verify` re-hashes a directory's files against a manifest and catches drift, independent of (and orthogonal to) checking the manifest's signature via `unil.VerifyDocument`. Deliberately does NOT create directories (Go++'s `os` builtin has no `Mkdir`), compile anything, or fetch a dependency -- that's a future `guac` CLI wrapping `wasigoc`/`goclang++` builds around this manifest layer. `guacpkg`'s golden: write a fake wasm file, build+write+read a manifest, verify it, tamper the file and confirm `Verify` catches it, then sign the same manifest with `unil.SignDocument`/`unil.VerifyDocument` unchanged to prove the two packages compose) |
| `crypto/elliptic` | **in** (NIST P-256 affine, IsOnCurve/Add/ScalarMult/ScalarBaseMult. G is on the curve; 1*G == G) |
| `crypto/rand` | **in** (Reader/Read fill from math/rand's time-seeded xorshift -- NOT a CSPRNG, same honest caveat as math/rand. Wiring WASI random_get would need a compiler builtin like time.Now) |
| `crypto/sha3` | **in** (SHA3-256 only, FIPS 202 Keccak sponge, domain 0x06. Verified against the empty-string and "abc" FIPS 202 vectors) |
| `crypto/hkdf` | **in** (HKDF-SHA256 Extract/Expand/Sum, RFC 5869 case 1 vector) |
| `crypto/pbkdf2` | **in** (PBKDF2-HMAC-SHA256, RFC 8018 / well-known password/salt/c=1 and c=2 vectors) |
| `crypto/tls` | **in** (`Dial`/`Read`/`Write`/`Close` are real via `gocvm.Call("tls.dial"/"tls.io.*", ...)` on a `goclang++.bat --shim-sandbox` build -- a genuine Schannel/SSPI handshake with automatic certificate + hostname validation, see the GocVM diary entry above; `Dial` does connect+handshake in one call, so `Handshake()` is a no-op once it succeeds. Plain wasm32-wasip1 falls back to the same "not supported" error as before. `LoadX509KeyPair` (client certs) stays stubbed -- out of scope) |
| `crypto/x509` | **in** (bounded DER header reader: ParseCertificate pulls the serial INTEGER out of TBSCertificate. No signature check, no chain Verify) |
| `crypto/x509/pkix` | **in** (Name + String = CommonName) |
| `crypto/rsa` | **in** (bounded: raw textbook modular exponentiation only -- `EncryptRaw`/`DecryptRaw` compute `m^E mod N`/`c^D mod N` via `math/big.Int.Exp` -- NOT real Go's actual API (`EncryptPKCS1v15`/`GenerateKey`/etc. don't exist here). No padding scheme (textbook RSA alone is deterministic/malleable, same "present for legacy/textbook interop only, not hardened" framing as `crypto/rc4`/`crypto/des`), and no `GenerateKey` (needs a modular inverse to derive D from E, which `math/big` doesn't have -- a `PrivateKey` must be built from an externally supplied N/E/D). **Verified against real Go itself** (go1.26.4, installed locally): both the classic textbook example (n=3233, e=17, d=2753, m=65 -> c=2790) and a second independently-chosen vector (n=589, e=7, d=463, m=123 -> c=61) were computed with real Go's own `math/big.Int.Exp` first, then reproduced exactly by this port. Found and fixed a real, general, previously-latent compiler bug while writing this package -- the first stdlib source in this project to initialize an embedded struct field by name in a composite literal (`PrivateKey{PublicKey: *pub, D: ...}`): `EmitCompositeLit`/`EmitCompositeLitPtr` (`src/cpp_generator.cc`) generated `__s.PublicKey = ...`/`__p->PublicKey = ...` for EVERY keyed/positional field including embedded ones, but an embedded field is emitted as a C++ base-class subobject, not a named member (`struct PrivateKey : public PublicKey {...}` -- see `EmitStructDefs`), so `PublicKey` there resolved to the base class's own injected-class-name instead of anything assignable, failing to compile. Fixed by special-casing an embedded field in all four composite-literal code paths (keyed/positional × value/pointer) to assign through the base subobject directly (`static_cast<Base&>(__s) = ...`), the same shape the struct's own generated `operator==` already used for embedded-field comparison; also had to resolve the embedded field's OWN package correctly (an unqualified embedded-field type name means a type in the STRUCT's declaring package, not the package of the file currently being generated -- `QualName`'s pkg-empty case defaults to the latter) |
| `crypto/aes` | **in** (AES-128 only -- no 192/256-bit keys. FIPS 197 textbook state-array formulation (SubBytes/ShiftRows/MixColumns/AddRoundKey via a general `gmul` GF(2^8) multiply, not a bitsliced/constant-time implementation -- same "no hardware AES-NI, no timing-side-channel hardening" caveat real Go's own pure-Go fallback path already carries). Implements `crypto/cipher`'s `Block`. Verified against the standard FIPS 197 Appendix B test vector (key `000102030405060708090a0b0c0d0e0f`, plaintext `00112233445566778899aabbccddeeff` -> `69c4e0d86a7b0430d8cdb78070b4c55a`), plus a decrypt round trip) |
| `crypto/cipher` | **in** (just the `Block` interface -- the shape `crypto/des`'s `Cipher` implements. No `Stream`/`AEAD`/`BlockMode` chaining-mode wrappers, same bounded-scope precedent as `hash.Hash` not including a generic streaming wrapper) |
| `crypto/des` | **in** (single DES, FIPS 46-3 -- present for legacy interop only, same "broken by modern standards" framing as this project's `crypto/rc4`/`crypto/sha1`; no Triple DES. Textbook bit-permutation formulation (IP/FP/E/P/PC1/PC2 tables + 8 S-boxes) operating on a `uint64` block value, not the bitslice-friendly classic C implementation. Verified against 2 standard FIPS 46-3 test vectors (the `0123456789ABCDEF`/key `133457799BBCDFF1` -> `85E813540F0AB405` vector, and the well-known all-zero-key/all-zero-plaintext -> `8CA64DE9C1B123A7` vector), plus a decrypt round trip. Found a real, general parser gap building it: a composite literal element that elides its own type (`[][]int{{1, 2}, {3, 4}}`'s inner `{1, 2}`, or a map value like `map[string][]int{"a": {1, 2}}`) failed to parse at all -- `ParsePrimary` had no case for a bare `{`. Fixed in `ParseCompositeLitBody` (`src/parser.cc`) by reusing the outer literal's element type for a bare-brace element/value (not for a bare-brace *key*, which would need the map's key type -- deliberately left unhandled, rare enough not to be worth the added ambiguity). The S-boxes themselves stayed a flattened single `[]int` (already verified working) rather than being rewritten to the nested form after the fact) |
| `crypto/md5` | **in** (one-shot `Sum` + streaming `Digest`; verified against the 3 standard RFC 1321 test vectors) |
| `crypto/sha1` | **in** (one-shot + streaming, same shape as `md5`; verified against 3 standard test vectors -- present for legacy interop only, SHA-1 is collision-broken) |
| `crypto/sha256` | **in** (one-shot + streaming; verified against 3 standard FIPS 180-4 test vectors including the well-known empty-string hash) |
| `crypto/sha512` | **in** (one-shot + streaming, same shape as `sha256` but 64-bit words/128-byte blocks/80 rounds; constants fetched from an authoritative source and verified against 3 standard FIPS 180-4 test vectors, not just transcribed by eye. Length suffix only ever fills the low 64 bits of the real 128-bit field -- bit-identical to real SHA-512 for any input that could actually occur here) |
| `crypto/hmac` | **in** (partial, deliberately bounded -- no generic `New(func() hash.Hash, key)` since this project has no shared `hash.Hash` interface; instead three concrete functions, `SumMD5`/`SumSHA1`/`SumSHA256`, all sharing the same 64-byte block size, plus `Equal` (NOT constant-time -- no `crypto/subtle` here). Verified against 5 real RFC 4231/RFC 2202 test vectors, including the long-key-gets-hashed-down case (key > block size)) |
| `database/sql` `database/sql/driver` | **in** (real, working generic dispatch layer -- NOT a stub -- against ANY conforming `driver.Driver`: `Register`/`Open`, `DB`/`Rows`/`Row`/`Stmt`/`Tx`/`Result`, argument conversion (nil/int64/float64/bool/[]byte/string plus Go's other integer/float widths widened), and real `Scan`-by-pointer-assertion. `driver` package covers the CORE SPI (`Driver`/`Conn`/`Stmt`/`Rows`/`Tx`/`Result`, plus the optional `Execer`/`Queryer` fast-path interfaces) -- no context-aware method variants (`DriverContext`/`Connector`/`*Context` interfaces), matching this project's own `context` package being a concrete cooperative struct with no real cancellation to make those variants meaningful. What's genuinely impossible on wasm32-wasip1 is a CONCRETE driver that dials a real socket to an external database server and holds it open under concurrent use (no socket syscalls at all, see `net`'s own tracker line) -- that gap lives entirely in a driver implementation, never in this generic layer, and this package would work unchanged with a real socket-backed driver once Wasi2G++ bridges real networking through its sandbox. `DB` does not pool connections (one lazily-opened connection, matching this project's cooperative single-goroutine-realistic execution model, not real Go's concurrent pool). No `time.Time` driver.Value support, no `sql.Null*` wrapper types (a NULL scanned into anything but `*any` errors, same as real Go for a non-nullable destination). Verified end-to-end (not against a real database -- against a small hand-written in-memory `driver.Driver`, the same technique real Go's own `database/sql` test suite uses internally via `fakedb_test.go` to test this exact generic layer without a real database): Register/Open/Exec/Query/Rows.Next/Scan/QueryRow/Prepare/Stmt.Exec/Stmt.Query/Begin/Commit/Rollback(`ErrTxDone`)/`ErrNoRows`/unknown-driver-error, exercised through BOTH dispatch paths (a `Conn` implementing `Execer`/`Queryer` directly, and the Prepare+Exec/Query+Close fallback for a `Conn` that doesn't) -- 27 assertions, all passing. **Three compiler bugs fixed** (`src/cpp_generator.cc`), all the same root shape (code that checks "is this the `any`/interface/nil-spelling case" without resolving through a type-alias chain first) or a new same-package-interface-cross-reference shape, both first exercised by this package: (1) `IsInterfaceType` didn't resolve a defined alias of `any` (`type Value any`, this package's own `driver.Value`) through `ResolveUnderlying`, so a type assertion on a `Value`-typed expression (`args[0].(int64)`) wrongly failed with "type assertion requires an interface value". (2) `EmitAdapt` (interface-to-interface conversion) only compared two interface types' bare NAMES, erroring "cannot convert interface 'Value' to 'any'" even though both resolve to the identical `wasigo::Any` C++ type via a transparent `using` alias -- fixed by resolving both sides through `ResolveUnderlying` and comparing the resolved root instead. (3) The same alias-blind-spot in Go-`nil`'s C++ "empty value" spelling (`EmitExprAs`'s Nil case) wrongly emitted `nullptr` for a `Value`-typed nil (needs `{}` for `wasigo::Any`) -- factored into a new recursive `NilSpellingFor` helper that resolves through `ResolveUnderlying` the same way. (4) A genuinely new shape, not an alias issue: an interface method with 2+ results (needing a generated nested `_result` struct) where one result is ANOTHER interface declared LATER in the same package (`driver.Driver.Open(name string) (Conn, error)`) failed two ways in sequence -- first "Conn does not name a type" (no forward declaration existed at all for interfaces referencing each other, only structs had this), then, after adding forward declarations, "field has incomplete type" (a value-typed struct field needs the type COMPLETE, not just declared, and interfaces were still emitted in plain file order). Fixed with a DFS topological sort over "which other same-package interfaces does this one's method set return", emitting full interface bodies in dependency order after forward-declaring all of them up front (a genuine cycle is a hard compile error, not a hang). Also renamed real Go's own `driver.RowsAffected` (a struct with a method ALSO named `RowsAffected()`) to `RowsAffectedResult` -- unrepresentable in C++ (a member function can never share its class's exact name), same "rename the Go type" precedent as `image/color`'s `Rgba`/`hash/fnv`'s `Digest32`. Full ctest suite re-run after all four fixes, zero regressions) |
| `debug/buildinfo` | **in** (bounded: `Read([]byte)` extracts the Go linker's embedded build-info blob directly from a binary's raw file bytes -- searches for the 16-byte-aligned magic directly in the file, skipping real Go's own per-format (ELF/PE/Mach-O/...) virtual-address section translation entirely (a real, deliberate simplification: true whenever a section's on-disk file offset and virtual address are congruent modulo 16, which holds for every mainstream linker's section alignment). Bounded to the modern (Go 1.18+) inline blob format only, not the pre-1.18 pointer-based one. `ModInfo` is the raw sentinel-stripped module-info block as one string, not further split into `Main`/`Deps`/`Settings`. **Verified against a REAL Go-built binary**, not a hand-made fixture: a tiny program was compiled with the actual installed Go 1.26.4 toolchain, and the exact raw buildinfo blob bytes extracted from that real `.exe` were fed to this port -- GoVersion and ModInfo matched real Go's own `debug/buildinfo.ReadFile`/`go version -m` output for the same binary exactly. Found and fixed a real, general, previously-latent compiler bug while writing this package: `wasigoc`'s own lexer (`src/lexer.cc`) had never implemented the `\xHH` hex-escape in string/rune literals at all (only `\n\t\r\\'"0`) -- needed here for the magic string `"\xff Go buildinf:"`, matching real Go's own linker-emitted byte sequence exactly. Fixed by adding a `\x` case to `ScanEscape` that consumes exactly 2 hex digits and returns one byte value (scoped deliberately to `\x` only -- `\u`/`\U`/full octal escapes decode to a Unicode code point that can span multiple UTF-8 bytes, which doesn't fit `ScanEscape`'s existing "return one byte" shape without a larger refactor, so those stay a separate, unfixed, documented gap) |
| `debug/dwarf` | **in** (bounded 32-bit compilation-unit header reader: unit_length/version/abbrev_offset/address_size. No DIE tree, no 64-bit DWARF) |
| `debug/gosym` | **in** (bounded pclntab magic/quantum/ptrsize header reader. Recognizes Go 1.2–1.20 magics. No PC-to-line) |
| `debug/plan9obj` | **in** (bounded, read-only, same "header reader" scope precedent as debug/elf/pe/macho: the 32-byte fixed a.out header (magic/text/data/bss/syms/entry/spsz/pcsz, all big-endian) plus the 8-byte expanded 64-bit entry field real Go's own format adds when `Magic&Magic64 != 0`, and the five fixed section slots (text/data/syms/spsz/pcsz) at their real Go offsets. `NewFile([]byte)`, same bound as its sibling debug/* packages. No symbol-table decoding (`Symbols`/`walksymtab` in real Go). **Verified against real Go itself** (go1.26.4, installed locally at `C:\Program Files\Go` -- see the compiler-bugs writeup's closing note), not Python or hand derivation: a real Go program built a MagicAMD64 (64-bit) fixture and parsed it with the actual `debug/plan9obj` package, and every FileHeader/Section field this port computes from the same bytes matched exactly -- the first debug/* package in this project verified this way rather than against a hand-built spec reading or a real system binary) |
| `debug/macho` | **in** (bounded, read-only, 64-bit only -- no 32-bit, no fat/universal binaries: `mach_header_64` plus `LC_SEGMENT_64` load commands (segment name + its sections' name/address/size/file offset). No symbol table, dyld info, or code signature. `NewFile([]byte)`, same bound as debug/pe/debug/elf. Same honest gap as debug/elf: NOT verified against a real Mach-O file (this project's host/target are both non-Mach-O), verified instead by careful hand construction against the stable Mach-O 64-bit layout (fixed 32-byte file header, 72-byte segment header, 80-byte section entries)) |
| `debug/elf` | **in** (bounded, read-only, 64-bit little-endian only -- no 32-bit, no big-endian: ELF identification + file header (`Type`/`Machine`/`Entry`) and the section header table with names resolved against `e_shstrndx`'s string table. No program headers, symbol table, relocations, or DWARF info. `NewFile([]byte)`, same bound as debug/pe. UNLIKE debug/pe (verified against a real system binary), this package was NOT tested against a real ELF file -- this project's host (Windows/MSVC) and target (wasm32-wasip1) are both non-ELF, so none was available; verified instead by careful hand construction against the well-documented, stable ELF64 spec (fixed 64-byte file/section header sizes) -- an honest, stated gap rather than a glossed-over one, same spirit as compress/lzw's "not verified against a real GIF" caveat. A first attempt at the hand-built test fixture actually caught a real bug -- in the FIXTURE, not the parser: the section-name strings were written without their leading `.` (`text`/`shstrtab` instead of `.text`/`.shstrtab`), which the string-table offsets had already been computed assuming were correct -- caught immediately as two failing name checks, fixed in the fixture, not the package) |
| `debug/pe` | **in** (bounded, read-only: DOS header (just enough to find `e_lfanew`), COFF file header, the `OptionalHeader` fields that matter (`Magic`/`AddressOfEntryPoint`/`ImageBase`, correctly sized for both PE32 and PE32+), and the section table. No symbol table, imports/exports, relocations, or resources -- same "header reader" scope precedent as `archive/tar`. `NewFile([]byte)` rather than real Go's `io.ReaderAt`, same bound as `archive/zip.NewReader`. Verified against a REAL system binary during development, not a synthetic fixture: parsed `wasigoc.exe` itself and independently cross-checked every field against `objdump -f`/`objdump -h`'s own separate parse of the same file -- machine type, section count, PE32+ magic, image base, absolute entry point address, and both size/address/file-offset for two sections, converting objdump's hex to decimal by hand before comparing, all matched exactly. The checked-in golden test instead uses a small hand-built PE32+ fixture (the wasm sandbox can't read a host `.exe`), same "own tiny fixture" shape as crypto/aes's FIPS test vector) |
| `embed` | **in** (stub FS -- wasigoc does not generate `//go:embed` data; Open/ReadFile return a clear "no files" error so imports compile. Implements io/fs.FS) |
| `encoding/base64` | **in** (`StdEncoding`/`URLEncoding`, standard padding only) |
| `encoding/hex` | **in** |
| `encoding/binary` | **in** (`LittleEndian`/`BigEndian` Uint16/32/64 get/put/append) |
| `encoding/json` | **in** (`Marshal`/`Unmarshal` for the *generic* decoded-JSON value shape -- `any`/`map[string]any`/`[]any`/`string`/`float64`/`bool` -- PLUS `Marshal`/`Unmarshal` for arbitrary structs via `reflect`, including nested struct fields and fields typed as a slice or a named slice type of scalars (`reflect.Value.SetSlice`); a slice-of-structs or nested-slice-typed field still isn't a settable target) |
| `encoding/csv` | **in** (RFC 4180: quoted fields, embedded commas/newlines, doubled-quote escaping, `Comment`/blank-line skipping, `FieldsPerRecord` enforcement, `TrimLeadingSpace`; `Comma`/`Comment` are ASCII bytes, not runes -- multi-byte delimiters unsupported. `Reader` slurps its whole input via `io.ReadAll` rather than streaming) |
| `encoding/base32` | **in** (RFC 4648: standard and hex alphabets, standard padding only; verified against the RFC's own section-10 test vectors for both alphabets, plus a longer round trip) |
| `encoding/pem` | **in** (RFC 1421-shaped blocks: `Decode`/`Encode`/`EncodeToMemory`. Bounded: header values are single-line only, no RFC 1421 continuation-line folding. Found and fixed a real, general compiler bug building this -- see the compiler-bugs writeup) |
| `encoding` | **in** (matches real Go exactly -- it's tiny there too: just `BinaryMarshaler`/`BinaryUnmarshaler`/`TextMarshaler`/`TextUnmarshaler`, no functions. Verified with a `*Point` type implementing all four, assigned to each interface, called through the interface, and round-tripped both text and binary encodings) |
| `encoding/asn1` | **in** (DER Marshal/Unmarshal of int64/bool/[]byte plus write-only struct SEQUENCE via reflect. No arbitrary-struct Unmarshal, same settable-reflect wall as encoding/json) |
| `encoding/gob` | **in** (tiny tagged binary for string/int64/[]byte only, not real Go's type-graph gob. No struct decode) |
| `encoding/xml` | **in** (Marshal only, same "read-only reflect blocks Unmarshal" bound as `encoding/json`'s own Marshal-only-via-reflect half -- no `xml:"..."` struct tags, no attributes, no slice-of-struct repetition. The root element's tag is the struct's own Go type name (`reflect.Value.Type().Name()`), matching real Go's own default; a nested struct field becomes a nested element recursively; text content is escaped for all five XML special characters. Verified against a real independent XML parser, not just self-consistency: the exact expected byte-for-byte output (including a name field containing literal `&`/`<`/`>` to exercise escaping) was ALSO fed to Python's `xml.etree.ElementTree.fromstring`, confirming it's well-formed XML a real parser accepts, with the escaped characters round-tripping back to the original `&`/`<`/`>` text) |
| `encoding/ascii85` | **in** (`Encode`/`Decode`/`MaxEncodedLen`/`EncodedLen` -- real Go's own low-level API is already `[]byte`-in-`[]byte`-out, not streaming, so this is a faithful match; no `NewEncoder`/`NewDecoder` io.Writer/io.Reader wrappers. `[N]byte` fixed arrays avoided in favor of a `make([]byte, 5)` scratch slice, same precedent as crypto/rc4/hash/crc32. Verified against Python's `base64.a85encode`/`a85decode`, including the `z`-for-all-zero-group shortcut and a full decode round trip) |
| `errors` | **in** (builtin, partial) |
| `expvar` | **in** (published, named, in-memory variables -- the registry half only, no HTTP endpoint since there's no `net/http` here. `Var` is a real single-method interface; `Int`/`Float`/`StringVar`/`Map` all implement it (`StringVar`, not `String` -- avoids the struct/method same-name C++ constructor collision, same trap as `hash/fnv`'s `Digest32`). `Publish`/`Get`/`Do`, registration order preserved) |
| `flag` | **in** (`String`/`Int`/`Bool` + `Var` forms, `Parse`, `Args`/`NArg`/`Arg`/`Parsed`; no `FlagSet` type, no `Usage`/`PrintDefaults`, no `Float64`/`Duration` flags) |
| `fmt` | **in** (builtin, partial) |
| `go/token` | **in** (`Token`, keyword `Lookup`, `Pos`; `TokenString`/`IsLiteral`/`IsOperator`/`IsKeyword`/`PosIsValid` are free functions, not methods -- `Token`/`Pos` are `type X int`, not structs, and methods here need a struct receiver) |
| `go/scanner` | **in** (a real tokenizer: identifiers/keywords, decimal/hex/octal/binary ints, floats, interpreted/raw strings, rune literals, `//`/`/* */` comments, and correct automatic semicolon insertion -- verified against 5 snippets including ASI across a newline-spanning block comment) |
| `go/ast` | **in** (partial -- one tagged `Node` struct with a `Kind` enum instead of real go/ast's Node/Expr/Stmt/Decl interface hierarchy, deliberately: this compiler's own internal AST uses exactly this shape for the same reason, it's much less risky to generate than a wide interface hierarchy. Expressions, statements, pointer/slice/map types, composite literals, `var`/`const` specs -- see `go/parser`'s comment for exactly what) |
| `go/parser` | **in** (partial -- real recursive-descent parsing with correct binary-operator precedence, into `go/ast`'s Node. Expressions: idents, literals, binary/unary (incl. `&x`/`*x`), calls, selectors, index, parens, composite literals (`T{...}`/`[]T{...}`/`map[K]V{...}`, positional elements only). Types: identifier/`pkg.Type`, pointer, slice, map. Statements: expr/assign/define/incdec, if, for (3-clause/cond-only/bare/range), switch (expr cases, no type switch), return, break/continue, blocks, single-spec `var`/`const`. Decls: `func` with `name type` params/one result, top-level `var`/`const`. NOT supported: type declarations, struct/interface literals, select/go/defer, type switch, multi-return, generics, labeled statements/goto, grouped `var (...)`/multi-name specs. `ParseExpr`/`ParseFile` return `(nil, error)` on a syntax error via sticky parser state, not panic/recover -- see the compiler-bugs writeup for why panic/recover doesn't work across function calls here) |
| `go/printer` | **in** (partial -- `Sprint(n) string` re-emits a `go/ast` Node tree as Go source text, tab-indented blocks; covers exactly the Node shapes `go/parser` produces, including everything in this round's grammar extension) |
| `go/format` | **in** (partial -- `Source(src) (string, error)`, a parse-then-print round trip, the same idea as gofmt bounded by `go/parser`'s/`go/printer`'s own scope) |
| `go/types` | **in** (partial, deliberately bounded -- not a whole-program type checker: `CheckExpr` type-checks one expression (idents/literals/binary/unary/paren/index/composite literals for slices, no calls), `CheckStmt` type-checks a whole statement tree (`var`/`const`, assign/define, if/for including conditions, range-for, switch including bare `switch{}`, blocks; NOT calls, NOT keyed map composite literals) against a `map[string]*Type` environment. Map types (`MapOf`) and slice/map indexing are also interned and checked. Every distinct type shape is *interned* to one canonical `*Type` keyed by its shape string ("int", "*int", "[]string", "map[string]int"), so `Identical(a, b)` is pointer equality, not a structural walk -- the same "Object Type Identifier" shape Blink's `WrapperTypeInfo` uses for DOM wrapper objects, and the same idea this compiler's own `type_key_of<T>()` already uses for `any`'s runtime identity (see runtime.hpp). No package-level checking, no method sets, no generics, no `go/types.Info`-style result maps) |
| `go/version` | **in** (`IsValid`/`Lang`/`Compare` for "goN[.N[.N]]"-shaped strings, numeric part-by-part comparison so "go1.9" < "go1.10"; no "devel" builds, no pre-release suffixes) |
| `go/build/constraint` | **in** (`Parse(line)` for both `//go:build ...` and old-style `// +build ...` lines, `Eval`/`String`; one tagged `Expr` struct -- `TagExpr`/`NotExpr`/`AndExpr`/`OrExpr` -- not real Go's `Expr` interface, same simplification `go/ast` already made. Not implemented: `PlusBuildLines`, converting an arbitrary `Expr` back to old-style lines) |
| `go/build` | **in** (Default Context GOOS=wasip1/GOARCH=wasm, IsLocalImport, ParsePackageName via go/parser. Import/ImportDir return "no directory listing") |
| `go/importer` | **in** (stub -- Default().Import returns "no package discovery") |
| `go/doc` | **in** (bounded to exactly the declarations this project's own `go/parser` produces -- `func` and single-spec top-level `var`/`const`, no `Type` docs at all, since `go/parser` doesn't parse type declarations yet. Unblocked by making `go/scanner` additively capture every comment's raw text+position into a new `Comments []Comment` side-channel field -- Scan's own return values/behavior are completely unchanged, so every existing `go/scanner`/`go/parser`/`go/printer`/`go/format`/`go/types` golden was re-run and still passes unmodified. `go/parser` gained one new additive entry point, `ParseFileWithComments`, alongside the untouched existing `ParseFile`. Association: for a declaration at position P, walk captured comments backward, including each one only while the raw source between its end and the current anchor is pure whitespace with at most one newline (i.e. immediately precedes, no blank line, nothing else in between) -- the same shape real Go's own `ast.CommentGroup`-to-declaration association uses. Matches real Go's own DEFAULT (non-`AllDecls`) mode: only exported (capitalized) names are included. Verified against real Go itself (go1.26.4, installed locally): the identical bounded source -- a package comment, a doc-commented exported func, a doc-commented exported var, an exported const with no doc, and a func deliberately separated from its preceding comment by a blank line -- fed to real Go's own `go/doc.NewFromFiles`, and every Doc field this port computed matched exactly, including the blank-line case correctly producing an empty Doc rather than swallowing an unrelated preceding comment, and the unexported-name filtering behaving identically) |
| `go/doc/comment` | **in** (bounded: parses an already-extracted doc-comment string, not tied to `go/scanner`'s comment discarding -- one tagged `Block` struct (`Kind` enum: Paragraph/Heading/Code) instead of real Go's Block-interface hierarchy, same simplification precedent as `go/ast`'s Node. Recognizes blank-line-separated paragraphs, `# Heading` lines (the Go 1.19+ convention), indented lines as a Code block. No inline spans (bold/italic/links within a paragraph), no `List` blocks, no `Printer` output formats. Verified with an 11-check doc comment mixing all three block kinds) |
| `go/constant` | **in** (bounded: one concrete `Value` struct (`Kind` + whichever field applies) instead of real Go's arbitrary-precision `big.Int`/`big.Rat`/`big.Float`-backed sealed interface -- `int64`/`float64` here, same bounded-precision precedent as this project's own `go/types`. No `Complex` kind (no complex number type in this compiler at all), no `MakeFromLiteral`. `MakeBool`/`MakeString`/`MakeInt64`/`MakeFloat64`/`MakeUnknown`, `BoolVal`/`StringVal`/`Int64Val`/`Float64Val`, `BinaryOp`/`UnaryOp`/`Compare` against real `go/token.Token` operators, `Sign`. `Kind.String()` doesn't exist (methods need a struct receiver here) -- `KindString` free function instead, same shape as `go/token`'s `TokenString`. Verified with 14 checks: int/float arithmetic, mixed int+float promotion, string concatenation, bool `&&`/`||`, unary negation, comparison, `Sign`, and division-by-zero correctly producing `Unknown` rather than crashing) |
| `hash` | **in** (`Hash`/`Hash32`/`Hash64`, `Hash32`/`Hash64` embedding `Hash` -- this project's first interface-embeds-interface package. Found and fixed a real compiler bug building it: embedded-interface method lookup (`CollectIfaceMethods`) resolved the embedded name against the *current* file being generated instead of the embedding interface's own package, so `hash.Hash32` embedding `hash.Hash` from outside package `hash` came back "unknown method" -- same "asking an unscoped / wrong-package question" bug family as the `encoding/pem`/`encoding/binary` fixes below. Verified `crypto/md5.Digest` satisfies `Hash`, `hash/adler32.Digest`/`hash/crc32.Digest` satisfy `Hash32`, `hash/crc64.Digest` satisfies `Hash64`, all called through the interface not the concrete type -- checksums cross-checked against Python's `zlib.adler32`/`zlib.crc32`, and the `crc64.Sum64()` result compared bit-for-bit against calling the same Digest directly without going through the interface) |
| `hash/adler32` | **in** (`Checksum` + streaming `Digest`; verified against the well-known "Wikipedia" test vector) |
| `hash/crc32` | **in** (IEEE polynomial only, no Castagnoli/Koopman; `ChecksumIEEE` + streaming `Digest`; verified against two standard test strings) |
| `hash/crc64` | **in** (ISO and ECMA polynomials, same shape as `hash/crc32`; `Checksum` + streaming `Digest`; verified against the standard CRC-64/GO-ISO and CRC-64/XZ check values for `"123456789"`, cross-checked independently) |
| `hash/maphash` | **in** (real, working, deliberately NOT bit-for-bit matching Go's own algorithm -- real Go's own contract for this package explicitly doesn't promise that either, same "Rosetta not parity" precedent as `math/rand`. Implemented as seeded FNV-1a-64: `Seed`/`MakeSeed`/`Hash` (lazy self-seeding zero value, `Write`/`WriteString`/`WriteByte`/`Sum64`/`Sum`/`Reset`/`SetSeed`/`Seed`) plus one-shot `Bytes`/`String`) |
| `hash/fnv` | **in** (FNV-1 and FNV-1a, 32- and 64-bit, no 128-bit; verified the 32-bit FNV-1a value against a known test vector) |
| `html` | **in** (`EscapeString`/`UnescapeString` only, no template parsing. `UnescapeString` handles the entities `EscapeString` can produce plus `apos`, and decimal/hex numeric character references -- NOT real Go's full ~2000-entry named-entity table (`nbsp`, `copy`, `times`, ...), a real documented gap) |
| `html/template` | **in** (bounded: the same `{{.Field}}`/`{{if}}` engine as this project's own `text/template` -- a separate, independent implementation, not a wrapper, since there's no way to share unexported package internals here -- but every substituted value is passed through `html.EscapeString` first. This is NOT real Go's actual html/template: real Go does CONTEXTUAL escaping (JS-string escaping inside `<script>`, URL escaping inside `href`, etc.); this package always applies the same HTML-entity escaping regardless of where the value lands -- safe-by-default for ordinary HTML text (never lets `<`/`>`/`&`/quotes through unescaped, so no script-tag injection), but NOT a substitute for real contextual escaping inside `<script>`/`<style>`/a URL attribute, a stated real gap. Verified against a real XSS-shaped payload, not just plain text: interpolating `<script>alert('x')</script> & stuff` produced fully entity-escaped output with `<script` occurring nowhere in the result, confirmed by direct substring check, and by round-tripping the escaped text back through Python's `html.unescape` to recover the exact original string) |
| `image/color/palette` | **in** (partial: `WebSafe` only, built via the same three-nested-loop construction that DEFINES the web-safe palette (every R/G/B combination from the 6-step ramp `{0,51,102,153,204,255}`) rather than transcribed, so it's provably the standard 216-color table by construction. `Plan9` NOT implemented -- it's a fixed 256-entry quantization table with no simple formula, and hand-transcribing 256 RGB triples is exactly the error-prone busywork this project avoids elsewhere. Verified: length 216, first entry black, last entry white, and one interior index hand-derived from the loop order and checked to match) |
| `image/draw` | **in** (bounded: `Draw` only, no `Drawer`/`FloydSteinberg`/`Quantizer`. `dst` is a concrete `*image.RGBA`, not real Go's `draw.Image` interface -- this project's `image` package has exactly one concrete pixel-buffer type anyway; `src` stays the generic `image.Image` interface. `Over` does real per-pixel alpha compositing on already-premultiplied values, consistent with this project's `color.Rgba.RGBA()` (like real Go's own `color.RGBA`) never multiplying by alpha itself. Verified against independently hand-derived arithmetic, not just self-consistency: `Src` op does a direct opaque copy (checked) and leaves an untouched pixel alone (checked); `Over` blending a premultiplied `{100,25,0,128}` source onto an opaque `{100,100,100,255}` destination was computed by hand from the 16-bit widened values (`sr=25700`, `inv=32639`, etc.) BEFORE running the program, then checked to match the actual output (150/75/49/255) exactly) |
| `image/gif` | **in** (`Decode` only, first frame of a GIF87a/GIF89a stream -- no `DecodeAll`/animation, no `Encode` (needs truecolor-to-palette color quantization, a substantial separate algorithm this project doesn't have). Returns a concrete `*image.RGBA` (this project's `image` has no `Paletted` type), resolving each palette index immediately. Supports global AND local color tables, the Graphic Control Extension's transparent-color index, and 4-pass row interlacing (GIF's interlacing is simple row reordering, unlike PNG's 7-pass Adam7 which this project's own `image/png` already documents as unsupported). This session corrected an earlier, unverified assumption of this project's own (that GIF needs LZW's "early change" quirk): real Go's own `compress/lzw` doc comment says explicitly it "implements LZW as used by the GIF and PDF file formats" (TIFF is the incompatible variant, not GIF), and real Go's own `image/gif` calls the stock `lzw.NewReader` with no GIF-specific wrapper at all -- confirmed directly against real Go's source, so this project's own `compress/lzw` needed no changes at all. Verified against real Go as the oracle: a Pillow-generated indexed GIF and a hand-built interlaced GIF (this project's own frame framing, compressed through real Go's own `compress/lzw.Writer`) were both first decoded with real Go's own `image/gif.Decode` to get the expected pixel values checked in the golden test, and a transparent-pixel case (Pillow's `transparency=` save option) checks the Graphic Control Extension path. **Compiler bug fixed** (`src/cpp_generator.cc`): Go's `:=` reuses (assigns to, doesn't redeclare) any name already declared in the same block scope, as long as at least one name on the left is new -- `size, err := readByte(...)` followed later by `data, err := readN(...)` in the same function is completely ordinary Go, but every multi-name `:=` emission site (`UnpackCallResults`, the parallel-RHS branch of `EmitShortVarDecl`, `EmitCommaOkDecl`, `EmitRecvOkDecl`, `EmitTypeAssertOkDecl`) unconditionally emitted a fresh C++ declaration for every name, so reusing `err` a second time in one function was a hard "redeclaration" compile error -- true of any stdlib source doing ordinary sequential error-checking with `:=`, just not previously triggered. Fixed by adding a `DeclaredInCurrentScope` check (consults the same scope-stack `Declare`/`Lookup` already uses) at all five sites: emit a plain assignment instead of a declaration when the name already exists in the current scope. Full ctest suite re-run (229 tests) confirmed zero regressions) |
| `image/jpeg` | **in** (`Decode` only -- no `Encode`/`DecodeConfig`/`DecodeAll`. Baseline (SOF0) sequential DCT only: no progressive/SOF2, no extended-sequential/SOF1, no arithmetic coding. 8-bit precision only. Grayscale (1 component) and YCbCr (3 component) only -- no CMYK/YCbCrK (needs Adobe APP14 interpretation this package skips). Any (h,v) subsampling ratio real Go itself accepts (4:4:4/4:4:0/4:2:2/4:2:0/4:1:1/4:1:0) is supported, upsampled by nearest-neighbor exactly like real Go's own `convertToRGB`. Restart markers (DRI/RSTn) are implemented per spec, but a mismatched restart marker is a hard error rather than real Go's `findRST` corrupt-stream resync -- an honest narrower bound (also: this specific path wasn't independently exercised by a fixture with restart markers this session, since real Go's own encoder has no restart-interval option to construct one with). Requires a single fully-interleaved scan naming every frame component (the near-universal shape for any ordinary baseline encoder) -- a scan naming fewer components is rejected as unsupported. Returns a concrete `*image.RGBA`. The Huffman decoder ports only real Go's general bit-by-bit algorithm, not its look-up-table fast path (a pure speed optimization real Go itself falls back off of for any code >8 bits or the tail of a scan, so the slow path alone is already complete and correct). The IDCT, by contrast, is ported VERBATIM from real Go's own `image/jpeg/dct.go` (the fixed-point Loeffler/Lightenberg/Mostchytz algorithm, `dctBox`/`c()`/`idctRows`/`idctCols`) specifically so decoded pixels are BIT-IDENTICAL to real Go's own decoder, not merely close (different JPEG decoders' IDCTs normally differ by a few least-significant bits -- confirmed by cross-checking against Pillow's decode of the same file, which was NOT bit-identical, unlike real Go's). Verified against real Go's own `image/jpeg.Decode` as the oracle across 4 real JPEGs (Pillow-encoded): grayscale, 4:4:4 and 4:2:0 chroma subsampling on a smooth gradient, and a 32x32 noisy/high-frequency 4:2:0 image (exercising more AC coefficients, ZRL runs, and multiple blocks per component) -- all four decoded byte-for-byte identical to real Go's pixels, zero mismatches. **Two compiler bugs fixed** (`src/cpp_generator.cc`): (1) indexing through a pointer to an array type (`p[i]` where `p *[N]T`, legal Go shorthand for `(*p)[i]`) was a hard "cannot index this type" error in both `InferType`'s Index case and `EmitIndex` -- neither checked for `TypeKind::Pointer` wrapping an array; needed for `*huffman`/`*block` params throughout this package. Fixed by resolving the pointee's underlying type in both places and, in codegen, emitting an explicit `(*(base))[idx]` deref (a bare `(base)[idx]` on a real C++ pointer would be pointer arithmetic one level too high, not element access). (2) Not a bug but a documented constraint surfaced here for the first time: `runtime.hpp`'s `slice_array` COPIES an array's windowed bytes into a new backing slice rather than aliasing the array's own storage ("arrays live on the stack... WASM has no growable stacks to alias"), so `arr[:]`-into-a-struct-field patterns (`d.readFull(tmp[:n])` into a `[128]byte` field, expecting the read to populate the field) silently write into a discarded copy. Worked around stdlib-side by making `decoder.tmp` and `huffman.vals` real `[]byte` slice fields (allocated via `make`) instead of fixed arrays -- slice re-slicing already aliases correctly project-wide, only array-to-slice conversion doesn't. Full ctest suite re-run after the compiler fix, zero regressions) |
| `image/png` | **in** (bounded: `Encode(w, *image.RGBA)` -- a concrete type, not real Go's `image.Image` interface, since this project's `image` package only has one concrete pixel-buffer type anyway -- always writes color type 6 (truecolor+alpha, 8-bit), filter type 0 (None), no interlacing. `Decode` is deliberately more general than `Encode` (same split as compress/flate's simple-encoder/general-decoder): color types 0/2/6 at 8-bit depth, and ALL FIVE PNG filter types (None/Sub/Up/Average/Paeth, RFC 2083 section 6) so it reads real-world PNGs written by any real encoder, not just this package's own output. Every chunk's CRC-32 is verified, not skipped. NOT supported: palette (color type 3), 16-bit depth, Adam7 interlacing. Verified genuinely bidirectionally against a real PNG library (Pillow, not just self-consistency): (1) round trip through this package's own Encode+Decode; (2) this package's own encoded PNG opened by `PIL.Image.open` and every pixel of a 16x12 gradient image compared exactly; (3) a real PNG written by Pillow (confirmed via manual chunk inspection to use filter types Sub AND Paeth, not the trivial all-zero case) decoded byte-for-byte correctly by this package's `Decode`, proving the general unfilter logic against real adaptively-filtered data) |
| `image` | **in** (bounded: `Point`/`Rectangle` -- real Go's own half-open-on-Max shape, `Add`/`Sub`/`In`/`Eq`/`Dx`/`Dy`/`Empty`/`Intersect`/`Union` -- the `Image` interface, and ONE concrete pixel buffer type, `RGBA` (`Pix`/`Stride`/`Rect`, real Go's own `image.RGBA` layout). No `NRGBA`/`Gray`/`Paletted` concrete types, no `Decode`/format registry, no `Draw`. Found and fixed two more real, general, and fairly significant compiler bugs building this -- both C++ declaration-completeness/name-lookup ordering issues, neither hit before because no earlier package combined a plain-struct-typed interface method with a struct whose OWN method references a LATER-declared struct: (1) an interface method using a plain STRUCT type (not a pointer/builtin/other-interface) -- `Image.Bounds() Rectangle` -- needs that struct COMPLETE at the point its forwarding method body returns it, but interfaces are emitted near the top of the file, long before struct bodies; simply reordering (structs-before-interfaces) isn't safe either, since a struct's own methods commonly adapt themselves TO an interface (needing the interface's `adapt<T>` already declared) -- a genuine two-way dependency. Fixed by deferring every interface forwarding method's BODY to an out-of-line definition, flushed only after every struct is complete (declarations stay inline, unaffected). (2) The exact same completeness problem exists between ordinary STRUCTS too -- real Go doesn't care about declaration order at all (`Point`'s own `In(r Rectangle)` reading `r.Min.X`, with `Rectangle` declared LATER in the same file, is completely ordinary Go -- literally how real Go's own image.go is written), but C++'s inline-method "complete-class context" only defers to right after that ONE struct's own closing brace, never end-of-file. Fixed the identical way: every struct method's body is now emitted out-of-line, after ALL structs' field-only skeletons exist, not inline during each struct's own definition. Fixing (2) surfaced a THIRD, narrower bug immediately: once every method's out-of-line definition sits after the WHOLE class (with every sibling member already declared), a method whose own NAME matches ANOTHER type's name (`hash/maphash`'s real `Hash.Seed() Seed` sitting next to `Hash.SetSeed(seed Seed)` -- a live regression this surfaced in an already-passing package, not a new one) shadows that type for every other out-of-line definition in the same struct, since ordinary C++ lookup finds the sibling member `Hash::Seed` before the outer type `Seed`. Fixed with an elaborated-type-specifier (`struct Seed`, which C++ guarantees always finds the type) for a struct type used directly as a parameter/return type in an out-of-line definition. All three fixes verified together: full suite re-run after each one, zero regressions) |
| `image/color` | **in** (bounded: `Color` interface, `Rgba`/`NRGBA`/`Gray`/`CMYK`/`Alpha` -- no Gray16/RGBA64/NRGBA64/Alpha16/YCbCr, no `Palette`. `Model`'s `modelFunc` wrapper (a struct holding a plain `func(Color) Color`, since a bare Go func can't have methods) matches real Go's own actual implementation exactly, not a simplification invented here. Real Go's first concrete type is `RGBA` with a method ALSO named `RGBA()` -- legal in real Go, not reproducible here (every method is emitted as a same-named C++ member function inside its receiver's struct body, and C++ always parses that as a constructor declaration; a general fix doesn't work either, since `Color`'s vtable trampoline is one `template<class T>` shared by every implementing type, and a single generic template body can't call a different C++ method name per T) -- renamed the struct to `Rgba` (case-only), same "rename the Go type, don't teach the compiler to mangle" precedent as `hash/fnv`'s `Digest32`/`Digest64`. Found and fixed two more real, general, previously-latent compiler bugs building this: (1) a package-scope global var with NO explicit type, whose initializer calls a same-package function (`var globalSrc = NewSource(seedFromClock())`, the exact shape `math/rand` already used successfully) broke with "call to undefined function" the moment a DIFFERENT package referenced that global -- the cross-package type-inference path re-ran `InferType` on the declaring package's own init expression using the CALLER's file as the unscoped-lookup context instead of the declaring package's, another instance of the recurring "asking an unscoped/wrong-package question" bug family. Fixed via a scoped `unscoped_lookup_pkg_` override (set only around that one recursive `InferType` call) that `LookupFreeFunc`/`LookupMethod`/`LookupStruct`/`LookupGlobalDecl`/`LookupInterface`/`LookupAlias` now consult before defaulting to the current file. (2) an interface method's call result type wasn't qualified across packages at all (`InferType`'s `LookupIfaceMethod` branch returned the bare unqualified declared type straight from the interface's OWN defining package, unlike the parallel struct-method branch, which already routed through `ResultTypeOfCall`/`QualifyResultType`) -- `color.Model.Convert(...)`'s result inferred as bare `Color` instead of `color.Color`. Fixed by routing that branch through `QualifyResultType` too. Also found and fixed a third, smaller general gap while investigating: a struct FIELD of function type, called via `x.f(...)` (needed by `modelFunc.Convert` calling its own `f` field) -- both `InferType` and `EmitCall`'s method-call resolution only ever checked `LookupMethod`/`LookupIfaceMethod`, never `LookupField` for a callable func-typed field; fixed by adding that fallback in both places. Verified with 6 checks: `RGBA()` interface dispatch, `Gray` luminance conversion (cross-checked against a hand-computed value), `CMYK`-to-`RGBA` white conversion, `NRGBA`-to-`RGBA` alpha premultiplication, and the `Black`/`White` constants) |
| `index/suffixarray` | **in** (bounded: `New`/`Lookup`/`Bytes` only, no `Read`/`Write` persistence -- there's no `gob` here to persist through anyway. Built with `sort.Slice` over suffix start indices (O(n log^2 n) comparison sort) rather than real Go's linear-time DC3/skew construction; `Lookup` itself is the same binary-search-over-sorted-suffixes cost as real Go) |
| `io` | **in** (partial) |
| `io/ioutil` | **in** (deprecated in real Go too -- `ReadAll`/`ReadFile`/`WriteFile` as direct pass-throughs to `io`/`os`. Not implemented: `ReadDir`/`TempFile`/`TempDir` (no directories or temp-file support in this project's `os`), `NopCloser` (needs `io.ReadCloser`, not added yet)) |
| `io/fs` | **in** (bounded: `FS`/`File`/`FileInfo` interfaces, `ValidPath`, generic `ReadFile`/`Stat` helpers operating on any caller-provided `FS` -- still no os-backed FS (an `os.DirFS`-equivalent wrapping `os.Stat`/`os.ReadDir`, which now exist -- see `os`'s own tracker line -- has not been built); no `ReadDir`/`Glob`/`WalkDir`/`Sub`, which would need a directory-bearing FS to exercise against (`path/filepath`'s own `WalkDir` -- see its tracker line -- is a separate, `os`-only implementation, not built on `io/fs.FS`). `FileMode` is a plain `uint32` (matching real Go's own underlying type) with no methods, same struct-receiver-only bound as `time.Duration` -- `IsDir` lives on `FileInfo` instead, matching real Go's own shape. Verified with an in-memory map-backed `FS` implementation: `ReadFile`/`Stat` through the interface, a not-found path, and `ValidPath` against 7 cases including the "." special case, leading/trailing slash, doubled slash, and a ".." element) |
| `log` | **in** (Print/Println/Fatal/Fatalln/Panic/Panicln plus Printf/Fatalf/Panicf, forwarding straight to `fmt.Printf`/`Sprintf`'s non-literal-format-string path -- see stdlib/log) |
| `log/slog` | **in** (bounded: a single concrete text-format `Logger`, no pluggable `Handler` interface, no `Group`/`LogAttrs`. `Level`/`LevelDebug`..`LevelError`, `LevelString` -- not a `Level.String()` method, same struct-receiver-only limitation as `time.Duration` -- `New`/`SetLevel`/`Debug`/`Info`/`Warn`/`Error`, package-level default-logger functions + `SetDefault`) |
| `log/syslog` | n/a |
| `maps` | **in** (pre-1.23 slice-returning shape -- no `iter.Seq`, this compiler has no range-over-func) |
| `math` | **in** (partial) |
| `math/bits` | **in** (OnesCount/LeadingZeros/TrailingZeros/Len/Reverse/RotateLeft, 32+64-bit, built with shifts/loops rather than hex literals since it predates hex-literal support -- see the compiler-bugs writeup) |
| `math/rand` | **in** (xorshift64* `Source`/`Rand` + package-level funcs, self-seeded from `time.Now()`) |
| `math/big` | **in** (bounded: `Int` only, sign + magnitude as base-1,000,000 `[]uint32` limbs (not real Go's base-2^32 internal representation -- decimal limbs make `String`/`SetString` trivial). `NewInt`/`Set`/`SetInt64`/`SetString`/`String` (base 10 only), `Add`/`Sub`/`Mul`/`Neg`/`Cmp`/`Sign`, `Quo`/`Rem` (truncated division) and `Div`/`Mod`/`DivMod` (Euclidean, `0 <= m < |y|`, matching real Go's own documented "unlike Go" convention for these four specifically), `Exp` (square-and-multiply with an optional modulus, extracting exponent bits by repeatedly dividing by two rather than a separate bit-extraction path, since decimal limbs have no cheap bitwise view). Division is schoolbook long division one limb at a time, each quotient digit found by binary search over `[0, 999999]` rather than digit estimation. Also implemented: GCD (extended Euclid) / ModInverse, SetBytes/Bytes (big-endian unsigned). NOT implemented: `Exp` with a negative exponent, bases other than 10. Verified against an independent Python bignum oracle: 30-digit add/sub/mul, a 30! factorial via repeated multiplication, sign/zero/malformed-string edge cases, PLUS (same round) Euclidean `DivMod` across all four sign combinations of dividend/divisor (the oracle itself had to be written carefully -- Python's own `divmod` follows the divisor's sign, not Euclidean's always-non-negative-remainder convention, so a naive "check against Python's `%`" would have silently validated the wrong convention), truncated `QuoRem`, plain and modular `Exp` (`7**128 mod 13`), and a large exact division (`20! / 7!`) -- 27/27 checks correct) |
| `math/cmplx` | **in** (Complex struct {Real, Imag float64} -- this compiler has no complex128. New/Abs/Add/Sub/Mul/Div/Conj/Inv, not 1+2i literals) |
| `math/rand/v2` | **in** (real Go's own package clause for this import path is `rand`, not `v2` -- kept that way here too so caller code reads `rand.IntN(...)` like real Go, not `v2.IntN(...)`; importing both `math/rand` and `math/rand/v2` in the same program would collide, same constraint real Go places on it (needs an import alias) since this compiler namespaces by package clause not import path -- not exercised. `Source` is `Uint64() uint64`-shaped like real v2, not v1's `Int63()`. `NewPCG` is NOT the real PCG algorithm, same Rosetta-not-parity precedent as v1's xorshift64* and `hash/maphash`. No package-level `Seed` (matches real v2, which dropped it deliberately), no `N[T]` generic entry point -- concrete `IntN`/`Int32N`/`Int64N`/`Uint64N` cover the same ground) |
| `mime` | **in** (`TypeByExtension`/`AddExtensionType` only -- no `ParseMediaType`/`FormatMediaType`, no `/etc/mime.types`. Seeded with real Go's own built-in default table, a faithful subset not invented values) |
| `mime/multipart` | **in** (bounded: `Writer.WriteField`/`WriteFile` take the whole value/data at once, not real Go's `CreateFormField`/`CreateFormFile` shape (which return an `io.Writer` to stream into); self-generated boundary built from `math/rand`, not cryptographically random. `Reader` parses the whole body up front at `NewReader` time (via `io.ReadAll`), same bounded shape as `encoding/csv`'s `Reader`; `Part.Header` is parsed directly (one header per line, no RFC 822 continuation folding) rather than routed through `net/textproto.Reader`. Verified round-tripping a `WriteField` + a `WriteFile` through `Writer`/`Close` and back through `Reader.NextPart`/`FormName`/`FileName`/`Read`, plus the end-of-parts `io.EOF` signal) |
| `mime/quotedprintable` | **in** (bounded: `Writer` streams real encoding with 76-column soft line breaks and correct trailing-whitespace escaping, but always normalizes a bare `\n` to canonical `\r\n` (unlike Python's `quopri`, which leaves it alone) -- a deliberate Rosetta-not-parity choice, not a bug. `Reader` is bounded like `encoding/csv`'s -- decodes the whole input up front via `io.ReadAll` at `NewReader` time rather than truly streaming. Decode logic (soft-break and `=XX` handling) cross-checked against Python's `quopri.decodestring`. Found and fixed a real, general compiler bug building it: a `[]byte{...}` composite literal with an element that indexes a `string` constant (`hexDigits[b>>4]`) emitted the C++ `operator[]` result -- `char` -- uncast into a `wasigo::Slice<uint8_t>` brace-init list; Go's own type system already says string-indexing yields `byte`, but the C++ side didn't match it, and clang (not MSVC, which only warned) rejects that as a narrowing-conversion error. Fixed in `EmitIndex` by casting a string index to `uint8_t` at the point of indexing) |
| `net` | **in** (`Dial`/`Listen`/`Listener.Accept`/`Conn.Read`/`Write`/`Close` and `ListenPacket`/`DialPacket`/`PacketConn.ReadFrom`/`WriteTo` are all real via `gocvm.Call("net.*", ...)` on a `goclang++.bat --shim-sandbox` build -- real Winsock sockets that actually move bytes (a handle table on the shim_sandbox side, not just a reachability probe), see the GocVM diary entry above. Plain wasm32-wasip1 still has no socket syscalls at all on this target (verified directly in wasi-libc's own header, not a todo) and falls back unchanged to the original local-loopback-only stack. `Pipe()` DOES work for real on both builds, though: an in-memory, synchronous, full-duplex `Conn` pair needing no sockets at all -- same idea real Go's own `net.Pipe` uses, just channel-backed here instead of mutex/cond-backed -- see the "Where we are" writeup) |
| `net/http` `net/http/cgi` `net/http/cookiejar` `net/http/fcgi` `net/http/httptest` `net/http/httptrace` `net/http/httputil` `net/http/pprof` `net/netip` `net/rpc` `net/rpc/jsonrpc` `net/smtp` | n/a (needs real sockets, which `net` above confirms don't exist for this target -- not attempted) |
| `net/mail` | **in** (partial -- `ParseAddress`/`ParseAddressList` for `"Name <addr@host>"`/bare `addr@host`, comma-separated lists; no RFC 5322 date/message parsing, no `ReadMessage`) |
| `net/textproto` | **in** (partial -- pure string/header handling, no `Conn`/`Pipeline`: `CanonicalMIMEHeaderKey`, `MIMEHeader` + free `Header{Get,Set,Add,Del,Values}` functions since methods need a struct receiver, `Reader.ReadLine`/`ReadMIMEHeader` with continuation-line folding) |
| `net/url` | **in** (`QueryEscape`/`QueryUnescape`/`PathEscape`/`PathUnescape`, `URL` struct + `Parse`/`String`, `ParseQuery` -- no userinfo/port split, `ParseQuery` returns a flat `map[string]string` not real Go's multi-value `Values`) |
| `os` | **in** (builtin: Args/Exit/Getenv/File/Stdout/Stdin/Stderr, `Stat`/`FileInfo`, `ReadDir`/`DirEntry` -- real directory listing via `<dirent.h>` opendir/readdir/closedir, genuine WASI `fd_readdir`, not a stub) + **rt** (Setenv/process still todo) |
| `os/exec` | **in** (`Run`/`Output`/`CombinedOutput`/`Start`/`Wait` are all real via `gocvm.Call("os.exec"/"os.exec.start"/"wait"/"stdout.read", ...)` on a `goclang++.bat --shim-sandbox` build (real `CreateProcess`; `Start`+`Wait` stream real output into `Cmd.Stdout`/`Stderr` via a pump goroutine, see the GocVM diary entry above); plain wasm32-wasip1 still has no subprocess support, so those fall back to the same clear "not supported" error as before. `LookPath` stays stubbed -- no PATH-search topic exists yet; `Cmd.Stdin` is always wired to `NUL` (no interactive input) and stdout/stderr can't be separated (the real backend combines them into one pipe) |
| `os/user` | **in** (`Current`/`Lookup`/`LookupId` are all real via `gocvm.Call("os.user", ...)` on a `goclang++.bat --shim-sandbox` build (real `GetUserNameW`/`LookupAccountNameW`/`LookupAccountSidW`+`NetUserGetInfo`, see the GocVM diary entry above); plain wasm32-wasip1 falls back to the same "not supported" error as before) |
| `os/signal` | **in** (deliberate no-op, same honest-boundary shape as `os/exec`/`runtime` -- WASI preview1 delivers no signals to a wasm guest at all. `Notify`/`Stop`/`Ignore`/`Reset`; signals are a plain `int` (POSIX-numbered), not real Go's `os.Signal` interface, since `os` is a compiler builtin here. Found and fixed one more general compiler bug building this: `package signal` collided with the C standard library's global `signal()` -- same class of fix already applied to `log`/`rand`, extended to cover this name too) |
| `path` | **in** (partial) |
| `path/filepath` | **in** (partial, slash-only; now includes `Rel` and `WalkDir`/`SkipDir` built on `os.ReadDir` -- no `Abs`, no `os.Getwd` to build it from) |
| `plugin` | n/a |
| `reflect` | **in** (partial, read-only -- `TypeOf`/`ValueOf`, `Value`/`Type` with `Kind`/`Name`/`NumField`/`Field`/`FieldName`/`Interface`/`Int`/`Float`/`Bool`/`String`; no `Set*`/addressable values, no Slice/Map/Chan/Func Kind classification, `int`/`int64` and other same-width Go type pairs share one Kind since they're the same C++ type here -- see the compiler-bugs writeup) |
| `regexp` | **in** (real backtracking engine, not a stub -- literals/`.`/`*`/`+`/`?` greedy/char classes with ranges and `\d\D\w\W\s\S`/`^$` anchors/`\|` alternation/`(...)` grouping; no capture groups, no `{m,n}`, no non-greedy, no lookaround; `Compile`/`MustCompile`/`Match`/`MatchString`/`FindString`/`FindStringIndex`/`FindAllString`/`ReplaceAllString`/`Split`) |
| `regexp/syntax` | **in** (bounded to exactly the pattern subset this project's own `regexp` package supports: literals, `.`, `*`/`+`/`?`, char classes with ranges and `\d\D\w\W\s\S` shorthand, `^`/`$`, `\|` alternation, `(...)` capturing groups; no `{m,n}`, no non-greedy, no lookaround, `Flags` accepted but ignored. A real, independent parser producing a public `Op`-tagged `Regexp` tree (`Sub`/`Rune`/`Cap` fields, same shape real Go's own package uses) -- NOT wired in to back this project's own `regexp` package, which keeps its separate unexported `node`-based parser (predates this package). Char classes are byte-range based (0-255), not full Unicode; a negated class is expanded into its literal complement ranges at parse time exactly like real Go (no separate negate flag on `Regexp`). Verified with 8 checks: `OpConcat`/`OpStar`/`OpAlternate`/`OpCharClass`/`OpCapture`/`OpPlus` tree shapes, negated-class range expansion (`[^a-c]` -> `[0,96]`+`[100,255]`), `\d` shorthand, and a malformed-pattern (unclosed group) error) |
| `runtime` | **in** (bounded, honestly: `GC()`/`Gosched()` are real no-ops, not fake hooks -- plain Go source here cannot reach `gc::heap().Collect()` or the coroutine scheduler without compiler-level special-casing, same as `os.Getenv`/`time.Now`, AND `wasigo::New<T>()` isn't routed through Oilpan yet regardless (see the Rosetta table), so a real collection pass would find nothing Go-visible to collect either way. Real Go's own `GC()`/`Gosched()` make zero behavioral guarantees, so a no-op is a valid, non-breaking implementation of the documented contract. `GOMAXPROCS`/`NumCPU`/`NumGoroutine` all correctly report `1` (one thread); `GOOS`/`GOARCH` are `"wasip1"`/`"wasm"`, the real target) |
| `runtime/cgo` `runtime/race` | n/a |
| `runtime/debug` | **in** (same honest-no-op reasoning as `runtime` itself -- `SetGCPercent`/`FreeOSMemory`/`SetMaxStack`/`SetMaxThreads`/`SetPanicOnFault` have nothing real to tune; `Stack()` returns an empty slice, a real documented gap not a fabricated trace) |
| `runtime/metrics` | **in** (same honest-no-op reasoning as `runtime/debug`: this runtime tracks no metrics at all -- single-threaded, no GC/scheduler instrumentation wired up -- so `All()` returns no descriptions and `Read` leaves every sample at its zero value, the correct terminal shape rather than a placeholder. Bounded: no `KindFloat64Histogram`/`Float64Histogram` -- every metric this runtime could ever produce is scalar, nothing here to histogram) |
| `runtime/pprof` | **in** (bounded: `NewProfile`/`Lookup`/`Profiles`/`(*Profile).Add`/`Remove`/`Count`/`Name` are real bookkeeping over a named custom-profile registry -- costs nothing, needs no OS support -- but `StartCPUProfile`/`WriteHeapProfile`/`(*Profile).WriteTo` all return a clear "not supported" error, same honest-terminal-shape precedent as `os/exec`: this target has no sampling profiler or symbolized-stack support to actually produce a pprof-format profile from) |
| `runtime/trace` | **in** (`Start` returns a clear "not supported" error, `Stop` a real no-op -- this target has no execution tracer, same shape as `runtime/pprof`'s own stubbed half) |
| `runtime/coverage` | **in** (bounded to the `io.Writer`-based half of real Go's API -- `WriteMeta`/`WriteCounters`/`ClearCounters`, all returning "not supported" -- no `WriteMetaDir`/`WriteCountersDir`, which take a directory path this project's `os` package has no directory support for anyway) |
| `slices` | **in** (partial: Contains/Index/Equal/Reverse/Sort/IsSorted/Max/Min/Clone/Insert/Delete/Concat) |
| `sort` | **in** (partial) |
| `strconv` | **in** (partial) |
| `strings` | **in** (partial) |
| `sync` | **in** (`Mutex`/`RWMutex` no-op, `Once`/`WaitGroup` counter-tracked -- see stdlib/sync; no `Map`, needs `any` equality this compiler doesn't have) |
| `sync/atomic` | **in** (legacy function-based API + the Go 1.19+ `Int32`/`Int64`/`Uint32`/`Uint64`/`Bool`/`Value` types; one thread, so every op is a plain load/store/compare) |
| `syscall` | **in** (see its own tracker line below; Getpid/Getppid/Getenv/Environ/Chdir/Kill real via `gocvm.Call` on a `goclang++.bat --shim-sandbox` build) |
| `testing` | **in** (bounded, deliberately not real Go's `go test` machinery -- this compiler has no build-time scanning to discover `func TestXxx(t *testing.T)` functions in a package, so a program using this package must call its own test functions itself; `Run(name, fn)` is a small helper that does that and prints a `--- PASS`/`--- FAIL` line like `go test -v`. No `Errorf`/`Fatalf`/`Logf` -- same documented wall as `log`'s own package comment: `fmt.Sprintf`'s format string must be a literal at the call site, so a Printf-shaped wrapper taking a `format string` parameter can never work here; `Error`/`Fatal`/`Log` take `...any` and space-join instead, matching `log.Print`'s own bound. `FailNow`/`Fatal` don't actually unwind the test function early (no real stack unwinding here at all, already documented elsewhere) -- just set the failed flag, a stated behavior difference. Verified with a passing and a deliberately failing test, checking `Run`'s own returned bool matches printed PASS/FAIL) |
| `testing/iotest` | **in** (partial: `OneByteReader`/`HalfReader`/`ErrReader`/`DataErrReader`. `DataErrReader` reads the wrapped Reader to completion up front (this project's "buffer, don't stream" precedent) and always attaches `io.EOF` specifically as the terminal error rather than preserving the wrapped Reader's own real error like real Go does -- a stated narrowing, not an oversight, since EOF is what this helper is actually for in practice. NOT implemented: `TimeoutReader` (no real deadline concept here), `TestReader` (a fuller conformance checker). Verified the specific behavior each wrapper exists to exercise: `OneByteReader` returns exactly 1 byte from a 10-byte buffer, `HalfReader` returns exactly half a requested 8-byte read, `ErrReader` returns the exact wrapped error, and `DataErrReader` on a single-byte source returns `(1, io.EOF)` from the SAME call -- the specific shape the real helper is meant to catch a caller mishandling) |
| `testing/fstest` | **in** (bounded: `TestFS` only, and much narrower than real Go's actual version, which walks the whole tree via `ReadDir` and cross-checks structure -- this project's `io/fs` has no `ReadDir` at all (no os-backed FS ever will either), so this bounded version only confirms every path in the expected list actually opens and reads via `fs.ReadFile`. Verified against an in-memory `FS`: a fully-satisfiable expected list returns nil, one including a missing path returns a non-nil error) |
| `testing/quick` | **in** (Check(func(int) bool) and CheckString only -- not arbitrary types, same read-only-reflect wall) |
| `testing/slogtest` | **in** (Exercise a slog.Logger into a bytes.Buffer; no Handler interface to TestHandler against) |
| `structs` | **in** (HostLayout marker type, matching real Go 1.23+) |
| `text/tabwriter` | **in** (bounded: buffers everything until `Flush`, splits on `\n`/`\t`, computes each column's width from every row's non-final cell, pads (`AlignRight` supported). No column-block resets, no `FilterHTML`/`StripEscape`/`DiscardEmptyColumns`/`TabIndent`/`Debug`. Verified against Go's own documented tabwriter example output, not just self-consistency) |
| `text/scanner` | **in** (bounded: `Init(io.Reader)`, `Scan`/`Next`/`Peek`/`TokenText`/`Pos`; classifies `Ident`/`Int`/`Float`/`Char`/`String`/`RawString` plus single-rune punctuation, `//` and `/* */` comments always skipped -- no `Mode`/`Whitespace` bitmask, no `Error` hook, no `IsIdentRune` override, and (like `encoding/csv`'s `Reader`) slurps the whole input up front via `io.ReadAll` rather than truly streaming. Adding this package's `package scanner` alongside the pre-existing `go/scanner` (also `package scanner`) found a real, general, and pretty severe latent build-system bug: `wasigoc`'s generated per-package header filename was derived from only the *last* path segment of the import path (`DeriveGenFilename` in `src/main.cc`), so any two packages sharing a directory basename -- `go/scanner` vs `text/scanner`, `net/http/pprof` vs `runtime/pprof`, etc. -- silently clobbered each other's header in the shared `--out-dir` with no build error at all, whichever got generated last. Broke `go/scanner`'s own already-passing golden test purely from build-order luck the moment this package existed, plus everything downstream of it (`go/parser`/`go/printer`/`go/format`/`go/types`). Fixed by folding the immediate parent directory into the filename too (`go_scanner_gen.hpp` / `text_scanner_gen.hpp`) -- every existing generated header got renamed as a side effect, which is harmless (build artifacts, nothing references the old names)) |
| `text/template` | **in** (bounded: `{{.Field}}`/`{{.Field.Nested}}` substitution and nestable `{{if .Field}}...{{else}}...{{end}}`, built on this project's read-only `reflect`. `{{range}}` is NOT implemented -- a hard blocker, not a scope choice: this project's `reflect` reports `Invalid` Kind for slices/maps and has no indexing, so there is no way to iterate a slice field at all. Also no `{{with}}`, pipelines, functions, variables, or template sets. One tagged `node` struct (Kind enum) instead of a parse-tree interface hierarchy, same simplification precedent as `go/ast`'s Node. Field lookup walks `NumField`/`FieldName` by name (no `FieldByName` in this project's reflect). Found and fixed a real, general compiler bug building this: `panic(err)` where `err` is a Go `error` value didn't compile at all (`wasigo::panic` had overloads for `std::string`/`const char*`/`int64_t` but not this project's `Error` type) -- `panic(x)` with a non-string/non-int argument had apparently never been exercised by any earlier stdlib package. Fixed by adding a `panic(Error)` overload in `runtime.hpp` that forwards to the string overload; `template.Must` (which does exactly `panic(err)`) is the first stdlib code to depend on it. Also found (not fixed, worked around): calling a function with another function's multi-return result forwarded directly as its argument list (`f(g())` where `g` returns exactly the two values `f` takes) -- `template.Must(template.New(name).Parse(text))`, real Go's own idiomatic one-liner, failed with "call has 1 argument(s) but 2 expected"; worked around in this package's own golden test by capturing `Parse`'s two return values into named locals first, then passing them to `Must` separately. Verified with 7 checks: field substitution across a nested struct, `if`/`else` both branches, a falsy int field correctly skipping its `if` body, and two malformed templates (unterminated action, missing `{{end}}`) both correctly rejected) |
| `text/template/parse` | **in** (bounded to exactly the subset this project's own `text/template` supports -- plain text, `{{.Field}}`/`{{.Field.Nested}}` actions, nestable `{{if .Field}}...{{else}}...{{end}}` -- NOT real Go's actual wide Node interface hierarchy (ListNode/TextNode/PipeNode/ActionNode/IfNode/RangeNode/...); one tagged `Node` struct (`Type` enum) instead, same simplification precedent as `go/ast`'s own Node. An independent second tokenizer/parser (not a wrapper -- no way to share `text/template`'s unexported internals across packages), rebuilt to hand back a public tree instead of executing inline; `Parse(name, text) (*Tree, error)` rather than real Go's `map[string]*Tree` multi-template return, since there's no `{{define}}`/`{{template}}` here to produce more than one. Verified two ways against real Go itself (go1.26.4, installed locally): (1) every template string this package's golden test parses was independently confirmed to parse without error under real Go's actual `text/template/parse.Parse`; (2) walking this package's own returned Tree by hand for a `{{if .Ok}}yes{{else}}no{{end}}` template and computing the substitution independently matched, byte for byte in both the true and false branches, this project's OWN `text/template.Template.Execute` running the identical template string. Found (not a new bug, hit the same known limitation `text/template`'s own tracker entry already documents): `template.Must(template.New(name).Parse(text))` -- forwarding a call's multi-return result directly as another call's argument list -- still fails with "call has 1 argument(s) but 2 expected"; worked around the same way, capturing `Parse`'s two return values into named locals first) |
| `time` | **in** (Duration, `Time`, `Now`, no-op Sleep -- see the table above) |
| `time/tzdata` | n/a (real Go's own package has exactly one purpose -- blank-imported to embed zoneinfo data for `time.LoadLocation` to read -- and this project's `time` package doesn't have `LoadLocation`/locations at all, UTC-only (see its own tracker line); embedding zone data with nothing in `time` able to consume it would be a stub with no real caller-visible effect, not the honest kind this project's other stubs are (`os/exec`/`runtime/trace`'s stubs each replace a real callable entry point with a "not supported" error -- there's no equivalent single entry point here to stand in for) |
| `unicode` | **in** (ASCII + Latin-1 classification only, no full category tables) |
| `unicode/utf16` | **in** |
| `unicode/utf8` | **in** (partial) |
| `unsafe` | **in** (tiny: Pointer is uint64 -- this compiler has no uintptr builtin -- Add/PointerFromInt/IntFromPointer. No Sizeof/Alignof/Offsetof -- those are compiler builtins) |
| `syscall` | **in** (Getpid/Getppid/Getenv/Environ/Chdir/Kill real via `gocvm.Call` on a `goclang++.bat --shim-sandbox` build (real Win32 process/env/directory/termination calls, see the GocVM diary entry above); Getwd="." always (no real Getwd topic); plain wasm32-wasip1 falls back to the same canned values/errors as before) |

## Build

`wasigoc` is a host-side dev tool (like `brujac`/`voodoomc`). It is built
with MSVC/clang on the machine, never as a WASI module. Its *output* is
what targets WASM.

```
cmake -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

CMake looks for wasi-sdk at, in order: `-DWASIGO_WASI_SDK_PATH=...`,
`$WASI_SDK_PATH`, `%USERPROFILE%\wasi-sdk`, `$HOME/wasi-sdk`. On this
machine that is `C:\Users\grego\wasi-sdk`. It prefers the triple wrapper
`bin\wasm32-wasip1-clang++.exe` over a bare `clang++`.

Tests:

| Test | What it proves |
| --- | --- |
| `runtime_smoketest` | `src/runtime.hpp` (goroutines, `TaskT<T>`, channels, slices, Error, recover, type_key, Oilpan-lite) on the host |
| `<example>_native` | generated C++ compiles and runs on the host compiler |
| `<example>_golden` | generated C++ compiles to `wasm32-wasip1` with wasi-sdk; if `wasmtime` is findable, the module is actually *run* under it and stdout is checked; otherwise only the `.wasm` magic number is checked |

Without wasi-sdk, `_golden` tests are omitted; native tests still run.

`wasmtime` (as of 2026-08-28: `wasmtime 48.0.1`, installed at
`C:\Users\grego\wasmtime-v48.0.1-x86_64-windows\wasmtime.exe`, not on
`PATH`) is located by CMake's `find_program(WASIGO_WASMTIME ...)`, or set
explicitly with `-DWASIGO_WASMTIME=<path>\wasmtime.exe` on the `cmake -S .
-B build` configure line (needed here, since it isn't on `PATH`) --
re-running that configure command is required after installing/moving it,
`cmake --build` alone won't pick up a cache change. `check_wasm.cmake`
invokes it as `wasmtime run --dir=.::. <module>.wasm`: the `--dir` preopens
the ctest working directory into the WASI sandbox as `.`, which real
os.File goldens (`osfile`) need for `WriteFile`/`Open`/`Create` to succeed
instead of failing every op with "cannot create file". For expected output
containing literal backslash-n/t/`"` sequences that would collide with
`EXPECTED_OUTPUT`'s own `"\\n"`-means-newline substitution (e.g. printed
JSON, see `jsonpkg`), drop a fixture file at
`tests/golden/expected/<name>.txt` (exact bytes, real newlines) instead --
`wasigo_add_golden` prefers it over the inline `EXPECTED_OUTPUT` string
when both exist.

## Compile to wasm

Do **not** use `clang++ --target=wasm32-wasip1` alone. On wasi-sdk 34
(LLVM 23) that default include path is:

1. `.../wasm32-wasip1/noeh/c++/v1`  (no-exceptions libc++)
2. `.../wasm32-wasip1/c++/v1`       (full libc++, including its own `ctype.h` / `errno.h`)
3. clang resource dir
4. wasi-libc (`.../wasm32-wasip1`, where the real `ctype.h` lives)

`#include <iostream>` then pulls libc++'s `ctype.h` (it defines
`_LIBCPP_CTYPE_H`) instead of wasi-libc's. libc++'s `<cctype>` treats that
as a bug and errors out; `<cstring>` never sees `memcpy`; `<cerrno>` never
sees `errno`. The same default also passes `-fexceptions`, so the object
file refers to `__cxa_throw` which noeh `libc++abi` does not provide.

The flags that actually work -- used by `tests/golden/run_golden.cmake` --
are: the triple wrapper, no exceptions, wipe the default C++ include path,
then add noeh libc++ *then* wasi-libc.

```
set WASI=%USERPROFILE%\wasi-sdk
set SYS=%WASI%\share\wasi-sysroot

build\Release\wasigoc.exe examples\hello\hello.go -o hello_gen.cpp

%WASI%\bin\wasm32-wasip1-clang++.exe -O2 -std=c++20 -fno-exceptions ^
  -nostdinc++ ^
  -isystem %SYS%\include\wasm32-wasip1\noeh\c++\v1 ^
  -isystem %SYS%\include\wasm32-wasip1 ^
  -isystem %SYS%\include ^
  -o hello.wasm hello_gen.cpp
```

`-fno-exceptions` is also the language mapping: user `panic` in a function
with `defer` is a `goto` to that function's epilogue (`PanicFrame` +
`DeferList`), not `throw`. Runtime panics (index out of range, send on a
nil/closed channel) call `abort`.

A program that uses `go` / `chan` / `select` gets `#define WASIGO_NEED_CORO 1`
in the generated TU so `src/runtime.hpp` includes `<coroutine>` and the
scheduler; programs that do not (hello, fib, structs) skip that.

## License

BSD-3-Clause.
