# Build and wasm

`wasigoc` is a **host** tool (MSVC or clang on the machine). It never
runs as a WASI module. Its *output* targets `wasm32-wasip1`.

## Build wasigoc

Needs CMake 3.16+ and a C++20 compiler.

```
cmake -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

MSVC multi-config generators need `-C Debug` (or `Release`) on `ctest`:

```
ctest --test-dir build -C Debug --output-on-failure
```

### Tests

| Test | What it proves |
| --- | --- |
| `runtime_smoketest` | `src/runtime.hpp` on the host (tasks, channels, slices, Error, recover, Oilpan-lite) |
| `<example>_native` | generated C++ compiles and runs on the host compiler |
| `<example>_golden` | generated C++ compiles to `wasm32-wasip1`; if `wasmtime` is found, the module is *run* and stdout is checked |

Without [wasi-sdk](https://github.com/WebAssembly/wasi-sdk), `_golden`
tests are omitted; native tests still run.

CMake looks for wasi-sdk at, in order: `-DWASIGO_WASI_SDK_PATH=...`,
`$WASI_SDK_PATH`, `%USERPROFILE%\wasi-sdk`, `$HOME/wasi-sdk`. It prefers
the triple wrapper `wasm32-wasip1-clang++`.

`wasmtime` is found with `find_program`, or set
`-DWASIGO_WASMTIME=/path/to/wasmtime`. Re-run configure after installing
it. Goldens invoke:

```
wasmtime run --dir=.::. <module>.wasm
```

`--dir` preopens the ctest working directory as `.` so `os.File` goldens
can `Open`/`Create`. Expected output that contains literal `\n` / `\t` /
`"` (JSON, scanners) lives in `tests/golden/expected/<name>.txt` instead
of the inline `EXPECTED_OUTPUT` string.

## Compile a program to wasm

```
wasigoc examples/hello/hello.go -o hello_gen.cpp
```

Do **not** use `clang++ --target=wasm32-wasip1` alone. On wasi-sdk 34
(LLVM 23) the default include path puts noeh libc++ *and* full libc++
ahead of wasi-libc. `#include <iostream>` then pulls libc++'s `ctype.h`;
`<cctype>` errors; the default also passes `-fexceptions`, so the object
refers to `__cxa_throw` which noeh `libc++abi` does not provide.

The flags that work (same as `tests/golden/run_golden.cmake`):

```
set WASI=%USERPROFILE%\wasi-sdk
set SYS=%WASI%\share\wasi-sysroot

%WASI%\bin\wasm32-wasip1-clang++.exe -O2 -std=c++20 -fno-exceptions ^
  -nostdinc++ ^
  -isystem %SYS%\include\wasm32-wasip1\noeh\c++\v1 ^
  -isystem %SYS%\include\wasm32-wasip1 ^
  -isystem %SYS%\include ^
  -o hello.wasm hello_gen.cpp
```

POSIX:

```
WASI="${WASI_SDK_PATH:-$HOME/wasi-sdk}"
SYS="$WASI/share/wasi-sysroot"

"$WASI/bin/wasm32-wasip1-clang++" -O2 -std=c++20 -fno-exceptions \
  -nostdinc++ \
  -isystem "$SYS/include/wasm32-wasip1/noeh/c++/v1" \
  -isystem "$SYS/include/wasm32-wasip1" \
  -isystem "$SYS/include" \
  -o hello.wasm hello_gen.cpp
```

`-fno-exceptions` is also the language mapping: user `panic` in a
function with `defer` is a `goto` to that function's epilogue, not
`throw`. Runtime panics call `abort`.

A program that uses `go` / `chan` / `select` gets
`#define WASIGO_NEED_CORO 1` in the generated TU.

## wasigoc CLI

```
wasigoc <input.go> -o <output.cpp> [--import-dir=DIR ...] [--out-dir=DIR]
```

`--out-dir` is where per-package `*_gen.hpp` headers land. Import
resolution is documented in [language.md](language.md).
