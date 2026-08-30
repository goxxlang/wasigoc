# wasigoc
<img width="1408" height="768" alt="1788049219961" src="https://github.com/user-attachments/assets/3f1444b7-70d6-49a2-be85-b0c7db589ed2" />

**Go++ is the language. `wasigoc` is the compiler.**

* **GitHub:** [goxxlang/wasigoc](https://github.com/goxxlang/wasigoc)
* **License:** BSD-3-Clause (`LICENSE`)

Go++ is Go *syntax* with a C++/WASM *runtime*: Oilpan GC, cooperative goroutines, and no data races. `wasigoc` reads a (restricted) `.go` file and emits C++ for [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) `wasm32-wasip1`.

```
Go++ source (.go)  ──>  wasigoc  ──>  C++ (.cpp)  ──>  wasm32-wasip1-clang++  ──>  .wasm

```

This is not `gc` and not a Go runtime port. `wasm32-wasip1` is one thread and has no growable stacks. The design is a **Rosetta**: keep the shape of the Go++ source, spell each construct as the C++ feature that is actually strong on WASM. See [docs/language.md](https://www.google.com/search?q=docs/language.md).

---

## Quickstart

### 1. Write Go++ Source (`hello.go`)

```go
package main

import "fmt"

func main() {
	fmt.Println("hello, wasi")
}

```

### 2. Transpile to C++

```bash
wasigoc examples/hello/hello.go -o hello_gen.cpp

```

### 3. Compile to WASM

Compile `hello_gen.cpp` with wasi-sdk's **triple wrapper** and the noeh include order — a bare `clang++ --target=wasm32-wasip1` will not work. See [docs/build.md](https://www.google.com/search?q=docs/build.md) for required flags.

---

## Ecosystem & Names

| Name | Description |
| --- | --- |
| **Go++** | The language |
| **`wasigoc`** | The compiler, and this repository ([goxxlang/wasigoc](https://github.com/goxxlang/wasigoc)). Compiles a whole Go++ program, including `int main()`. |
| **shim_sandbox** | Sibling repository: Pipe/bus, extra G++ stubs, ABAC System shim ([goxxlang/shim_sandbox](https://github.com/goxxlang/shim_sandbox)) |

---

## What's In

* **Frontend:** Recursive-descent Go frontend with automatic semicolon insertion.
* **Modules & Scope:** Packages implemented as C++ namespaces; supports `go.mod` `replace` directives and `internal/` access rules.
* **Standard Library:** Four builtins (`fmt`, `errors`, `os`, `reflect`) plus **146** compiled packages under `stdlib/` — matching public `go list std` minus `internal/`/`vendor/` and target-impossible APIs.
* **Concurrency:** Cooperative `go` / `chan` / `select` powered by C++20 coroutines.
* **Codecs & Hashes:** Real codecs and hashes (`flate`/`gzip`/`zlib`/`bzip2`/`lzw`, `PNG`/`JPEG`/`GIF`, `SHA-2`/`3`, `AES-128`, `P-256`, `Ed25519`, …) with documented bounds.
* **Syscall Handling:** Honest stubs where WASI preview 1 has no syscalls (`os/exec`, `net.Dial`; `net.Pipe()` is real and is what [shim_sandbox](https://github.com/goxxlang/shim_sandbox) speaks).

Language surface and Rosetta table: [docs/language.md](https://www.google.com/search?q=docs/language.md).

Stdlib status: [docs/stdlib.md](https://www.google.com/search?q=docs/stdlib.md).

Per-package tracker and compiler-bug diary: [docs/design-log.md](https://www.google.com/search?q=docs/design-log.md).

---

## Build Instructions

CMake 3.16+, C++20 (MSVC or clang). Note that `wasigoc` is a **host** binary.

```bash
cmake -B build
cmake --build build
ctest --test-dir build --output-on-failure

```

* **MSVC Note:** Pass `-C Debug` to `ctest`.
* **Optional Tools:** Install [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) for `_golden` wasm tests; install [wasmtime](https://wasmtime.dev/) to *run* those modules.

Details, wasi-sdk include order, and the `wasigoc` CLI: [docs/build.md](https://www.google.com/search?q=docs/build.md).

---

## Directory Layout

```text
src/         wasigoc (lexer, parser, generator) + runtime.hpp
stdlib/      Go++ standard library (ordinary .go)
examples/    goldens (hello, rosetta, per-package programs)
tests/       runtime smoketest + wasm golden harness
docs/        language, stdlib, build, design log

```

---

## Documentation Index

| Document | Contents |
| --- | --- |
| [docs/language.md](https://www.google.com/search?q=docs/language.md) | Go++ syntax, Rosetta, modules |
| [docs/stdlib.md](https://www.google.com/search?q=docs/stdlib.md) | Builtins, stubs, n/a, how to grow |
| [docs/build.md](https://www.google.com/search?q=docs/build.md) | CMake, ctest, compile to wasm |
| [docs/design-log.md](https://www.google.com/search?q=docs/design-log.md) | Full tracker + compiler-bug writeup |
| [CONTRIBUTING.md](https://www.google.com/search?q=CONTRIBUTING.md) | Names, goldens, style |
| [SECURITY.md](SECURITY.md) | What this is not; shim_sandbox ABAC |
