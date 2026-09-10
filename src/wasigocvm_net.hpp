// wasigocvm guest net bridge: gocvm.Call + poll() workers.
//
// Product gate is WASIGO_GOCVM (set by wasigocvm.bat / goldens), not stock
// __wasip2__. Bytecode Alliance wasip2 is a Component Model world; we are
// not that guest. We keep gocvm as the dispatch gate, cooperative scheduler
// as the one logical thread until toolchain/ ships pthread, and shim_sandbox
// ABAC on the host. POSIX sockets come from the wasigocvm sysroot (today
// still wasi-libc via a wasip2 clang wrapper -- temporary).
//
// Blocking recv()/accept() would freeze every goroutine. Until threads exist
// in our sysroot, the worker pool is poll() on non-blocking fds, integrated
// with Scheduler::run() the same way Chan waiters already are.
//
// Included from runtime.hpp under WASIGO_GOCVM && WASIGO_NEED_CORO.
// Needs -I <repo>/src on the clang++ line.
#pragma once

#include <algorithm>
#include <cstdint>
#include <cstdlib>
#include <deque>
#include <string>
#include <unordered_map>
#include <vector>

namespace gocvm {

class WasigocvmNetBridge : public AsyncHostBridge {
 public:
  uint64_t Submit(const std::string& topic, const std::string& payload) override {
    uint64_t id = ++next_id_;
    Completion c;
    c.id = id;
    bool parked = false;
    dispatch(id, topic, payload, &c, &parked);
    if (!parked) completions_.push_back(std::move(c));
    return id;
  }

  bool PollOne(Completion* out) override {
    if (completions_.empty()) pump(0);
    return pop(out);
  }

  void WaitOne(Completion* out) override {
    // Cap each park so a single orphan reader cannot freeze the
    // cooperative scheduler. After enough empty pumps, fail the oldest
    // pending op so goldens surface "timeout" instead of hanging ctest.
    int idle = 0;
    while (completions_.empty()) {
      if (pending_.empty()) {
        Completion c;
        c.ok = false;
        c.err = "wasigocvm: WaitOne with no pending work";
        *out = std::move(c);
        return;
      }
      size_t before = pending_.size();
      pump(50);
      if (completions_.empty() && pending_.size() == before) {
        if (++idle > 100) {  // ~5s
          auto it = pending_.begin();
          Completion c;
          c.id = it->first;
          complete_err(&c, "error: i/o timeout");
          completions_.push_back(std::move(c));
          pending_.erase(it);
          idle = 0;
        }
      } else {
        idle = 0;
      }
    }
    pop(out);
  }

 private:
  enum class Op { kConnect, kAccept, kRead, kWrite, kReadFrom, kWriteTo, kHostRead };

  struct Sock {
    int fd = -1;
    bool udp = false;
    bool listen = false;
  };

  struct Parked {
    uint64_t id = 0;
    Op op = Op::kRead;
    uint64_t handle = 0;
    int fd = -1;
    short events = 0;
    int maxlen = 0;
    std::string remaining;
    std::string to_addr;
  };

  uint64_t next_id_ = 0;
  uint64_t next_handle_ = 0;
  std::unordered_map<uint64_t, Sock> socks_;
  std::unordered_map<uint64_t, Parked> pending_;
  std::deque<Completion> completions_;

  // Control connection to a companion native shim_sandbox host process
  // (see shim_sandbox/src/gocvm_host.cc), for the topics that categorically
  // cannot run inside a wasm32 sandbox (os.exec, os.user, syscall, tls.dial
  // -- real CreateProcess/GetUserNameW/Schannel). Only armed on first use
  // (WASIGOCVM_HOST_BRIDGE opt-in, see dispatch()'s fallback below); a
  // build/run that never touches these topics never dials out at all.
  //
  // Sends are synchronous (loopback, small frames -- a bounded retry, not
  // an unbounded spin, on the rare EWOULDBLOCK). Reads are multiplexed:
  // multiple requests can be genuinely in flight on the one shared stream
  // at once (unlike the send side, nothing here serializes them), so
  // there is exactly one sentinel `pending_` entry (kHostReadId) that
  // accumulates bytes and, each time it wakes, parses every complete
  // frame currently buffered and resolves each by the id INSIDE that
  // frame -- not by which pending_ entry poll() happened to pick.
  static constexpr uint64_t kHostReadId = ~0ull;
  int host_fd_ = -1;
  uint64_t host_handle_ = 0;
  std::string host_read_buf_;
  std::vector<uint64_t> host_wait_ids_;

