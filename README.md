# wasigoc (go++ fork)

This directory is **`~/go++`**, a fork of WASIGo++. See [FORK.md](FORK.md).

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
| **`goclang++`** | Same `wasigoc` frontend, compiled to a **native** host `.exe` with clang++ instead of wasm32-wasip1 -- no WASI restrictions (real exceptions/threads/sockets), so it links `shim_sandbox` directly. `goclang++.bat`, [docs/build.md](docs/build.md) |
| **GocVM** | `gocvm.Call(topic, payload)` -- the one compiler-known dispatch gate (`src/runtime.hpp`'s `wasigo::gocvm`, one builtin function, not per-package FFI) that real stdlib source (`net`, `crypto/tls`, `os/exec`, `os/user`, `syscall`) calls to reach a real host bridge, linked by default under `goclang++.bat` (`--no-shim-sandbox` opts out). No bridge (plain `wasigoc`/wasm32-wasip1, or `--no-shim-sandbox`) -> the same honest stub errors as always; a real bridge failure (including an internal bridge panic) surfaces as a genuine Go `error`, never a swallowed fallback. Diary: [docs/design-log.md](docs/design-log.md) |
| **shim_sandbox** | Sibling: Pipe/bus, extra G++ stubs, ABAC System shim, and GocVM's real backend (real sockets/processes/TLS/user lookups) ([goxxlang/shim_sandbox](https://github.com/goxxlang/shim_sandbox)) |

`wasigoc` compiles a whole Go++ program, including `int main()`.

## What's in

- Recursive-descent Go frontend (automatic semicolon insertion).
- Packages as C++ namespaces; `go.mod` replace + `internal/` rules.
- Five builtins (`fmt`, `errors`, `os`, `reflect`, `gocvm`) + **146**
  compiled packages under `stdlib/` — public `go list std` minus
  `internal/`/`vendor/` and target-impossible APIs.
- Cooperative `go` / `chan` / `select` (C++20 coroutines).
- Real codecs and hashes (flate/gzip/zlib/bzip2/lzw, PNG/JPEG/GIF,
  SHA-2/3, AES-128, P-256, Ed25519, …) with documented bounds.
- A clear "not supported" error, never a silent no-op, where WASI
  preview 1 has no syscalls at all (`os/exec`, `net.Dial`,
  `crypto/tls.Dial`, `os/user.Lookup`, `syscall.Chdir`, …; `net.Pipe()`
  is real and is what [shim_sandbox](https://github.com/goxxlang/shim_sandbox)
  speaks) -- and genuinely real, not stubbed, when compiled with
  `goclang++.bat` (shim_sandbox + ABAC link **by default** now that
  GocVM's `ErrorState` machinery surfaces every bridge failure,
  including an internal panic, as a real Go error instead of ever
  aborting -- `--no-shim-sandbox`/`--no-abac` opt back out): real
  sockets, real subprocesses with streamed output, a real TLS handshake
  (certificate validation always on), real user/env lookups, via GocVM.
  See the "Names" table above and [docs/design-log.md](docs/design-log.md)'s
  diary.
- Type identity: interned `go/types` (pointer equality) and matching
  C++ codegen — methods on defined types (`type Duration int64`),
  generic named types (`type Set[T any] struct`), anonymous
  `interface{ M() }`, range-over-func, named array/slice types.
- GC: Oilpan-lite (cppgc) — `GarbageCollected<T>`, `Member<T>`,
  `Persistent<T>`, stop-the-world mark-sweep via `Trace`/`Visitor`. No
  Go collector clone — C++ Oilpan on a single-thread target.
- VThreads: cooperative virtual threads registered through the same
  Oilpan-lite GC (`gocvm::VThread`) — goroutines are C++20 stackless
  coroutines on a single runqueue (no OS threads, no wasi-threads), and
  a `VThread` also tracks a goroutine suspended on a real GocVM host
  call (`State::kAwaitingHost`) so that call doesn't block every other
  goroutine while it's in flight.

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
