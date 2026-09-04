# Contributing

**Go++** is the language. **wasigoc** is the compiler. This repo is
[goxxlang/wasigoc](https://github.com/goxxlang/wasigoc).

## Names

| Name | What it is |
| --- | --- |
| Go++ | The language: Go syntax, C++/WASM runtime model |
| `wasigoc` | The compiler: `.go` → C++ (this repository) |
| shim_sandbox | Sibling: Pipe/bus, extra G++ stubs, ABAC System shim ([goxxlang/shim_sandbox](https://github.com/goxxlang/shim_sandbox)) |

Do not call the language "wasigoc" or the compiler "Go++".

## Layout

| Path | Role |
| --- | --- |
| `src/` | `wasigoc` (lexer, parser, generator, `runtime.hpp`) |
| `stdlib/` | Go++ standard library as ordinary `.go` |
| `examples/` | Goldens: a small Go++ program per feature or package |
| `tests/` | Runtime smoketest + wasm golden harness |
| `docs/` | Language, build, stdlib, design log |

## Growing the stdlib

1. Write Go under `stdlib/<import-path>/*.go` (skip `*_test.go`; one
   directory is one package). `wasigoc` already searches `stdlib/`.
2. Add `examples/<name>/main.go` that imports it and prints checks.
3. Register a golden in `CMakeLists.txt` with `wasigo_add_golden` and
   `DEPENDS` on the new `.go` files.
4. Prefer implementing in Go. Touch `src/runtime.hpp` or generator
   builtins only for WASI (`os` fds, `time.Now`) or the four builtins
   (`fmt`, `errors`, `os`, `reflect`).
5. Bounded is fine. Stubs that return a clear "not supported" error are
   fine. Pretending a WASI-impossible API works is not.
6. Update [docs/stdlib.md](docs/stdlib.md) and the tracker in
   [docs/design-log.md](docs/design-log.md).

## Compiler changes

`wasigoc` is a host tool (MSVC or clang). Its *output* targets
`wasm32-wasip1`. A generator change is not done until:

- a golden compiles with wasi-sdk's `wasm32-wasip1-clang++` (noeh
  include order — see [docs/build.md](docs/build.md))
- if `wasmtime` is available, that module *runs* and stdout matches

Do not use a bare `clang++ --target=wasm32-wasip1`.

## Tests

```
cmake -B build
cmake --build build
ctest --test-dir build -C Debug --output-on-failure
```

On single-config generators omit `-C Debug`. Native tests always run.
`_golden` tests need [wasi-sdk](https://github.com/WebAssembly/wasi-sdk);
with `wasmtime` on `PATH` (or `-DWASIGO_WASMTIME=...`) they execute the
module.

## Style

Match surrounding Go++: no methods on non-struct defined types, state
actual bounds plainly in the package comment. Comments explain a
constraint, not the commit.