  static bool would_block() {
    return errno == EAGAIN || errno == EWOULDBLOCK || errno == EINPROGRESS;
  }

  static std::string syserr(const char* what) {
    return std::string("error: ") + what + ": errno " + std::to_string(errno);
  }

  static bool split_first(const std::string& s, char sep, std::string* a, std::string* b) {
    auto i = s.find(sep);
    if (i == std::string::npos) {
      *a = s;
      *b = "";
      return false;
    }
    *a = s.substr(0, i);
    *b = s.substr(i + 1);
    return true;
  }

  static bool split_hostport(const std::string& hostport, std::string* host, std::string* port) {
    if (hostport.empty()) return false;
    if (hostport[0] == '[') {
      auto end = hostport.find("]:");
      if (end == std::string::npos) return false;
      *host = hostport.substr(1, end - 1);
      *port = hostport.substr(end + 2);
      return !port->empty();
    }
    auto i = hostport.rfind(':');
    if (i == std::string::npos) return false;
    *host = hostport.substr(0, i);
    *port = hostport.substr(i + 1);
    return !port->empty();
  }

  static bool is_udp(const std::string& network) {
    return network == "udp" || network == "udp4" || network == "udp6";
  }

  static bool fill_v4(const std::string& host, const std::string& port, bool bind,
                      sockaddr_in* out) {
    std::memset(out, 0, sizeof(*out));
    out->sin_family = AF_INET;
    int p = std::atoi(port.c_str());
    if (p <= 0 || p > 65535) return false;
    out->sin_port = htons(static_cast<uint16_t>(p));
    if (host.empty() || host == "*" || host == "0.0.0.0") {
      (void)bind;
      out->sin_addr.s_addr = htonl(INADDR_LOOPBACK);
      return true;
    }
    if (host == "localhost") {
      out->sin_addr.s_addr = htonl(INADDR_LOOPBACK);
      return true;
    }
    // Hard fork: numeric / loopback only. DNS is W2G_ALLOW_DNS on the host
    // (wasmtime ip-name-lookup); we do not call getaddrinfo from the guest
    // by default — it is ambient authority preview 1 also refused, and it
    // can block the one cooperative thread.
    return inet_pton(AF_INET, host.c_str(), &out->sin_addr) == 1;
  }

  static std::string fmt_v4(const sockaddr_in& a) {
    char buf[INET_ADDRSTRLEN] = {};
    inet_ntop(AF_INET, &a.sin_addr, buf, sizeof(buf));
    return std::string(buf) + ":" + std::to_string(ntohs(a.sin_port));
  }

  static int set_nonblock(int fd) {
    int fl = fcntl(fd, F_GETFL, 0);
    if (fl < 0) return -1;
    return fcntl(fd, F_SETFL, fl | O_NONBLOCK);
  }

  static int socktype(const std::string& network) {
    return is_udp(network) ? SOCK_DGRAM : SOCK_STREAM;
  }

  bool pop(Completion* out) {
    if (completions_.empty()) return false;
    *out = std::move(completions_.front());
    completions_.pop_front();
    return true;
  }

  uint64_t alloc(int fd, bool udp, bool listen) {
    uint64_t h = ++next_handle_;
    socks_[h] = Sock{fd, udp, listen};
    return h;
  }

