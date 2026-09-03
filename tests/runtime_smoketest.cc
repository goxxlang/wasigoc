// Native smoke test of the Rosetta runtime: goroutines, channels, defer,
// slices-with-cap, Error-vs-nil, panic/recover. This is the mapping wasigoc
// emits into; if this fails, generated WASM will fail too.
#include "runtime.hpp"

#include <cassert>
#include <string>

using namespace wasigo;

static Task pingpong() {
  auto ch = make_chan<int64_t>(0);
  // GoAsyncLit, not an immediately-invoked lambda: a coroutine lambda's
  // frame stores only a pointer back to its own closure ("this"); a
  // temporary closure invoked-and-discarded inline is destroyed at the
  // end of this full expression, but the produced Task is only
  // initially suspended -- the scheduler resumes it later, after the
  // closure (and this capture-by-reference's referent) is already gone.
  // See the GoAsyncLit doc comment in runtime.hpp -- same bug class the
  // compiler's own EmitGo had for `go func(){...}()`.
  go(GoAsyncLit([&]() -> Task {
    co_await ch.send(7);
    co_return;
  }));
  int64_t v = co_await ch.recv();
  assert(v == 7);
  co_return;
}

static Task select_default() {
  auto ch = make_chan<int64_t>(0);
  int64_t v = 0;
  int idx = co_await GSelect{}.recv(ch, &v).deflt();
  assert(idx == 1);
  co_return;
}

static Task buffered() {
  auto ch = make_chan<int64_t>(1);
  co_await ch.send(3);
  auto [v, ok] = co_await ch.recv_ok();
  assert(ok && v == 3);
  close(ch);
  auto [z, ok2] = co_await ch.recv_ok();
  assert(!ok2 && z == 0);
  co_return;
}

static TaskT<int64_t> take(Chan<int64_t> ch) {
  int64_t v = co_await ch.recv();
  co_return v * 2;
}

static Task returning_task() {
  auto ch = make_chan<int64_t>(0);
  go(GoAsyncLit([&]() -> Task {
    co_await ch.send(21);
    co_return;
  }));
  int64_t x = co_await take(ch);
  assert(x == 42);
  co_return;
}

// Returns the defer-run order rather than asserting on it directly: pf
// and defers are local to the nested block below, so their destructors
// (which is what actually runs the deferred closures, exactly like
// compiler-emitted `goto __wasigo_end` + fall-through does) fire at the
// closing brace -- *after* any statement placed at this function's own
// top level would already have executed. Checking `order` in the same
// place the original single-scope version did was checking it before
// the defers had run at all.
static std::string boom_local() {
  std::string order;
  {
    PanicFrame pf;
    DeferList defers;
    defers.push([&] { order += "1"; });
    defers.push([&] { order += "0"; });
    defers.push([&] {
      auto r = recover();
      assert(r.ok);
      order += "R";
    });
    pf.has_pending = true;
    pf.pending = "x";
    goto __wasigo_end;
  __wasigo_end:;
  }
  return order;
}

// -- gocvm::Call's ErrorState state machine (kClear/kBridgeActive/kPanic) --
// No compiled Go++ source exercises the "no bridge"/"bridge panic"/
// "reentrant" branches (compile.bat links no bridge at all, and the real
// shim_sandbox bridge never panics in practice), so this drives
// gocvm::Call and its HostBridge/AbacHook contract directly.

struct OkBridge : gocvm::HostBridge {
  bool Call(const std::string&, const std::string& payload,
            std::string* reply_out, std::string*) override {
    *reply_out = payload + "!";
    return true;
  }
};

struct FailBridge : gocvm::HostBridge {
  bool Call(const std::string&, const std::string&, std::string*,
            std::string* err_out) override {
    *err_out = "connect refused";
    return false;
  }
};

