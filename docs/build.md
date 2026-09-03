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

## Compile a program natively (`goclang++`)

`wasigoc`'s own output isn't wasm-specific -- the generated C++ is
plain C++20 with `src/runtime.hpp` inlined, so it compiles equally well
with a **host** compiler, no wasi-sdk involved. That's how the
`<example>_native` ctest targets already work (host g++/MSVC), and it's
also how a Go++ program gets to link real host libraries wasm32-wasip1
can't reach at all -- no sockets, no threads, no exceptions there (see
"Compile a program to wasm" above). `goclang++.bat` is that path as a
standalone tool, using clang++ instead of the wasi-sdk triple wrapper:

```
goclang++.bat hello.go                    -> hello.exe (gocvm+ABAC linked by default)
goclang++.bat hello.go -o out.exe
goclang++.bat foo.go --no-shim-sandbox     no gocvm.Call bridge at all
goclang++.bat foo.go --no-abac             bridge linked, skip -DW2G_ABAC_SYSTEM=1
goclang++.bat foo.go --shim-sandbox        same default, but a missing/unbuilt
                                            shim_sandbox is now a hard error
```

It needs `wasigoc.exe` already built (`compile.bat` with no `.go` file)
and a native clang++ -- checked at `CLANGXX_PATH`, `%LLVM_DIR%\bin`,
`C:\Program Files\LLVM\bin`, then `PATH`. No wasi-sdk flags at all: no
`-fno-exceptions`, no noeh include order, no sysroot -- this is
ordinary host C++.

**Target triple defaults to `x86_64-w64-mingw32`** (override with
`GOCLANG_TARGET`), matching the mingw g++ this workspace already
builds `wasigoc.exe` and [shim_sandbox](../../shim_sandbox)'s `libw2g.a`
with -- so linking against it needs no ABI mismatch handling and no
shim_sandbox rebuild. A different target triple (e.g. an MSVC-ABI
clang-cl setup) needs shim_sandbox rebuilt with that same ABI first, or
linking fails with mismatched name-mangling/STL errors.

shim_sandbox + ABAC are linked **by default** -- `gocvm.Call`'s
`ErrorState` machinery (`src/runtime.hpp`) surfaces every bridge
failure, including an internal bridge panic or an ABAC deny, as a real
Go `error` instead of ever aborting the process, so there's no safety
reason left to keep this opt-in (see design-log.md's GocVM diary).
`goclang++.bat` looks for shim_sandbox at `%SHIM_SANDBOX_DIR%` or the
sibling `../shim_sandbox`; if it's missing or not yet built
(`cmake -B build && cmake --build build` there), the build silently
falls back to no bridge at all (`gocvm.Call` reports "no host bridge
registered", same as always) rather than failing. Pass `--shim-sandbox`
explicitly to turn a missing/unbuilt shim_sandbox into a hard error
instead (useful in CI, where a silent fallback would be the wrong
failure mode); `--no-shim-sandbox` skips the bridge entirely;
`--no-abac` keeps the bridge but skips `-DW2G_ABAC_SYSTEM=1` --
consumers of `w2g::Shim` need that define themselves (not just the
library) to actually reach `fopen`/`getenv` rather than compile-time
deny; a runtime ABAC policy is still required on top of the define (see
shim_sandbox's `docs/abac.md`).

## wasigoc CLI

```
wasigoc <input.go> -o <output.cpp> [--import-dir=DIR ...] [--out-dir=DIR]
```

`--out-dir` is where per-package `*_gen.hpp` headers land. Import
resolution is documented in [language.md](language.md).