  Sock* lookup(uint64_t h) {
    auto it = socks_.find(h);
    return it == socks_.end() ? nullptr : &it->second;
  }

  bool parse_handle(const std::string& s, uint64_t* out) {
    if (s.empty()) return false;
    for (char c : s) {
      if (c < '0' || c > '9') return false;
    }
    *out = static_cast<uint64_t>(std::strtoull(s.c_str(), nullptr, 10));
    return true;
  }

  void complete_ok(Completion* c, std::string reply) {
    c->ok = true;
    c->reply = std::move(reply);
    c->err.clear();
  }

  void complete_err(Completion* c, std::string err) {
    c->ok = true;  // gocvm.Call's (string, error): real backend failures are reply
    c->reply = std::move(err);
    c->err.clear();
  }

  void park(uint64_t id, Op op, uint64_t handle, int fd, short events, int maxlen,
            std::string remaining, std::string to_addr) {
    Parked p;
    p.id = id;
    p.op = op;
    p.handle = handle;
    p.fd = fd;
    p.events = events;
    p.maxlen = maxlen;
    p.remaining = std::move(remaining);
    p.to_addr = std::move(to_addr);
    pending_[id] = std::move(p);
  }

  void cancel_fd(int fd, const char* why) {
    std::vector<uint64_t> ids;
    for (auto& kv : pending_) {
      if (kv.second.fd == fd) ids.push_back(kv.first);
    }
    for (uint64_t id : ids) {
      Completion c;
      c.id = id;
      complete_err(&c, std::string("error: ") + why);
      completions_.push_back(std::move(c));
      pending_.erase(id);
    }
  }

  void dispatch(uint64_t id, const std::string& topic, const std::string& payload,
                Completion* c, bool* parked) {
    *parked = false;
    if (topic == "net.dial") {
      std::string network, address;
      split_first(payload, ' ', &network, &address);
      do_dial(id, network, address, c, parked);
      return;
    }
    if (topic == "net.listen") {
      std::string network, address;
      split_first(payload, ' ', &network, &address);
      do_listen(network, address, c);
      return;
    }
    if (topic == "net.accept") {
      do_accept(id, payload, c, parked);
      return;
    }
    if (topic == "net.io.read") {
      do_read(id, payload, c, parked);
      return;
    }
    if (topic == "net.io.write") {
      do_write(id, payload, c, parked);
      return;
    }
    if (topic == "net.io.readfrom") {
      do_readfrom(id, payload, c, parked);
      return;
    }
    if (topic == "net.io.writeto") {
      do_writeto(id, payload, c, parked);
      return;
    }
    if (topic == "net.io.close") {
      do_close(payload, c);
      return;
    }
    if (topic == "net.tcp.bind") {
      do_bind_probe("tcp", payload, c);
      return;
    }
    if (topic == "net.udp.bind") {
      do_bind_probe("udp", payload, c);
      return;
    }
#if defined(WASIGOCVM_HOST_BRIDGE)
    // Anything else (os.exec.*, os.user, syscall.*, tls.dial) forwards
    // as-is to the companion native host -- this bridge never needs to
    // know the individual topic shapes, W2gSapiHandle on the host side
    // already does.
    do_host_call(id, topic, payload, c, parked);
    return;
#else
    // Hard fork: topics that still need a native host (exec, tls/Schannel,
    // user db) stay honest failures, not a silent Pipe fallback, unless
    // WASIGOCVM_HOST_BRIDGE opts into the companion-process forwarder
    // above. isNoBridge is reserved for "this build has no bridge at all".
    c->ok = false;
    c->err = "unsupported on wasigocvm (needs shim_sandbox native host -- build with --shim-sandbox)";
#endif
  }

#if defined(WASIGOCVM_HOST_BRIDGE)
  // ver=1 | id u32 | topic_len u16 | topic | payload_len u32 | payload,
  // prefixed with a u32 total length covering everything after itself.
  // All little-endian. Mirrors shim_sandbox/include/w2g/msg.h's shape
  // closely enough to keep the two implementations easy to compare, but
  // deliberately self-contained -- this header has no shim_sandbox
  // include dependency otherwise, and every wasigocvm build compiles it.
  static void put_u16(std::string* o, uint16_t v) {
    o->push_back(static_cast<char>(v & 0xff));
    o->push_back(static_cast<char>((v >> 8) & 0xff));
  }
  static void put_u32(std::string* o, uint32_t v) {
    for (int i = 0; i < 4; ++i) o->push_back(static_cast<char>((v >> (8 * i)) & 0xff));
  }
  static uint16_t get_u16(const std::string& s, size_t off) {
    return static_cast<uint16_t>(static_cast<uint8_t>(s[off]) |
                                  (static_cast<uint16_t>(static_cast<uint8_t>(s[off + 1])) << 8));
  }
  static uint32_t get_u32(const std::string& s, size_t off) {
    uint32_t v = 0;
    for (int i = 0; i < 4; ++i) v |= static_cast<uint32_t>(static_cast<uint8_t>(s[off + i])) << (8 * i);
    return v;
  }

