# wasigocvm

**wasigocvm** is the web-native Go++ machine: our compiler driver +
sysroot contract. It is **not** "stock wasip2 with better flags."

Stock Bytecode Alliance wasip2 is a Component Model world and a
triple that has sockets but no pthread. Flag-tuning that triple cannot
grow Oilpan + type_key + full libc++ + threads as one product. So the
product gate is `-DWASIGO_GOCVM=1` and `wasigocvm.bat`; `toolchain/`
now ships our own sysroot (built via `toolchain/bootstrap.sh`, see
below), still paired with the stock `wasm32-wasip2-clang++` binary
since a sysroot-only bootstrap run has no bundled compiler of its own.

```
Go++  →  wasigoc  →  C++  →  wasigocvm clang++ (full libc++)  →  .wasm
                              │
                              ├─ Oilpan (wasigo::gc)
                              ├─ type_key interning (no itables)
                              ├─ gocvm net (wasigocvm_net.hpp + poll)
                              └─ threads when toolchain/sysroot has them
```

## Four pillars

| Pillar | Status |
| --- | --- |
| **Full libc++** | eh headers/libs, `-fexceptions -frtti`, standard WASM EH |
| **Oilpan** | `runtime.hpp` (`GarbageCollected` / `Member` / `Persistent`) |
| **Type-id interning** | `type_key_of<T>()` + `go/types` Object Type Identifier |
| **Threads** | Our sysroot exists but `threads: false` -- needs a >=23 host clang to build (>=22 got us exceptions only); gocvm uses `poll()` workers meanwhile |

## Build / run

```
wasigocvm.bat examples\httppkg\main.go -o httppkg.wasm
..\shim_sandbox\tools\w2g-run.bat httppkg.wasm
```

`compile.bat --wasip2` is **retired**. Prefer `WASIGO_TOOLCHAIN` /
`toolchain/sysroot`; fallback is still a wasi-sdk with `eh/`.

ctest: `hello_wasigocvm`, `netpkg_wasigocvm`, `httppkg_wasigocvm`.

## Host bridge: os.exec / os.user / syscall / tls.dial

`net.*` runs entirely in-guest (`wasigocvm_net.hpp` opens real sockets from
inside the wasm32 sandbox itself). The topics that categorically cannot --
`os.exec.*`, `os.user`, `syscall.*`, `tls.dial` (real `CreateProcess`/
`GetUserNameW`/Schannel) -- stay honest failures by default ("unsupported on
wasigocvm ... build with --shim-sandbox"). `--host-bridge`
(`wasigocvm.bat`/`.sh`, `-DWASIGOCVM_HOST_BRIDGE=1`) opts a build into
forwarding those topics instead, over loopback TCP, to a companion native
process:

```
..\shim_sandbox\build\gocvm_host.exe [port]     # default port 47821
wasigocvm.bat --host-bridge examples\hostbridge\main.go -o hostbridge.wasm
wasmtime run --dir=.::. -S inherit-network -S tcp ^
  --env WASIGOCVM_HOST_ADDR=127.0.0.1:47821 hostbridge.wasm
```

`gocvm_host.exe` (`shim_sandbox/src/gocvm_host.cc`) accepts exactly one
connection, decodes each request frame, runs it through `W2gSapiHandle` --
the same real backend `goclang++.bat --shim-sandbox` links in-process, no
logic duplicated -- and writes the reply back, then exits. One process per
wasm-guest run, matching `w2g-run.*`'s own launch/teardown.

ctest: `hostbridge_native`/`hostbridge_golden`/`hostbridge_wasigocvm` (no
bridge -- the documented fallback, `1`) and `hostbridge_hostbridge` (real
bridge: compiles with `WASIGOCVM_HOST_BRIDGE=1`, starts a real
`gocvm_host.exe`, and checks the guest's `syscall.Getpid()` comes back as
that process's *actual* live pid, not the fallback). The last one needs a
PowerShell driver (`tests/golden/run_hostbridge_golden.ps1`), not
`check_wasm.cmake`'s single static COMMAND -- a companion process has to be
started between compiling and running the wasm, then torn down after.
Skipped when `~/shim_sandbox/build/gocvm_host.exe` isn't built or no
PowerShell is found (see the `wasigo: gocvm_host at ...` / `wasigo: no
gocvm_host.exe ...` CMake configure message).

## Rewrite note

Guest net used to key off `__wasip2__` and live in `wasip2_net.hpp`.
That made the stock triple the product identity. It now keys off
`WASIGO_GOCVM` and `wasigocvm_net.hpp`.

Bootstrap: `toolchain/bootstrap.sh` / `bootstrap.ps1`, CI workflow
`wasigocvm-toolchain`. Feature probes: `src/wasigocvm_config.hpp`.

## 2026-09-09: first real bootstrap.sh run, sysroot wired in

Ran `bootstrap.sh` end to end for the first time (previously written but
never actually executed) and fixed three real bugs it hit: wasi-sdk's
CMake needs a Clang >=22 host compiler (not GCC), `wasm-component-ld`
(a separate bytecodealliance tool) is a silent hard requirement for any
wasm32-wasip2/wasip3 `-shared` link, and `WASI_SDK_EXCEPTIONS` needs
`DUAL` (not `ON`) to produce the `eh/`+`noeh/` split layout the drivers
already probed for. Also: `ln -sfn` from WSL onto a `/mnt/*` prefix
makes a symlink native Windows tools can't see (a dead reparse point) --
copy instead. `CMakeLists.txt` / `wasigocvm.bat` / `wasigocvm.sh` now
pair a sysroot-only toolchain install with the stock wasi-sdk's
clang++, since a plain `--target install` run has no bundled compiler
of its own (that's wasi-sdk's separate, unverified `dist` target,
`--dist`). Verified: all 3 `*_wasigocvm` goldens pass against the new
sysroot, full suite 286/286. Threads still `false` -- the host clang
we built with (22.1.8, `apt.llvm.org`'s newest as of this date) is
below the >=23 `COOP_THREADS_POSSIBLE` gate; next real step is finding
a >=23 clang (or building LLVM ourselves) and re-bootstrapping.
