# go++ (fork of WASIGo++)

This is a working copy of `~/WASIGo++`. **Do not edit `~/WASIGo++` from this
tree.** Changes here are the goxxlang netstack + JSON work.

## Userspace netstack (`stdlib/net`)

WASI preview 1 still has no sockets. The stack is `net.Pipe`:

| API | What it is |
|-----|------------|
| `Pipe()` | reliable ordered duplex (TCP-shaped, buffered) |
| `Listen`/`Dial` `"tcp"` | loopback: Dial hands a Pipe end to `Accept` |
| `ListenPacket`/`DialPacket` `"udp"` | connected UDP: DialPacket attaches a framed Pipe |
| `PacketPipe()` | UDP-shaped datagram pair without a bind |
| `SplitHostPort` / `JoinHostPort` | `"host:port"` and `[::1]:port` |
| `net/http` | HTTP/1.0 `Get`/`Post`, `Serve`/`ServeHandler`, `ServeMux` |

`Dial("tcp", "example.com:80")` still fails — no host sockets. Bind two
guests later by attaching a Pipe as the uplink.

## JSON

`encoding/json` Unmarshal into a struct pointer, nested structs, and
Marshal of a slice of structs. Field `Set*` writes through `adapt_ptr`.
Struct tags `json:"name"` and `json:"-"` rename or omit fields.