  static std::string encode_host_frame(uint64_t id, const std::string& topic,
                                       const std::string& payload) {
    std::string body;
    put_u32(&body, static_cast<uint32_t>(id));
    put_u16(&body, static_cast<uint16_t>(topic.size()));
    body += topic;
    put_u32(&body, static_cast<uint32_t>(payload.size()));
    body += payload;
    std::string frame;
    put_u32(&frame, static_cast<uint32_t>(body.size()));
    frame += body;
    return frame;
  }

  bool ensure_host_conn(std::string* err) {
    if (host_fd_ >= 0) return true;
    const char* addr_env = std::getenv("WASIGOCVM_HOST_ADDR");
    std::string hostport = (addr_env && *addr_env) ? addr_env : "127.0.0.1:47821";
    std::string host, port;
    if (!split_hostport(hostport, &host, &port)) {
      *err = "error: bad WASIGOCVM_HOST_ADDR " + hostport;
      return false;
    }
    sockaddr_in addr{};
    if (!fill_v4(host, port, /*bind=*/false, &addr)) {
      *err = "error: resolve " + hostport;
      return false;
    }
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) {
      *err = syserr("socket");
      return false;
    }
    // Blocking connect, deliberately: this is loopback-only, to a host
    // process the run wrapper (w2g-run.*) already started, so it either
    // succeeds near-instantly or fails fast with ECONNREFUSED -- neither
    // case risks a real stall of the one cooperative thread.
    if (connect(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
      *err = std::string("error: shim_sandbox host not reachable at ") + hostport +
             " (start it first -- see docs/wasigocvm.md): errno " + std::to_string(errno);
      close(fd);
      return false;
    }
    set_nonblock(fd);
    host_fd_ = fd;
    host_handle_ = alloc(fd, /*udp=*/false, /*listen=*/false);
    return true;
  }

  void do_host_call(uint64_t id, const std::string& topic, const std::string& payload,
                    Completion* c, bool* parked) {
    std::string err;
    if (!ensure_host_conn(&err)) {
      complete_err(c, err);
      return;
    }
    std::string frame = encode_host_frame(id, topic, payload);
    size_t off = 0;
    int idle_spins = 0;
    while (off < frame.size()) {
      ssize_t n = send(host_fd_, frame.data() + off, frame.size() - off, 0);
      if (n > 0) {
        off += static_cast<size_t>(n);
        idle_spins = 0;
        continue;
      }
      if (n < 0 && would_block() && ++idle_spins < 1000) continue;
      complete_err(c, n < 0 ? syserr("host send") : "error: host send stalled");
      drop_fd(host_fd_);
      close(host_fd_);
      host_fd_ = -1;
      return;
    }
    host_wait_ids_.push_back(id);
    if (!pending_.count(kHostReadId)) {
      park(kHostReadId, Op::kHostRead, host_handle_, host_fd_, POLLIN, 0, "", "");
    }
    *parked = true;
  }

