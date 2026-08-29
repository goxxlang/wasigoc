// Native smoke test of the Rosetta runtime: goroutines, channels, defer,
// slices-with-cap, Error-vs-nil, panic/recover. This is the mapping wasigoc
// emits into; if this fails, generated WASM will fail too.
#include "runtime.hpp"

#include <cassert>
#include <string>

using namespace wasigo;

static Task pingpong() {
  auto ch = make_chan<int64_t>(0);
  go([&]() -> Task {
    co_await ch.send(7);
    co_return;
  }());
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
  go([&]() -> Task {
    co_await ch.send(21);
    co_return;
  }());
  int64_t x = co_await take(ch);
  assert(x == 42);
  co_return;
}

static void boom_local() {
  std::string order;
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
  assert(order == "R01");
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

  boom_local();

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
  return 0;
}
