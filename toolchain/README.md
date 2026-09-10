# wasigocvm toolchain

This is **our** WASI/C++ sysroot contract for Go++, not stock wasip2
with extra `-f` flags.

| Pillar | Where it lives |
| --- | --- |
| Full **libc++** (exceptions + RTTI) | eh sysroot + `wasigocvm.bat` / `.sh` |
| **Oilpan** GC | `src/runtime.hpp` `wasigo::gc` |
| **type_key** / go/types interning | `type_key_of<T>()`, `go/types` Object Type Identifier |
| **Sockets** | sysroot libc + `src/wasigocvm_net.hpp` (poll workers) |
| **Threads** | needs pthread in sysroot; stamp `threads:false` until then |

## Bootstrap

```bash
# Linux / macOS / WSL
./toolchain/bootstrap.sh --jobs 8

# Windows → WSL
.\toolchain\bootstrap.ps1 -Jobs 8
```

Needs a **Clang >=22 host compiler** on PATH (wasi-sdk's own CMake refuses a
GCC host and gates `WASI_SDK_EXCEPTIONS` on >=22) -- `apt.llvm.org`'s
`llvm.sh 22` is the fastest way to get one on a stock Ubuntu box where the
default `clang` package is older. `bootstrap.sh` itself fetches the other
real prerequisite, `wasm-component-ld` (a separate Rust tool clang's
wasm32-wasip2/wasip3 driver shells out to for *any* `-shared` link, even
though it looks unrelated to exceptions/threads -- without it the build
fails deep in, with an opaque `posix_spawn failed: No such file or
directory` on `libc.so`).

By default this only builds the **sysroot** (`--target install`) using
whatever host clang it found -- no `bin/wasm32-*-clang++` wrapper scripts
come out of it, since those are wasi-sdk's `dist` target and may need
building LLVM/clang from scratch on top (`--dist`, unverified by this
project so far, budget real time for it). Pair the sysroot this produces
with an existing wasm32-wasip2-capable clang++ yourself until then.

Threads stay `false` regardless: `COOP_THREADS_POSSIBLE` in
`wasi-sdk-sysroot.cmake` needs a **>=23** host clang, a stricter gate than
the >=22 one exceptions need. `apt.llvm.org` didn't have anything past 22
as of 2026-09.

CI: `.github/workflows/wasigocvm-toolchain.yml` (workflow_dispatch) uploads
`wasigocvm-toolchain.tar.gz`. Unpack into `toolchain/` and set
`WASIGO_TOOLCHAIN` to that directory.

Install layout the drivers expect:

```
$WASIGO_TOOLCHAIN/
  wasigocvm-toolchain.json
  bin/wasm32-wasigocvm-clang++   # wrapper → wasip2 until we own a triple
  bin/wasm32-wasip2-clang++
  sysroot/                       # or share/wasi-sysroot (symlinked)
    include/wasm32-wasip2/eh/...
```

Drivers probe `wasm32-wasigocvm` eh layout first, then `wasm32-wasip2`.

## Driver (today)

```
wasigocvm.bat examples\httppkg\main.go -o httppkg.wasm
# or: ./wasigocvm.sh ...
..\shim_sandbox\tools\w2g-run.bat httppkg.wasm
```

Always: full libc++, `-DWASIGO_GOCVM=1`, standard WASM EH.

## Roadmap

1. **Now:** `WASIGO_GOCVM` + eh libc++ + Oilpan + type_key + poll net + bootstrap.
2. **Next:** land a CI-built sysroot under `toolchain/` on this machine.
3. **Then:** shared-everything-threads / patched wasi-libc → `WASIGO_GOCVM_HAS_PTHREAD`.

## Oilpan + type_key (not optional)

Rosetta, not extras: `wasigo::gc::*`, `type_key_of<T>()`, `go/types` OTIs.
RTTI is additive for C++ interop.

## Host

`w2g-run` defaults to `-W exceptions`. `W2G_THREADS=1` is a no-op until
the stamp says `threads: true`.