  void fail_all_host(const std::string& why) {
    for (uint64_t wid : host_wait_ids_) {
      Completion fc;
      fc.id = wid;
      complete_err(&fc, why);
      completions_.push_back(std::move(fc));
    }
    host_wait_ids_.clear();
  }

  void host_pump_read() {
    char buf[4096];
    ssize_t n = recv(host_fd_, buf, sizeof(buf), 0);
    if (n == 0 || (n < 0 && !would_block())) {
      fail_all_host(n == 0 ? "error: shim_sandbox host closed connection"
                           : syserr("host recv"));
      drop_fd(host_fd_);
      close(host_fd_);
      host_fd_ = -1;
      host_read_buf_.clear();
      return;
    }
    if (n > 0) host_read_buf_.append(buf, static_cast<size_t>(n));
    while (host_read_buf_.size() >= 4) {
      uint32_t body_len = get_u32(host_read_buf_, 0);
      if (host_read_buf_.size() < 4u + body_len) break;
      size_t off = 4;
      uint32_t rid = get_u32(host_read_buf_, off);
      off += 4;
      uint16_t topic_len = get_u16(host_read_buf_, off);
      off += 2 + topic_len;  // reply doesn't need its own topic echoed back
      uint32_t payload_len = get_u32(host_read_buf_, off);
      off += 4;
      std::string payload = host_read_buf_.substr(off, payload_len);
      host_read_buf_.erase(0, 4 + body_len);
      auto it = std::find(host_wait_ids_.begin(), host_wait_ids_.end(), rid);
      if (it != host_wait_ids_.end()) host_wait_ids_.erase(it);
      Completion rc;
      rc.id = rid;
      complete_ok(&rc, std::move(payload));
      completions_.push_back(std::move(rc));
    }
    if (!host_wait_ids_.empty()) {
      park(kHostReadId, Op::kHostRead, host_handle_, host_fd_, POLLIN, 0, "", "");
    }
  }
#endif  // WASIGOCVM_HOST_BRIDGE

  void do_bind_probe(const std::string& network, const std::string& address, Completion* c) {
    int fd = -1;
    std::string err;
    if (!bind_fd(network, address, /*listen=*/false, &fd, &err)) {
      complete_err(c, err);
      return;
    }
    sockaddr_in bound{};
    socklen_t blen = sizeof(bound);
    getsockname(fd, reinterpret_cast<sockaddr*>(&bound), &blen);
    close(fd);
    complete_ok(c, "ok bound=" + fmt_v4(bound));
  }

  bool bind_fd(const std::string& network, const std::string& address, bool do_listen, int* fd_out,
               std::string* err) {
    std::string host, port;
    if (!split_hostport(address, &host, &port)) {
      *err = "error: missing port in address " + address;
      return false;
    }
    sockaddr_in addr{};
    if (!fill_v4(host, port, /*bind=*/true, &addr)) {
      *err = "error: resolve " + address;
      return false;
    }
    int fd = socket(AF_INET, socktype(network), 0);
    if (fd < 0) {
      *err = syserr("socket");
      return false;
    }
    set_nonblock(fd);
    int yes = 1;
    setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof(yes));
    if (bind(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
      *err = syserr("bind");
      close(fd);
      return false;
    }
    if (do_listen && !is_udp(network)) {
      if (listen(fd, SOMAXCONN) < 0) {
        *err = syserr("listen");
        close(fd);
        return false;
      }
    }
    *fd_out = fd;
    return true;
  }

  void do_listen(const std::string& network, const std::string& address, Completion* c) {
    int fd = -1;
    std::string err;
    if (!bind_fd(network, address, /*listen=*/true, &fd, &err)) {
      complete_err(c, err);
      return;
    }
    sockaddr_in bound{};
    socklen_t blen = sizeof(bound);
    getsockname(fd, reinterpret_cast<sockaddr*>(&bound), &blen);
    uint64_t h = alloc(fd, is_udp(network), !is_udp(network));
    complete_ok(c, "ok handle=" + std::to_string(h) + " bound=" + fmt_v4(bound));
  }

