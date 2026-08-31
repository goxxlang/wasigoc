# WASIGo++ / wasigoc

This directory (`~/WASIGo++`) is the canonical checkout of
[goxxlang/wasigoc](https://github.com/goxxlang/wasigoc).

Language-surface work (lexer/parser completeness, interned `go/types`,
defined-type methods, generic named types, anonymous interfaces,
range-over-func) was developed in a `~/go++` working copy and merged
back here.

WASI preview 1 still has no sockets. The userspace stack is `net.Pipe`:

| API | What it is |
|-----|------------|
| `Pipe()` | reliable ordered duplex (TCP-shaped, buffered) |
| `Listen`/`Dial` `"tcp"` | loopback: Dial hands a Pipe end to `Accept` |
| `ListenPacket`/`DialPacket` `"udp"` | connected UDP: DialPacket attaches a framed Pipe |
| `PacketPipe()` | UDP-shaped datagram pair without a bind |
| `SplitHostPort` / `JoinHostPort` | `"host:port"` and `[::1]:port` |
| `net/http` | HTTP/1.0 `Get`/`Post`, `Serve`/`ServeHandler`, `ServeMux` |

`Dial("tcp", "example.com:80")` still fails — no host sockets.

`encoding/json` Unmarshal into a struct pointer, nested structs, and
Marshal of a slice of structs. Struct tags `json:"name"` and `json:"-"`
rename or omit fields.
