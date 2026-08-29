# Security

Go++/`wasigoc` is a compiler and a cooperative WASM runtime. It is not
a sandbox by itself.

- Generated code panics (or `abort`s) on slice/map/chan misuse instead
  of C++ UB. That is memory-safety relative to the Go++ source, not
  isolation from the host.
- wasm32-wasip1 guests see only what the WASM runtime preopens (`os.File`
  via wasi-libc `fopen`).
- Host OS access from extra G++ layers belongs in
  [shim_sandbox](https://github.com/goxxlang/shim_sandbox) `w2g::Shim`:
  compile-time `-DW2G_ABAC_SYSTEM=1` plus a runtime ABAC allow. Without
  that define, the shim never calls `fopen`.
- Crypto packages that exist for interop (`crypto/rc4`, `crypto/des`,
  textbook RSA/DSA) are documented as broken by modern standards.
  `crypto/rand` is a time-seeded PRNG, not a CSPRNG.

Report issues against this repository. There is no separate bounty
program.