  void do_dial(uint64_t id, const std::string& network, const std::string& address, Completion* c,
               bool* parked) {
    std::string host, port;
    if (!split_hostport(address, &host, &port)) {
      complete_err(c, "error: missing port in address " + address);
      return;
    }
    sockaddr_in addr{};
    if (!fill_v4(host, port, /*bind=*/false, &addr)) {
      complete_err(c, "error: resolve " + address);
      return;
    }
    int fd = socket(AF_INET, socktype(network), 0);
    if (fd < 0) {
      complete_err(c, syserr("socket"));
      return;
    }
    set_nonblock(fd);
    int rc = connect(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr));
    if (rc == 0 || (is_udp(network) && rc < 0 && would_block())) {
      // UDP connect is just an association; EINPROGRESS still means "set".
      finish_connect(c, fd, is_udp(network), addr);
      return;
    }
    if (rc < 0 && errno == EINPROGRESS) {
      uint64_t h = alloc(fd, is_udp(network), false);
      park(id, Op::kConnect, h, fd, POLLOUT, 0, "", fmt_v4(addr));
      *parked = true;
      return;
    }
    complete_err(c, syserr("connect"));
    close(fd);
  }

  void drop_fd(int fd) {
    for (auto it = socks_.begin(); it != socks_.end();) {
      if (it->second.fd == fd) it = socks_.erase(it);
      else ++it;
    }
  }

  void finish_connect(Completion* c, int fd, bool udp, const sockaddr_in& remote) {
    int soerr = 0;
    socklen_t elen = sizeof(soerr);
    getsockopt(fd, SOL_SOCKET, SO_ERROR, &soerr, &elen);
    if (soerr != 0) {
      errno = soerr;
      complete_err(c, syserr("connect"));
      drop_fd(fd);
      close(fd);
      return;
    }
    sockaddr_in local{};
    socklen_t llen = sizeof(local);
    getsockname(fd, reinterpret_cast<sockaddr*>(&local), &llen);
    uint64_t h = 0;
    for (auto& kv : socks_) {
      if (kv.second.fd == fd) {
        h = kv.first;
        break;
      }
    }
    if (h == 0) h = alloc(fd, udp, false);
    complete_ok(c, "ok handle=" + std::to_string(h) + " local=" + fmt_v4(local) +
                       " remote=" + fmt_v4(remote));
  }

