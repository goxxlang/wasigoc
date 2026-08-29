# wasigoc

**Go++ is the language. `wasigoc` is the compiler.** GitHub: [goxxlang/wasigoc](https://github.com/goxxlang/wasigoc) (`++` is not a legal GitHub name).

Go++ is Go *syntax* with a C++/WASM *runtime*: Oilpan GC, cooperative
goroutines, no data races. `wasigoc` reads a (restricted) `.go` file and
emits C++ for [wasi-sdk](https://github.com/WebAssembly/wasi-sdk)
`wasm32-wasip1`.

```
Go++ source (.go)  →  wasigoc  →  C++ (.cpp)  →  wasm32-wasip1-clang++  →  .wasm
```

This is not `gc` and not a Go runtime port. wasm32-wasip1 is one thread
and has no growable stacks. The design is a **Rosetta**: keep the shape
of the Go++ source, spell each construct as the C++ feature that is
actually strong on WASM. See [docs/language.md](docs/language.md).

```go
package main

import "fmt"

func main() {
	fmt.Println("hello, wasi")
}
```

```
wasigoc examples/hello/hello.go -o hello_gen.cpp
```

Then compile `hello_gen.cpp` with wasi-sdk's **triple wrapper** and the
noeh include order — a bare `clang++ --target=wasm32-wasip1` will not
work. Flags: [docs/build.md](docs/build.md).

## Names

| Name | What it is |
| --- | --- |
| **Go++** | The language |
| **`wasigoc`** | The compiler, and this repository ([goxxlang/wasigoc](https://github.com/goxxlang/wasigoc)) |
| **shim_sandbox** | Sibling: Pipe/bus, extra G++ stubs, ABAC System shim ([goxxlang/shim_sandbox](https://github.com/goxxlang/shim_sandbox)) |

`wasigoc` compiles a whole Go++ program, including `int main()`.

## What's in

- Recursive-descent Go frontend (automatic semicolon insertion).
- Packages as C++ namespaces; `go.mod` replace + `internal/` rules.
- Four builtins (`fmt`, `errors`, `os`, `reflect`) + **146** compiled
  packages under `stdlib/` — public `go list std` minus
  `internal/`/`vendor/` and target-impossible APIs.
- Cooperative `go` / `chan` / `select` (C++20 coroutines).
- Real codecs and hashes (flate/gzip/zlib/bzip2/lzw, PNG/JPEG/GIF,
  SHA-2/3, AES-128, P-256, Ed25519, …) with documented bounds.
- Honest stubs where WASI preview 1 has no syscalls (`os/exec`,
  `net.Dial`; `net.Pipe()` is real and is what [shim_sandbox](https://github.com/goxxlang/shim_sandbox) speaks).

Language surface and Rosetta table: [docs/language.md](docs/language.md).
Stdlib status: [docs/stdlib.md](docs/stdlib.md). Per-package tracker and
compiler-bug diary: [docs/design-log.md](docs/design-log.md).

## Build

CMake 3.16+, C++20 (MSVC or clang). `wasigoc` is a **host** binary.

```
cmake -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

On MSVC, pass `-C Debug` to `ctest`. Optional:
[wasi-sdk](https://github.com/WebAssembly/wasi-sdk) for `_golden` wasm
tests; [wasmtime](https://wasmtime.dev/) to *run* those modules.

Details, wasi-sdk include order, and the `wasigoc` CLI:
[docs/build.md](docs/build.md).

## Layout

```
src/            wasigoc (lexer, parser, generator) + runtime.hpp
stdlib/         Go++ standard library (ordinary .go)
examples/       goldens (hello, rosetta, per-package programs)
tests/          runtime smoketest + wasm golden harness
docs/           language, stdlib, build, design log
```

## Docs

| Doc | Contents |
| --- | --- |
| [docs/language.md](docs/language.md) | Go++ syntax, Rosetta, modules |
| [docs/stdlib.md](docs/stdlib.md) | Builtins, stubs, n/a, how to grow |
| [docs/build.md](docs/build.md) | CMake, ctest, compile to wasm |
| [docs/design-log.md](docs/design-log.md) | Full tracker + compiler-bug writeup |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Names, goldens, style |
| [SECURITY.md](SECURITY.md) | What this is not; shim_sandbox ABAC |

License: BSD-3-Clause (`LICENSE`).