// Simulates a runtime panic surfacing from generated Go++ code the bridge
// calls into (bounds check, nil deref, ...): a PanicFrame with a pending
// panic, unwound via its destructor exactly like compiler-emitted
// `goto __wasigo_end` does. Per the ErrorState contract documented on
// HostBridge::Call, the bridge must check bridge_panicked() and return
// false rather than continuing.
struct PanicBridge : gocvm::HostBridge {
  bool Call(const std::string&, const std::string&, std::string* reply_out,
            std::string*) override {
    {
      PanicFrame pf;
      pf.has_pending = true;
      pf.pending = "boom from bridge";
    }
    if (g_error_state().bridge_panicked()) return false;
    *reply_out = "should not reach";
    return true;
  }
};

// A bridge that itself makes a nested gocvm::Call — not something any real
// bridge does today, but enter_bridge()'s reentrancy guard exists
// specifically to diagnose this instead of corrupting g_error_state, so
// it needs its own coverage.
struct ReentrantBridge : gocvm::HostBridge {
  bool Call(const std::string&, const std::string&, std::string* reply_out,
            std::string*) override {
    auto inner = gocvm::Call("reentrant.topic", "x");
    assert(inner.r1 != nullptr);
    *reply_out = inner.r1.str();
    return true;
  }
};

struct DenyAbac : gocvm::AbacHook {
  bool Check(const std::string& topic) override { return topic != "denied.topic"; }
};

static void gocvm_error_state() {
  // kClear, no bridge registered at all: distinguishable from a real
  // bridge's own failure by err_out never being set.
  {
    auto r = gocvm::Call("some.topic", "x");
    assert(r.r0.empty());
    assert(r.r1 != nullptr);
    assert(r.r1.str().find("no host bridge registered") != std::string::npos);
    assert(g_error_state().is_clear());
  }

  // kClear -> kBridgeActive -> kClear (success path).
  OkBridge ok;
  gocvm::RegisterHostBridge(&ok);
  {
    auto r = gocvm::Call("echo", "hi");
    assert(r.r1 == nullptr);
    assert(r.r0 == "hi!");
    assert(g_error_state().is_clear());
  }

  // A real bridge failure (ok=false) is a normal error, not a panic.
  FailBridge fail;
  gocvm::RegisterHostBridge(&fail);
  {
    auto r = gocvm::Call("net.dial", "x");
    assert(r.r1 != nullptr);
    assert(r.r1.str().find("connect refused") != std::string::npos);
    assert(r.r1.str().find("bridge panic") == std::string::npos);
    assert(g_error_state().is_clear());
  }

  // kBridgeActive -> kPanic -> kClear: a panic inside the bridge must
  // surface as a wasigo::Error, never abort the process.
  PanicBridge panic_bridge;
  gocvm::RegisterHostBridge(&panic_bridge);
  {
    auto r = gocvm::Call("os.exec", "x");
    assert(r.r1 != nullptr);
    assert(r.r1.str().find("bridge panic: boom from bridge") != std::string::npos);
    assert(g_error_state().is_clear());  // consume_panic() reset it
  }
  // ErrorState must be usable again immediately after a stashed panic.
  gocvm::RegisterHostBridge(&ok);
  {
    auto r = gocvm::Call("echo", "still works");
    assert(r.r1 == nullptr);
    assert(r.r0 == "still works!");
  }

  // Reentrant gocvm::Call is diagnosed, not UB, and leaves ErrorState
  // clean for the next (non-reentrant) call.
  ReentrantBridge reentrant;
  gocvm::RegisterHostBridge(&reentrant);
  {
    auto r = gocvm::Call("outer.topic", "x");
    assert(r.r1 == nullptr);  // outer bridge itself returned true
    assert(r.r0.find("reentrant bridge call") != std::string::npos);
    assert(g_error_state().is_clear());
  }
  gocvm::RegisterHostBridge(&ok);
  {
    auto r = gocvm::Call("echo", "post-reentrant");
    assert(r.r1 == nullptr);
    assert(r.r0 == "post-reentrant!");
  }

  // ABAC deny short-circuits before the bridge is ever entered.
  DenyAbac deny;
  gocvm::RegisterAbacHook(&deny);
  {
    auto denied = gocvm::Call("denied.topic", "x");
    assert(denied.r1 != nullptr);
    assert(denied.r1.str().find("abac deny") != std::string::npos);
    auto allowed = gocvm::Call("allowed.topic", "y");
    assert(allowed.r1 == nullptr);
    assert(allowed.r0 == "y!");
  }
  gocvm::RegisterAbacHook(nullptr);
  gocvm::RegisterHostBridge(nullptr);
}