  void do_accept(uint64_t id, const std::string& handle, Completion* c, bool* parked) {
    uint64_t h = 0;
    if (!parse_handle(handle, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s || s->fd < 0) {
      complete_err(c, "error: unknown handle " + handle);
      return;
    }
    sockaddr_in remote{};
    socklen_t rlen = sizeof(remote);
    int nfd = accept(s->fd, reinterpret_cast<sockaddr*>(&remote), &rlen);
    if (nfd >= 0) {
      set_nonblock(nfd);
      uint64_t nh = alloc(nfd, false, false);
      complete_ok(c, "ok handle=" + std::to_string(nh) + " remote=" + fmt_v4(remote));
      return;
    }
    if (would_block()) {
      park(id, Op::kAccept, h, s->fd, POLLIN, 0, "", "");
      *parked = true;
      return;
    }
    complete_err(c, syserr("accept"));
  }

  void do_read(uint64_t id, const std::string& payload, Completion* c, bool* parked) {
    std::string hs, maxs;
    split_first(payload, '\x1f', &hs, &maxs);
    uint64_t h = 0;
    if (!parse_handle(hs, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s || s->fd < 0) {
      complete_err(c, "error: unknown handle " + hs);
      return;
    }
    int maxlen = std::atoi(maxs.c_str());
    if (maxlen < 0) maxlen = 0;
    if (maxlen > 65536) maxlen = 65536;
    std::string buf;
    buf.resize(static_cast<size_t>(maxlen));
    ssize_t n = recv(s->fd, buf.data(), buf.size(), 0);
    if (n >= 0) {
      buf.resize(static_cast<size_t>(n));
      complete_ok(c, std::move(buf));
      return;
    }
    if (would_block()) {
      park(id, Op::kRead, h, s->fd, POLLIN, maxlen, "", "");
      *parked = true;
      return;
    }
    complete_err(c, syserr("read"));
  }

  void do_write(uint64_t id, const std::string& payload, Completion* c, bool* parked) {
    std::string hs, data;
    split_first(payload, '\x1f', &hs, &data);
    uint64_t h = 0;
    if (!parse_handle(hs, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s || s->fd < 0) {
      complete_err(c, "error: unknown handle " + hs);
      return;
    }
    if (!write_all(id, h, s->fd, data, c, parked)) return;
  }

  bool write_all(uint64_t id, uint64_t h, int fd, const std::string& data, Completion* c,
                 bool* parked) {
    size_t off = 0;
    while (off < data.size()) {
      ssize_t n = send(fd, data.data() + off, data.size() - off, 0);
      if (n > 0) {
        off += static_cast<size_t>(n);
        continue;
      }
      if (n < 0 && would_block()) {
        park(id, Op::kWrite, h, fd, POLLOUT, 0, data.substr(off), "");
        *parked = true;
        return false;
      }
      complete_err(c, syserr("write"));
      return false;
    }
    complete_ok(c, std::to_string(data.size()));
    return true;
  }

  void do_readfrom(uint64_t id, const std::string& payload, Completion* c, bool* parked) {
    std::string hs, maxs;
    split_first(payload, '\x1f', &hs, &maxs);
    uint64_t h = 0;
    if (!parse_handle(hs, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s || s->fd < 0) {
      complete_err(c, "error: unknown handle " + hs);
      return;
    }
    int maxlen = std::atoi(maxs.c_str());
    if (maxlen < 0) maxlen = 0;
    if (maxlen > 65536) maxlen = 65536;
    std::string buf;
    buf.resize(static_cast<size_t>(maxlen));
    sockaddr_in from{};
    socklen_t flen = sizeof(from);
    ssize_t n = recvfrom(s->fd, buf.data(), buf.size(), 0, reinterpret_cast<sockaddr*>(&from),
                           &flen);
    if (n >= 0) {
      buf.resize(static_cast<size_t>(n));
      complete_ok(c, fmt_v4(from) + "\x1f" + buf);
      return;
    }
    if (would_block()) {
      park(id, Op::kReadFrom, h, s->fd, POLLIN, maxlen, "", "");
      *parked = true;
      return;
    }
    complete_err(c, syserr("readfrom"));
  }

  void do_writeto(uint64_t id, const std::string& payload, Completion* c, bool* parked) {
    std::string hs, rest;
    split_first(payload, '\x1f', &hs, &rest);
    std::string addr, data;
    split_first(rest, '\x1f', &addr, &data);
    uint64_t h = 0;
    if (!parse_handle(hs, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s || s->fd < 0) {
      complete_err(c, "error: unknown handle " + hs);
      return;
    }
    // Connected UDP (net.DialPacket): send() — sendto on a connected
    // wasi socket is flaky under some hosts.
    if (!s->listen && s->udp) {
      ssize_t n = send(s->fd, data.data(), data.size(), 0);
      if (n >= 0) {
        complete_ok(c, std::to_string(n));
        return;
      }
      if (would_block()) {
        park(id, Op::kWriteTo, h, s->fd, POLLOUT, 0, data, addr);
        *parked = true;
        return;
      }
      complete_err(c, syserr("writeto"));
      return;
    }
    std::string host, port;
    if (!split_hostport(addr, &host, &port)) {
      complete_err(c, "error: missing port in address " + addr);
      return;
    }
    sockaddr_in to{};
    if (!fill_v4(host, port, /*bind=*/false, &to)) {
      complete_err(c, "error: resolve " + addr);
      return;
    }
    ssize_t n = sendto(s->fd, data.data(), data.size(), 0, reinterpret_cast<sockaddr*>(&to),
                         sizeof(to));
    if (n >= 0) {
      complete_ok(c, std::to_string(n));
      return;
    }
    if (would_block()) {
      park(id, Op::kWriteTo, h, s->fd, POLLOUT, 0, data, addr);
      *parked = true;
      return;
    }
    complete_err(c, syserr("writeto"));
  }

  void do_close(const std::string& handle, Completion* c) {
    uint64_t h = 0;
    if (!parse_handle(handle, &h)) {
      complete_err(c, "error: bad handle");
      return;
    }
    Sock* s = lookup(h);
    if (!s) {
      complete_err(c, "error: unknown handle " + handle);
      return;
    }
    int fd = s->fd;
    cancel_fd(fd, "closed");
    if (fd >= 0) close(fd);
    socks_.erase(h);
    complete_ok(c, "ok");
  }

  void resume(Parked& p) {
    Completion c;
    c.id = p.id;
    bool parked = false;
    Sock* s = lookup(p.handle);
    if (!s || s->fd < 0) {
      complete_err(&c, "error: unknown handle");
      completions_.push_back(std::move(c));
      return;
    }
    switch (p.op) {
      case Op::kConnect: {
        sockaddr_in remote{};
        std::string host, port;
        if (split_hostport(p.to_addr, &host, &port)) fill_v4(host, port, false, &remote);
        finish_connect(&c, p.fd, s->udp, remote);
        break;
      }
      case Op::kAccept:
        do_accept(p.id, std::to_string(p.handle), &c, &parked);
        break;
      case Op::kRead:
        do_read(p.id, std::to_string(p.handle) + "\x1f" + std::to_string(p.maxlen), &c, &parked);
        break;
      case Op::kWrite:
        write_all(p.id, p.handle, p.fd, p.remaining, &c, &parked);
        break;
      case Op::kReadFrom:
        do_readfrom(p.id, std::to_string(p.handle) + "\x1f" + std::to_string(p.maxlen), &c,
                    &parked);
        break;
      case Op::kWriteTo: {
        std::string payload = std::to_string(p.handle) + "\x1f" + p.to_addr + "\x1f" + p.remaining;
        do_writeto(p.id, payload, &c, &parked);
        break;
      }
#if defined(WASIGOCVM_HOST_BRIDGE)
      case Op::kHostRead:
        host_pump_read();
        parked = true;  // host_pump_read() pushes its own completions directly
        break;
#endif
    }
    if (!parked) completions_.push_back(std::move(c));
  }

  void pump(int timeout_ms) {
    if (pending_.empty()) return;
    std::vector<pollfd> fds;
    std::vector<uint64_t> ids;
    fds.reserve(pending_.size());
    ids.reserve(pending_.size());
    for (auto& kv : pending_) {
      pollfd p{};
      p.fd = kv.second.fd;
      p.events = kv.second.events;
      fds.push_back(p);
      ids.push_back(kv.first);
    }
    int n = poll(fds.data(), fds.size(), timeout_ms);
    if (n <= 0) return;
    std::vector<uint64_t> ready;
    for (size_t i = 0; i < fds.size(); ++i) {
      if (fds[i].revents != 0) ready.push_back(ids[i]);
    }
    for (uint64_t id : ready) {
      auto it = pending_.find(id);
      if (it == pending_.end()) continue;
      Parked p = std::move(it->second);
      pending_.erase(it);
      resume(p);
    }
  }
};

inline void install_wasigocvm_net_bridge() {
  static WasigocvmNetBridge bridge;
  RegisterAsyncHostBridge(&bridge);
}

}  // namespace gocvm
