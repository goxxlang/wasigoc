# wasigoc
<img width="1408" height="768" alt="1788049219961" src="https://github.com/user-attachments/assets/3f1444b7-70d6-49a2-be85-b0c7db589ed2" />

**Go++ is the language. `wasigoc` is the compiler.**

* **GitHub:** [goxxlang/wasigoc](https://github.com/goxxlang/wasigoc)
* **License:** BSD-3-Clause (`LICENSE`)

Go++ is Go *syntax* with a C++/WASM *runtime*: Oilpan GC, cooperative goroutines, no data races. `wasigoc` reads a (restricted) `.go` file and emits C++ for [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) `wasm32-wasip1`.

```
Go++ source (.go)  ──>  wasigoc  ──>  C++ (.cpp)  ──>  wasm32-wasip1-clang++  ──>  .wasm
```

This is not `gc` and not a Go runtime port. `wasm32-wasip1` is one thread and has no growable stacks. The design is a **Rosetta**: keep the shape of the Go++ source, spell each construct as the C++ feature that is actually strong on WASM. See [docs/language.md](docs/language.md).

---

## Quickstart

### 1. Write Go++ source (`hello.go`)

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

Compile `hello_gen.cpp` with wasi-sdk's **triple wrapper** and the noeh include order — a bare `clang++ --target=wasm32-wasip1` will not work. See [docs/build.md](docs/build.md) for required flags.

### 4. Or compile to a native binary

`goclang++.bat` runs the same `wasigoc` frontend but targets a host `.exe` via clang++ instead of wasm32-wasip1 — no WASI restrictions (real exceptions, threads, sockets). It links [shim_sandbox](https://github.com/goxxlang/shim_sandbox) by default, so `net`, `os/exec`, `os/user`, `syscall`, and `crypto/tls` become real (real sockets, real subprocesses, a real TLS handshake) instead of returning "not supported". See [docs/build.md](docs/build.md).

---

## Ecosystem & names

| Name | Description |
| --- | --- |
| **Go++** | The language |
| **`wasigoc`** | The compiler, and this repository ([goxxlang/wasigoc](https://github.com/goxxlang/wasigoc)). Compiles a whole Go++ program, including `int main()`. |
| **`goclang++`** | Native-binary build of the same frontend (`goclang++.bat`) — see Quickstart above |
| **GocVM** | `gocvm.Call(topic, payload)`, the one dispatch gate `net`/`os/exec`/`os/user`/`syscall`/`crypto/tls` use to reach a real host bridge under `goclang++`. Non-blocking: a call suspends only the calling goroutine, not the whole scheduler, and a bridge failure (including an internal panic) comes back as a normal Go `error`. Diary: [docs/design-log.md](docs/design-log.md) |
| **shim_sandbox** | Sibling repository: Pipe/bus, extra G++ backends, ABAC System shim, and GocVM's real implementation (sockets/processes/TLS/user lookups) ([goxxlang/shim_sandbox](https://github.com/goxxlang/shim_sandbox)) |

---

## What's in

* **Frontend:** Recursive-descent Go frontend with automatic semicolon insertion.
* **Type identity:** Interned `go/types` (pointer equality) and matching C++ codegen — methods on defined types (`type Duration int64`), generic named types (`type Set[T any] struct`), named array/slice types, anonymous `interface{ M() }`, range-over-func.
* **Modules & scope:** Packages as C++ namespaces; `go.mod` `replace` directives and `internal/` access rules.
* **Standard library:** Five builtins (`fmt`, `errors`, `os`, `reflect`, `gocvm`) plus **146** compiled packages under `stdlib/` — matching public `go list std` minus `internal/`/`vendor/` and target-impossible APIs.
* **Concurrency:** Cooperative `go`/`chan`/`select` on C++20 coroutines. Real thread-safety in `Chan`/`Map`/the GC heap (`Chan` matches Go's own concurrent-use guarantee; `Map` panics on detected concurrent writes the same way Go's runtime does, rather than silently corrupting).
* **GC:** Oilpan-lite (`cppgc`) — `GarbageCollected<T>`, `Member<T>`, `Persistent<T>`, stop-the-world mark-sweep. Not a Go collector clone.
* **VThreads:** A registry of virtual goroutine threads riding on the same GC. A VThread waiting on a GocVM host call can be mapped to the real OS thread serving it (`gocvm.OSThreadFor`).
* **Codecs & hashes:** Real codecs and hashes (`flate`/`gzip`/`zlib`/`bzip2`/`lzw`, `PNG`/`JPEG`/`GIF`, `SHA-2`/`3`, `AES-128`, `P-256`, `Ed25519`, …) with documented bounds.
* **Syscall handling:** A clear "not supported" error, never a silent no-op, where WASI preview 1 has no syscalls at all (`os/exec`, `net.Dial`, `crypto/tls.Dial`, `os/user.Lookup`, `syscall.Chdir`, …). `net.Pipe()` works everywhere — it's local, in-memory duplex. Compiled with `goclang++` instead, the same calls are real: real sockets, real subprocesses with streamed output, a real TLS handshake, real user/env lookups, all via GocVM.

Language surface and Rosetta table: [docs/language.md](docs/language.md). Stdlib status: [docs/stdlib.md](docs/stdlib.md). Per-package tracker and compiler-bug diary: [docs/design-log.md](docs/design-log.md).

---

## Build instructions

CMake 3.16+, C++20 (MSVC or clang). `wasigoc` is a **host** binary.

```bash
cmake -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

* **MSVC note:** pass `-C Debug` to `ctest`.
* **Optional tools:** install [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) for `_golden` wasm tests; install [wasmtime](https://wasmtime.dev/) to *run* those modules.

Details, wasi-sdk include order, and the `wasigoc` CLI: [docs/build.md](docs/build.md).

---

## Directory layout

```text
src/         wasigoc (lexer, parser, generator) + runtime.hpp
stdlib/      Go++ standard library (ordinary .go)
examples/    goldens (hello, rosetta, per-package programs)
tests/       runtime smoketest + sync stress test + wasm golden harness
docs/        language, stdlib, build, design log
```

---

## Documentation index

| Document | Contents |
| --- | --- |
| [docs/language.md](docs/language.md) | Go++ syntax, Rosetta, modules |
| [docs/stdlib.md](docs/stdlib.md) | Builtins, stubs, n/a, how to grow |
| [docs/build.md](docs/build.md) | CMake, ctest, compile to wasm and to native |
| [docs/design-log.md](docs/design-log.md) | Full tracker + compiler-bug diary |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Names, goldens, style |
| [SECURITY.md](SECURITY.md) | What this is not; shim_sandbox ABAC |
