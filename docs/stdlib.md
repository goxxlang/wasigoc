# Go++ standard library

Public `go list std` minus `internal/` / `vendor/`, compiled as ordinary
`.go` under `stdlib/` the same way `strings` is. Do **not** add compiler
builtins unless the package must touch WASI or the Rosetta runtime (`os`
fds, `time.Now`). One thread / no growable stacks still applies.

**Status:** 4 builtins + 146 compiled packages. Everyday ported Go++
source compiles. A package that can't exist on wasm32-wasip1 returns a
clear "not supported" error or is marked n/a — it never fakes success.

Per-package notes, bounds, and the compiler bugs each package
surfaced: [design-log.md](design-log.md) (tracker from
`### Tracker` onward).

## Builtins (not `.go`)

| Package | In | Missing |
| --- | --- | --- |
| `fmt` | `Print`/`Println`/`Sprint`/`Sprintln`/`Printf`/`Sprintf`/`Errorf`/`Fprint*` | format string must be a **literal**; verbs `%d %s %f %v %t %c %w %%` only; no `Scan*` |
| `errors` | `New`, `Is`, `Unwrap`, `Join` | `As` |
| `os` | `Args`, `Exit`, `Getenv`, `File` (`Open`/`Create`/`ReadFile`/`WriteFile`, `Read`/`Write`/`Close`), std streams, `Stat`/`FileInfo`, `ReadDir`/`DirEntry` (real, via `stat(2)`/`opendir`+`readdir`) | `Setenv`, process, `Remove`/`Mkdir` |
| `reflect` | `TypeOf`/`ValueOf`, `Value`/`Type` (`Kind`/`Name`/`NumField`/`Field`/`FieldName`/`Interface`/`Int`/`Float`/`Bool`/`String`, `Set*`) | no Chan/Func Kind |

Host file I/O from a WASI guest should go through
[shim_sandbox](https://github.com/goxxlang/shim_sandbox) `w2g::Shim`
(compile-time ABAC), not ambient `fopen`.

## Honest stubs (WASI cannot do this)

These compile and return a clear error. They are the terminal shape,
not a todo: `os/exec`, `os/user`, `net.Dial` to a real host (loopback
`Listen`/`Dial` `"tcp"` and `Pipe()` are real), `crypto/tls`,
`syscall` (mutating), `runtime/trace`, `runtime/pprof` write half,
`embed`.

`net.Pipe()` is the duplex [shim_sandbox](https://github.com/goxxlang/shim_sandbox) speaks.
`net/http` is HTTP/1.0 over that stack (`Get`/`Post`/`Serve`/`ServeMux`),
not sockets on the host.

## n/a on this target

`plugin`, `runtime/cgo`, `runtime/race`, `log/syslog`, `time/tzdata`
(this `time` is UTC-only).

## Partial (real, bounded)

Everything else under `stdlib/` is present and exercised by a golden.
Typical bounds, also in each package comment:

- crypto: SHA-2/3, HMAC, HKDF, PBKDF2, AES-128, DES/RC4 (legacy),
  textbook RSA/DSA, P-256 ECDH/ECDSA, Ed25519. No TLS handshake.
- compress/image: real codecs, often decode-general / encode-simple.
- `encoding/json`: Marshal/Unmarshal of structs via reflect (including
  `json:"name"` / `json:"-"` tags); Unmarshal into a struct pointer
  writes through settable Values.
- `go/*`: tokenizer/parser/printer plus an interned (object-type
  identity) checker: defined types by name, anonymous interfaces by
  method set, `Set[int]` instantiations, range-over-func signatures.
  wasigoc now emits those same shapes as C++ (defined-type method
  wrappers, class templates, interned iface adapters, yield loops).
- `math/cmplx`: `complex128` (`1+2i`, `real`/`imag`/`complex`).
- `regexp`: backtracking; no `{m,n}`, no non-greedy, no lookaround.
- `sync`: no-op mutexes; `WaitGroup.Wait` cannot block the runqueue.
- `time.Sleep`: no-op (must not block the cooperative scheduler).

## Extensions beyond `go list std`

`stdlib/unil` is not part of real Go's standard library — it's the
WASMUniLoader "unil" bill-of-materials format (files, runtime
components, capabilities, canonical JSON, SHA-256 digest, Ed25519
signing), a Go++ port of `~/WASMUniLoader/cpp/src/sbom.cc`'s C++ core
so a Go++ program can produce and verify byte-identical documents
without linking that C++ code. `import "unil"` resolves the same way
any stdlib package does (`wasigoc` searches `stdlib/` regardless of
whether the import path is real Go). See its package doc comment and
[design-log.md](design-log.md)'s tracker entry.

`stdlib/guac` builds on it: the on-disk shape of a distributable Go++
wasm package — a directory with compiled wasm file(s) plus a
`guac.json` manifest, which is an ordinary `unil.Document` (no schema
changes, just two naming conventions — see its package doc comment).
`HashFile`/`BuildManifest` hash files already on disk; `WriteManifest`/
`ReadManifest` persist and reload the manifest; `Verify` catches drift
between a directory and its manifest. It does not create directories
(no `Mkdir` in this `os`), compile anything, or fetch a dependency —
a `guac` CLI wrapping `wasigoc`/`goclang++` builds around this layer
doesn't exist yet.

Building it surfaced a real, previously-latent bug in
`crypto/ed25519`: the `dEd` curve constant had a digit transcription
error, so the compiled-in base point never actually satisfied the
curve equation — invisible until something did a genuine external
Sign-then-Verify round trip, which no existing golden did. Fixed; see
the tracker entry for `crypto/ed25519`.

## Growing it

See [CONTRIBUTING.md](../CONTRIBUTING.md). Tick the tracker in the
design log when a package lands.