int main() {
  auto s = Slice<int64_t>{1, 2, 3};
  assert(len(s) == 3);
  assert(cap(s) == 3);
  auto t = s.slice(1, 3);
  assert(t[0] == 2);
  t[0] = 9;
  assert(s[1] == 9);
  s = append(s, 4);
  t[0] = 8;
  assert(s[1] == 9);

  Map<std::string, int64_t> nilm;
  assert(is_nil(nilm));
  auto m = make_map<std::string, int64_t>();
  assert(!is_nil(m));
  m["a"] = 1;
  auto [val, ok] = m.lookup("a");
  assert(ok && val == 1);

  Error n;
  assert(n == nullptr);
  auto e = errors_new("");
  assert(e != nullptr);
  auto e2 = errors_new("boom");
  assert(e2.str() == "boom");

  assert(boom_local() == "R01");

  assert(type_key_of<int64_t>() == type_key_of<int64_t>());
  assert(type_key_of<int64_t>() != type_key_of<std::string>());

  auto d0 = decode_rune("Go", 0);
  assert(d0.r == 'G' && d0.size == 1);
  auto d1 = decode_rune("Go", 1);
  assert(d1.r == 'o' && d1.size == 1);
  auto b = bytes_from_string("hi");
  assert(len(b) == 2 && b[0] == 'h');
  assert(string_from_bytes(b) == "hi");

  Any boxed = Any::adapt<int64_t>(7);
  assert(!is_nil(boxed));
  assert(boxed.must_cast<int64_t>() == 7);
  auto [av, aok] = boxed.try_cast<std::string>();
  assert(!aok);

  assert(gmin(3LL, 1LL, 2LL) == 1);
  assert(gmax(3LL, 1LL, 2LL) == 3);
  auto zs = Slice<int64_t>{1, 2, 3};
  gclear(zs);
  assert(zs[0] == 0 && len(zs) == 3);
  auto zm = make_map<std::string, int64_t>();
  zm["a"] = 1;
  gclear(zm);
  assert(len(zm) == 0);

  std::array<int64_t, 4> arr{9, 8, 7, 6};
  auto as = slice_array(arr, 1, 3);
  assert(len(as) == 2 && as[0] == 8 && as[1] == 7);

  Func<int64_t()> mv = [p = 5LL]() { return p; };
  assert(mv() == 5);
  auto boom1 = errors_new("boom");
  auto boom2 = errors_new("boom");
  assert(errors_is(boom1, boom2));
  assert(!errors_is(boom1, errors_new("other")));

  struct Node : gc::GarbageCollected<Node> {
    int64_t v = 0;
    gc::Member<Node> next;
    void Trace(gc::Visitor& vis) const { vis.Trace(next); }
  };
  auto* n1 = gc::heap().Make<Node>();
  n1->v = 1;
  {
    gc::Persistent<Node> root(n1);
    auto* n2 = gc::heap().Make<Node>();
    n2->v = 2;
    n1->next = n2;
    assert(gc::heap().live() == 2);
    gc::heap().Collect();
    assert(gc::heap().live() == 2);
    n1->next = nullptr;
    gc::heap().Collect();
    assert(gc::heap().live() == 1);
  }
  gc::heap().Collect();
  assert(gc::heap().live() == 0);

  run(pingpong());
  run(select_default());
  run(buffered());
  run(returning_task());

  gocvm_error_state();
  return 0;
}
