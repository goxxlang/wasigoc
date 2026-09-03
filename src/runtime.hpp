// wasigo runtime -- Rosetta from Go constructs that do not line up with C++
// into templates that play to C++ and wasm32-wasip1, rather than a Go runtime
// clone.
//
// wasm32-wasip1 is one thread and has no growable stacks. We therefore do not
// emulate Go's preemptive M:N scheduler or its racy memory model. GC is not a
// Go collector clone -- it is C++ Oilpan (precise Trace / Member / Persistent,
// stop-the-world; nothing to concurrent-mark with on this target). What we
// keep is the *shape* of the source (goroutines, channels, defer, slices)
// mapped onto C++ strengths: RAII, templates, stackless coroutines, and a
// cooperative runqueue that is data-race-free by construction.
//
//    Go                         idiomatic WASM C++
//    -------------------------  ---------------------------------------------
//    goroutine                  wasigo::Task / TaskT<T>  (C++20 coroutine) + go()
//    chan T                     wasigo::Chan<T>  (shared handle, await send/recv)
//    select                     wasigo::GSelect  (index of the ready case)
//    defer f()                  RAII wasigo::defer([&]{ f(); })  (dtor LIFO)
//    panic / recover            PanicFrame + goto epilogue (noeh / no-setjmp WASM)
//    error                      wasigo::Error (nil is empty optional, not "")
//    []T, cap, slicing          wasigo::Slice<T>  (shared backing, bounds-checked)
//    map[K]V                    wasigo::Map<K,V>  (nil vs empty; iteration unordered)
//    interface / interface{}    generated vtable + adapt<T>; any is wasigo::Any; recover() is Recovered
//    func literals / method val wasigo::Func (virtual erasure; no <functional>) / C++ lambda ([=] across go)
//    generics                   C++ templates (strictly stronger)
//    iota                       constexpr ints, evaluated at transpile time
//    embedding                  public C++ inheritance (promotion falls out)
//    value receiver             const method + copy of *this (Go mutation isolation)
//    make / new / close / copy  make_slice/make_map/make_chan, New, close, copy
//    min / max / clear          gmin / gmax / gclear (slice zeros in place; map drops entries)
//    GC                         wasigo::gc Oilpan-lite (Trace / Member / Persistent, STW)
//
// Security (why these templates, not "just use vector and pthread"):
//   - Slice/Map/Chan index and nil ops panic instead of UB.
//   - go() captures by value so a spawned task cannot dangle on the spawner's
//     stack (WASM has no escape analysis).
//   - Cooperative scheduling: no data races, no atomics, no wasi-threads.
//   - user panic is a goto to the function epilogue (noeh has no throw/setjmp);
//     defer still runs via DeferList; runtime panics abort outside a bridge.
//   - ErrorState state machine: the single SFI error boundary at gocvm::Call.
//     Inside a bridge dispatch, panic() stashes instead of aborting — no C++
//     exceptions, no setjmp, works identically on noeh and native builds.
//   - send/recv on a nil channel panics rather than blocking forever -- a
//     forever-block is a Go footgun with no good C++ spelling on a target
//     that cannot be preempted.
#pragma once

// When this header is included by the native smoketest (not inlined by
// wasigoc), enable the full mapping. Generated TUs define WASIGO_GENERATED
// and only the WASIGO_NEED_* flags they actually use -- wasm32-wasip1's
// noeh libc++ has no setjmp and <algorithm> pulls a broken <cctype>.
#if !defined(WASIGO_GENERATED)
#ifndef WASIGO_NEED_CORO
#define WASIGO_NEED_CORO 1
#endif
#endif

// iostream must come before the C stdio headers: wasi-sdk's noeh libc++
// blows up if <cstdio>/<cstdlib> have already defined ctype bits.
#include <iostream>
#include <algorithm>
#include <array>
#include <chrono>
#include <cstdint>
#include <cstddef>
#include <cstdio>
#include <cstdlib>
#include <dirent.h>
#include <sys/stat.h>
#include <initializer_list>
#include <memory>
#include <optional>
#include <sstream>
#include <string>
#include <type_traits>
#include <unordered_map>
#include <utility>
#include <vector>
#ifdef WASIGO_NEED_CORO
#include <coroutine>
#include <deque>
#endif

namespace wasigo {

// Go complex64 / complex128. std::complex is avoided: wasi-sdk's noeh
// libc++ has pulled exception-shaped pieces from <complex> before.
struct Complex64 {
  float re = 0;
  float im = 0;
};
struct Complex128 {
  double re = 0;
  double im = 0;
};

inline Complex64 operator+(Complex64 a, Complex64 b) { return {a.re + b.re, a.im + b.im}; }
inline Complex64 operator-(Complex64 a, Complex64 b) { return {a.re - b.re, a.im - b.im}; }
inline Complex64 operator-(Complex64 z) { return {-z.re, -z.im}; }
inline Complex64 operator*(Complex64 a, Complex64 b) {
  return {a.re * b.re - a.im * b.im, a.re * b.im + a.im * b.re};
}
inline Complex64 operator/(Complex64 a, Complex64 b) {
  float d = b.re * b.re + b.im * b.im;
  return {(a.re * b.re + a.im * b.im) / d, (a.im * b.re - a.re * b.im) / d};
}
inline bool operator==(Complex64 a, Complex64 b) { return a.re == b.re && a.im == b.im; }
inline bool operator!=(Complex64 a, Complex64 b) { return !(a == b); }

inline Complex128 operator+(Complex128 a, Complex128 b) { return {a.re + b.re, a.im + b.im}; }
inline Complex128 operator-(Complex128 a, Complex128 b) { return {a.re - b.re, a.im - b.im}; }
inline Complex128 operator-(Complex128 z) { return {-z.re, -z.im}; }
inline Complex128 operator*(Complex128 a, Complex128 b) {
  return {a.re * b.re - a.im * b.im, a.re * b.im + a.im * b.re};
}
inline Complex128 operator/(Complex128 a, Complex128 b) {
  double d = b.re * b.re + b.im * b.im;
  return {(a.re * b.re + a.im * b.im) / d, (a.im * b.re - a.re * b.im) / d};
}
inline bool operator==(Complex128 a, Complex128 b) { return a.re == b.re && a.im == b.im; }
inline bool operator!=(Complex128 a, Complex128 b) { return !(a == b); }

template <class T>
inline Complex128 as_complex128(T x) {
  if constexpr (std::is_same_v<T, Complex128>) return x;
  else if constexpr (std::is_same_v<T, Complex64>)
    return {static_cast<double>(x.re), static_cast<double>(x.im)};
  else
    return {static_cast<double>(x), 0};
}
template <class T>
inline Complex64 as_complex64(T x) {
  if constexpr (std::is_same_v<T, Complex64>) return x;
  else if constexpr (std::is_same_v<T, Complex128>)
    return {static_cast<float>(x.re), static_cast<float>(x.im)};
  else
    return {static_cast<float>(x), 0};
}
inline float creal(Complex64 z) { return z.re; }
inline double creal(Complex128 z) { return z.re; }
inline float cimag(Complex64 z) { return z.im; }
inline double cimag(Complex128 z) { return z.im; }

inline std::ostream& operator<<(std::ostream& o, Complex64 z) {
  o << '(' << z.re << (z.im < 0 ? "" : "+") << z.im << "i)";
  return o;
}
inline std::ostream& operator<<(std::ostream& o, Complex128 z) {
  o << '(' << z.re << (z.im < 0 ? "" : "+") << z.im << "i)";
  return o;
}

// ---- ErrorState (SFI error state machine) -----------------------------------
// ALL error handling in the wasigo runtime — panic, recover, and gocvm
// bridge faults — flows through a single thread-local state machine.
// No C++ exceptions, no setjmp/longjmp.  Works identically on native
// and wasm32-wasip1/noeh builds.
//
// States:
//   kClear        — normal execution.  panic() aborts (runtime fault).
//   kBridgeActive — inside a gocvm::Call dispatch.  panic() transitions
//                   to kPanic instead of aborting, so the bridge call
//                   returns an error to generated Go++ code.
//   kPanic        — a panic fired inside a bridge call.  The bridge
//                   must check and return early; gocvm::Call surfaces
//                   the stashed message as wasigo::Error.
//   kRecovered    — a user recover() consumed the pending panic.
//
// The state machine is the single SFI error boundary: generated Go++
// code never uses try/catch/throw, and gocvm::Call is the only place
// where a runtime fault can be caught and turned into a value.
//
// User-level panic in a function that has defer is still the
// compiler-emitted `__pf.has_pending = true; goto __wasigo_end;`
// pattern — PanicFrame is unchanged for that path.  ErrorState only
// activates at the gocvm boundary.

enum class ErrorStateKind {
  kClear,
  kBridgeActive,
  kPanic,
  kRecovered,
};

struct ErrorState {
  ErrorStateKind kind = ErrorStateKind::kClear;

  // Panic message stash.  Pre-reserved to kStashReserve bytes so that an
  // OOM-triggered panic can land without a secondary allocation.  The
  // reserve is established once on first enter_bridge() (not at static
  // init, since thread_local constructors run before main on some
  // toolchains and wasm runtimes).
  static constexpr std::size_t kStashReserve = 256;
  std::string panic_msg;

  bool is_clear()        const { return kind == ErrorStateKind::kClear; }
  bool is_bridge()       const { return kind == ErrorStateKind::kBridgeActive; }
  bool is_panic()        const { return kind == ErrorStateKind::kPanic; }
  bool is_recovered()    const { return kind == ErrorStateKind::kRecovered; }

  // Enter bridge dispatch.  Returns false if already inside a bridge
  // (reentrant gocvm.Call — not expected, but diagnosed rather than UB).
  //
  // Reentrancy note: shim_sandbox bridge calls are strictly synchronous
  // (run-to-completion, no co_await, no callback into Go++ user code),
  // so a flat flag is sufficient.  If a future bridge needs to call back
  // into generated code, this must become a depth-counted stack — but
  // that would also require re-entering the cooperative runqueue, which
  // is a much larger design change.  For now, reentrant calls are
  // diagnosed and rejected.
  bool enter_bridge() {
    if (kind != ErrorStateKind::kClear) return false;
    kind = ErrorStateKind::kBridgeActive;
    if (panic_msg.capacity() < kStashReserve) {
      panic_msg.reserve(kStashReserve);
    }
    panic_msg.clear();
    return true;
  }

  // Called by panic() when kBridgeActive: stash message, transition to
  // kPanic.  The bridge implementation must check bridge_panicked() and
  // return early.
  //
  // Allocation safety: panic_msg was pre-reserved in enter_bridge().
  // If msg is longer than the reserve, the std::string move is a pointer
  // swap (no allocation).  If msg is short, it fits in the reserved
  // buffer.  A truly catastrophic OOM that prevents even a move is not
  // recoverable on any target.
  void bridge_panic(std::string msg) {
    kind = ErrorStateKind::kPanic;
    panic_msg = std::move(msg);
  }

  // Check whether a panic is pending (for bridge code to early-return).
  bool bridge_panicked() const { return kind == ErrorStateKind::kPanic; }

  // Consume the pending panic as a wasigo::Error and reset to kClear.
  // Called by gocvm::Call after the bridge returns.
  std::string consume_panic() {
    std::string msg = std::move(panic_msg);
    kind = ErrorStateKind::kClear;
    return msg;
  }

  // Leave bridge dispatch (success path).  Resets to kClear.
  void leave_bridge() {
    kind = ErrorStateKind::kClear;
    panic_msg.clear();
  }
};

// A plain `inline thread_local ErrorState g_error_state;` (C++17 inline
// variable) triggers "multiple definition of TLS init function for
// wasigo::g_error_state" at link time on this toolchain (WinLibs mingw
// GCC 16.1.0/binutils): the compiler-generated TLS guard/init function
// for an inline thread_local *variable* isn't COMDAT-folded across
// object files on this PE/COFF target, so every TU that includes this
// header (each of shim_sandbox's libw2g.a members, plus its consuming
// executable) links its own copy and collides. A thread_local *function-
// local static* behind an ordinary inline function uses a different,
// long-standardized guard mechanism that this same toolchain has always
// folded correctly (the pattern every portable Meyer's-singleton relies
// on) -- so every access below goes through g_error_state() rather than
// a bare variable.
inline ErrorState& g_error_state() {
  thread_local ErrorState state;
  return state;
}

// RAII guard for bridge scope.  Guarantees leave_bridge() or
// consume_panic() is called even if bridge code has an early return
// or an unhandled branch.  gocvm::Call uses this instead of bare
// enter_bridge()/leave_bridge() calls.
struct BridgeScope {
  bool entered = false;
  bool consumed = false;

  explicit BridgeScope(bool ok) : entered(ok) {}
  BridgeScope(const BridgeScope&) = delete;
  BridgeScope& operator=(const BridgeScope&) = delete;

  ~BridgeScope() {
    if (!entered || consumed) return;
    // If we get here, nobody consumed or left — either a bug or an
    // unhandled early return in gocvm::Call itself.  Reset to kClear
    // so subsequent calls aren't stuck in kBridgeActive.
    if (g_error_state().is_panic()) {
      // Panic was stashed but nobody consumed it — surface it to
      // stderr and reset, rather than silently swallowing.
      std::cerr << "gocvm: unchecked bridge panic: "
                << g_error_state().panic_msg << "\n";
      g_error_state().consume_panic();
    } else {
      g_error_state().leave_bridge();
    }
  }

  void mark_consumed() { consumed = true; }
};

// ---- panic / recover --------------------------------------------------------
// wasi-sdk's noeh libc++ has no C++ exceptions. Its setjmp.h refuses to
// compile without the wasm exception-handling proposal. User-level panic
// in a function that has defer is therefore a compiler-emitted
//   __pf.has_pending = true; goto __wasigo_end;
// so DeferList runs on the way out and recover() can read the frame.
//
// When ErrorState is kBridgeActive (inside a gocvm::Call), panic() does
// NOT abort — it stashes the message on g_error_state and returns.
// The bridge code must cooperate by checking g_error_state().bridge_panicked()
// and returning early (bool false); gocvm::Call then surfaces the stashed
// message as a wasigo::Error.  Outside a bridge call, panic() still aborts.

struct Recovered {
  bool ok = false;
  std::string msg;
};
inline bool is_nil(const Recovered& r) { return !r.ok; }
inline std::ostream& operator<<(std::ostream& os, const Recovered& r) { return os << r.msg; }

struct PanicFrame {
  bool has_pending = false;
  std::string pending;
  bool recovered = false;
  PanicFrame* prev = nullptr;
  PanicFrame();
  ~PanicFrame();
};

inline thread_local PanicFrame* g_panic_frame = nullptr;

inline PanicFrame::PanicFrame() : prev(g_panic_frame) { g_panic_frame = this; }

inline PanicFrame::~PanicFrame() {
  g_panic_frame = prev;
  if (has_pending && !recovered) {
    // If a bridge is active, promote to ErrorState rather than aborting.
    if (g_error_state().is_bridge()) {
      g_error_state().bridge_panic(std::move(pending));
      return;
    }
    std::cerr << "panic: " << pending << "\n";
    std::abort();
  }
}

// panic() — runtime faults (bounds, nil map, send on closed chan, etc.).
// Stays [[noreturn]] for generated code: outside a bridge, aborts.
// Inside a gocvm bridge call (kBridgeActive), stash the message on
// ErrorState.  The bridge must check g_error_state().bridge_panicked()
// and return early; gocvm::Call surfaces it as a wasigo::Error.
//
// panic_or_stash() is the non-noreturn entry point for code that may
// be called from inside a bridge and needs to handle the return.
// panic() wraps it with an unconditional abort fallthrough so that
// generated code (which is never inside a bridge) keeps [[noreturn]].
inline bool panic_or_stash(std::string m) {
  if (g_error_state().is_bridge()) {
    g_error_state().bridge_panic(std::move(m));
    return true;  // stashed — caller must return early
  }
  return false;  // not stashed — caller should abort
}
[[noreturn]] inline void panic(std::string m) {
  if (panic_or_stash(std::string(m))) {
    // Inside a bridge: panic_or_stash stashed the message.  We still
    // need to unwind to the bridge's Call() return.  Since noeh has no
    // exceptions and no longjmp, the bridge itself must cooperate by
    // checking bridge_panicked().  But panic() call sites in generated
    // code (the only ones that rely on [[noreturn]]) are never inside a
    // bridge, so this path is unreachable for them.  For bridge code
    // that calls panic(), use panic_or_stash() directly instead.
    //
    // If we do somehow reach here (bug), abort is still safe.
    std::abort();
  }
  std::cerr << "panic: " << m << "\n";
  std::abort();
}
[[noreturn]] inline void panic(const char* m) { panic(std::string(m ? m : "")); }
[[noreturn]] inline void panic(int64_t v) { panic(std::to_string(v)); }

inline Recovered recover() {
  if (!g_panic_frame || !g_panic_frame->has_pending) return {};
  g_panic_frame->recovered = true;
  Recovered r;
  r.ok = true;
  r.msg = std::move(g_panic_frame->pending);
  g_panic_frame->has_pending = false;
  return r;
}

template<class F>
class Defer {
 public:
  explicit Defer(F f) : f_(std::move(f)) {}
  Defer(const Defer&) = delete;
  Defer& operator=(const Defer&) = delete;
  Defer(Defer&& o) noexcept : f_(std::move(o.f_)), armed_(o.armed_) { o.armed_ = false; }
  ~Defer() {
    if (armed_) f_();
  }

 private:
  F f_;
  bool armed_ = true;
};

template<class F>
Defer<F> defer(F f) {
  return Defer<F>(std::move(f));
}

// Function-scoped defer list so panic can be captured *before* defers run
// (Go's order). RAII still owns the list; each `defer` is a push.
struct DeferList {
  struct Node {
    virtual ~Node() = default;
    virtual void run() = 0;
  };
  template<class F>
  struct Impl : Node {
    F f;
    explicit Impl(F fn) : f(std::move(fn)) {}
    void run() override { f(); }
  };
  std::vector<std::unique_ptr<Node>> fns;
  template<class F>
  void push(F f) {
    fns.push_back(std::make_unique<Impl<F>>(std::move(f)));
  }
  ~DeferList() {
    while (!fns.empty()) {
      auto n = std::move(fns.back());
      fns.pop_back();
      n->run();
    }
  }
};

// ---- error ------------------------------------------------------------------
// Not a string. errors.New("") is non-nil; only a default-constructed Error
// compares equal to nil.

class Error {
 public:
  Error() = default;
  explicit Error(std::string m) : msg_(std::move(m)) {}
  // fmt.Errorf's %w: wraps another Error so errors.Is/Unwrap can walk the
  // chain, the way Go's %w-wrapping does.
  Error(std::string m, Error wrapped)
      : msg_(std::move(m)), wrapped_(std::make_shared<Error>(std::move(wrapped))) {}
  bool is_nil() const { return !msg_.has_value(); }
  const std::string& str() const {
    static const std::string kEmpty;
    return msg_ ? *msg_ : kEmpty;
  }
  bool has_wrapped() const { return static_cast<bool>(wrapped_); }
  Error unwrap() const { return wrapped_ ? *wrapped_ : Error(); }

 private:
  std::optional<std::string> msg_;
  std::shared_ptr<Error> wrapped_;
};

// panic(err) where err is a Go `error` value -- real Go's panic takes
// `any`, and panicking with an error (not just a string) is an extremely
// common idiom (`if err != nil { panic(err) }`). Declared here, after
// Error's own definition, rather than alongside panic's other overloads
// above (which predate Error existing in this header).
[[noreturn]] inline void panic(Error e) { panic(e.str()); }

inline Error errors_new(std::string m) { return Error(std::move(m)); }
inline Error errors_new_wrap(std::string m, Error wrapped) {
  return Error(std::move(m), std::move(wrapped));
}
inline Error errors_unwrap(const Error& err) { return err.unwrap(); }

// errors.Join: concatenates messages ("\n"-joined, nil errors skipped); all
// nil (or an empty list) is nil. Doesn't chain for errors.Is the way real
// Go's multi-Unwrap does -- Is() on a joined error only matches by the
// joined string, not by walking the individual joined errors.
inline Error errors_join(std::initializer_list<Error> errs) {
  std::string msg;
  bool any = false;
  for (const Error& e : errs) {
    if (e.is_nil()) continue;
    if (any) msg += "\n";
    msg += e.str();
    any = true;
  }
  if (!any) return Error();
  return Error(std::move(msg));
}

inline bool errors_is(const Error& err, const Error& target) {
  Error cur = err;
  for (;;) {
    if (cur.is_nil() || target.is_nil()) return cur.is_nil() && target.is_nil();
    if (cur.str() == target.str()) return true;
    if (!cur.has_wrapped()) return false;
    cur = cur.unwrap();
  }
}

inline std::ostream& operator<<(std::ostream& os, const Error& e) {
  return os << e.str();
}
inline bool operator==(const Error& e, std::nullptr_t) { return e.is_nil(); }
inline bool operator!=(const Error& e, std::nullptr_t) { return !e.is_nil(); }
inline bool operator==(std::nullptr_t, const Error& e) { return e.is_nil(); }
inline bool operator!=(std::nullptr_t, const Error& e) { return !e.is_nil(); }
// `err == io.EOF` is at least as common Go idiom as errors.Is(err, io.EOF)
// (io.EOF/io.ErrClosedPipe-style sentinel checks are traditionally written
// with ==); same string-equal semantics as errors_is/errors.Is.
inline bool operator==(const Error& a, const Error& b) { return errors_is(a, b); }
inline bool operator!=(const Error& a, const Error& b) { return !errors_is(a, b); }

inline bool is_nil(std::nullptr_t) { return true; }
template<class T>
bool is_nil(T* p) {
  return p == nullptr;
}
inline bool is_nil(const Error& e) { return e.is_nil(); }

// Stable identity for a C++ type without RTTI (wasi-sdk noeh is built
// -fno-rtti). Generated interfaces stash this next to the vtable so
// x.(T) / x.(T) comma-ok can recover the boxed value.
template<class T>
const void* type_key_of() {
  static char k;
  return &k;
}

template<class T>
T iface_must_cast(const std::shared_ptr<void>& self, const void* key) {
  if constexpr (std::is_pointer_v<T>) {
    using U = std::remove_pointer_t<T>;
    if (!self || key != type_key_of<T>()) panic("interface conversion");
    return static_cast<U*>(self.get());
  } else {
    if (!self || key != type_key_of<T>()) panic("interface conversion");
    return *static_cast<T*>(self.get());
  }
}

template<class T>
std::pair<T, bool> iface_try_cast(const std::shared_ptr<void>& self, const void* key) {
  if constexpr (std::is_pointer_v<T>) {
    using U = std::remove_pointer_t<T>;
    if (!self || key != type_key_of<T>()) return {nullptr, false};
    return {static_cast<U*>(self.get()), true};
  } else {
    if (!self || key != type_key_of<T>()) return {T{}, false};
    return {*static_cast<T*>(self.get()), true};
  }
}

// No RTTI means Any can't ask "does my boxed T support operator<<" at
// runtime; ask at adapt<T>() compile time instead and stash a print
// function alongside the box (nullptr, i.e. "<any>", for a T that has none
// -- e.g. a func value or an unexported struct with no operator<<).
template <class T, class = void>
struct has_ostream_op : std::false_type {};
template <class T>
struct has_ostream_op<
    T, std::void_t<decltype(std::declval<std::ostream&>() << std::declval<const T&>())>>
    : std::true_type {};

// ---- reflect support ---------------------------------------------------
// Kind numbering matches the constants stdlib/reflect/reflect.go exposes
// as Go values (Invalid==0, Bool==1, ...) -- keep the two in sync by hand,
// there's no shared source of truth between this C++ enum and that Go
// file. Int64/Uint64/Uint8/Int32 double up for Go's same-width aliases
// (int and int64 both lower to C++ int64_t, byte and uint8 both lower to
// uint8_t, etc. -- see NamedCppType's kBuiltins table) since those pairs
// are literally the same C++ type here and can't be told apart without
// per-declaration metadata this compiler doesn't keep.
enum class RKind : int {
  Invalid = 0,
  Bool,
  Int8,
  Int16,
  Int32,
  Int64,
  Uint8,
  Uint16,
  Uint32,
  Uint64,
  Float32,
  Float64,
  String,
  Ptr,
  Struct,
  Slice,
  Complex64,
  Complex128,
};

struct FieldInfo;  // defined after Any -- see the forward-declaration note below

// A struct's per-field description, emitted by cpp_generator.cc right
// after the struct's own definition as a free function ADL-findable from
// Any::adapt<T> (`wasigo_reflect_describe(const T*, vector<FieldInfo>&)`):
// this is the only per-type reflection metadata the compiler generates --
// everything else in stdlib/reflect/reflect.go is built on top of it plus
// Any's existing type_key/kind/type_name.
template <class T, class = void>
struct has_reflect_describe : std::false_type {};
template <class T>
struct has_reflect_describe<
    T, std::void_t<decltype(wasigo_reflect_describe(std::declval<T*>(),
                                                      std::declval<std::vector<FieldInfo>&>()))>>
    : std::true_type {};

template <class T, class = void>
struct has_reflect_typename : std::false_type {};
template <class T>
struct has_reflect_typename<T, std::void_t<decltype(wasigo_reflect_typename(std::declval<const T*>()))>>
    : std::true_type {};

template <class T>
constexpr int kind_of() {
  if constexpr (std::is_same_v<T, bool>) return static_cast<int>(RKind::Bool);
  else if constexpr (std::is_same_v<T, int8_t>) return static_cast<int>(RKind::Int8);
  else if constexpr (std::is_same_v<T, int16_t>) return static_cast<int>(RKind::Int16);
  else if constexpr (std::is_same_v<T, int32_t>) return static_cast<int>(RKind::Int32);
  else if constexpr (std::is_same_v<T, int64_t>) return static_cast<int>(RKind::Int64);
  else if constexpr (std::is_same_v<T, uint8_t>) return static_cast<int>(RKind::Uint8);
  else if constexpr (std::is_same_v<T, uint16_t>) return static_cast<int>(RKind::Uint16);
  else if constexpr (std::is_same_v<T, uint32_t>) return static_cast<int>(RKind::Uint32);
  else if constexpr (std::is_same_v<T, uint64_t>) return static_cast<int>(RKind::Uint64);
  else if constexpr (std::is_same_v<T, float>) return static_cast<int>(RKind::Float32);
      else if constexpr (std::is_same_v<T, double>) return static_cast<int>(RKind::Float64);
  else if constexpr (std::is_same_v<T, Complex64>) return static_cast<int>(RKind::Complex64);
  else if constexpr (std::is_same_v<T, Complex128>) return static_cast<int>(RKind::Complex128);
  else if constexpr (std::is_same_v<T, std::string>) return static_cast<int>(RKind::String);
  else if constexpr (std::is_pointer_v<T>) return static_cast<int>(RKind::Ptr);
  else if constexpr (has_reflect_describe<T>::value) return static_cast<int>(RKind::Struct);
  // Slice/Map/Chan/Func/Interface kinds aren't classified (report
  // Invalid) -- reflecting *into* a slice/map's elements would need its
  // own len/index metadata this compiler doesn't generate yet; struct
  // field reflection (the motivating use case, arbitrary-struct JSON
  // marshaling) doesn't need it for the field's own Kind, only for a
  // slice/map-*typed* field's Kind, which callers should treat as a
  // known gap rather than a wrong answer.
  else return static_cast<int>(RKind::Invalid);
}

// Defined after Slice<T> — binds Len/Index when T is wasigo::Slice<U>.
template <class T>
void finish_any_kind(struct Any& a);

// interface{} / any: boxed value + type_key, no RTTI. recover() stays Recovered.
struct Any {
  std::shared_ptr<void> self;
  const void* type_key = nullptr;
  void (*print_fn)(std::ostream&, const void*) = nullptr;
  int kind = 0;
  const char* type_name = "";
  // Set only when the boxed T is a struct with a generated
  // wasigo_reflect_describe (see has_reflect_describe above) -- appends
  // this value's fields, by name, as their own Any-boxed values.
  void (*reflect_fields_fn)(const std::shared_ptr<void>&, std::vector<FieldInfo>&) = nullptr;
  int64_t (*slice_len_fn)(const std::shared_ptr<void>&) = nullptr;
  Any (*slice_index_fn)(const std::shared_ptr<void>&, int64_t) = nullptr;
  bool is_nil() const { return !self; }
  template<class T>
  static Any adapt(T v) {
    Any a;
    a.self = std::make_shared<T>(std::move(v));
    a.type_key = type_key_of<T>();
    a.kind = kind_of<T>();
    if constexpr (has_reflect_typename<T>::value) {
      a.type_name = wasigo_reflect_typename(static_cast<const T*>(nullptr));
    }
    if constexpr (has_reflect_describe<T>::value) {
      a.reflect_fields_fn = +[](const std::shared_ptr<void>& self, std::vector<FieldInfo>& out) {
        wasigo_reflect_describe(static_cast<T*>(self.get()), out);
      };
    }
    if constexpr (std::is_same_v<T, bool>) {
      // Go's fmt prints a bool as "true"/"false"; plain `os << bool` would
      // print C++'s default 1/0 (see EmitBuiltinFmtCall/EmitPrintf for the
      // same fix on a statically-bool arg -- this is the boxed-in-`any`
      // case those can't see).
      a.print_fn = +[](std::ostream& os, const void* p) {
        os << (*static_cast<const bool*>(p) ? "true" : "false");
      };
    } else     if constexpr (has_ostream_op<T>::value) {
      a.print_fn = +[](std::ostream& os, const void* p) { os << *static_cast<const T*>(p); };
    }
    finish_any_kind<T>(a);
    return a;
  }
  template<class T>
  static Any adapt_ptr(T* v) {
    if (!v) return {};
    Any a;
    a.self = std::shared_ptr<void>(static_cast<void*>(v), [](void*) {});
    a.type_key = type_key_of<T*>();
    // A *T boxed into `any` reflects as if it were T itself (Kind::Struct,
    // not Kind::Ptr -- real Go would need an extra Value.Elem() step this
    // compiler doesn't implement; skipping straight to the struct keeps
    // `json.Marshal(v)` and `json.Marshal(&v)` both usable through the
    // same reflect-based code path without it).
    a.kind = kind_of<T>();
    if constexpr (has_reflect_typename<T>::value) {
      a.type_name = wasigo_reflect_typename(static_cast<const T*>(nullptr));
    }
    if constexpr (has_reflect_describe<T>::value) {
      a.reflect_fields_fn = +[](const std::shared_ptr<void>& self, std::vector<FieldInfo>& out) {
        wasigo_reflect_describe(static_cast<T*>(self.get()), out);
      };
    }
    if constexpr (has_ostream_op<T>::value) {
      a.print_fn = +[](std::ostream& os, const void* p) { os << *static_cast<const T*>(p); };
    }
    finish_any_kind<T>(a);
    return a;
  }
  template<class T>
  T must_cast() const {
    return iface_must_cast<T>(self, type_key);
  }
  template<class T>
  std::pair<T, bool> try_cast() const {
    return iface_try_cast<T>(self, type_key);
  }

  // ---- reflect.Value / reflect.Type methods (both are `any` -- see
  // NamedCppType's `t->pkg == "reflect"` case in cpp_generator.cc).
  // NumField()/Field() need FieldInfo complete, so they're defined out of
  // line just below, after FieldInfo's real definition.
  int Kind() const { return kind; }
  std::string Name() const { return std::string(type_name); }
  Any Interface() const { return *this; }
  int64_t Int() const {
    if (!self) return 0;
    switch (static_cast<RKind>(kind)) {
      case RKind::Int8: return *static_cast<const int8_t*>(self.get());
      case RKind::Int16: return *static_cast<const int16_t*>(self.get());
      case RKind::Int32: return *static_cast<const int32_t*>(self.get());
      case RKind::Int64: return *static_cast<const int64_t*>(self.get());
      case RKind::Uint8: return *static_cast<const uint8_t*>(self.get());
      case RKind::Uint16: return *static_cast<const uint16_t*>(self.get());
      case RKind::Uint32: return *static_cast<const uint32_t*>(self.get());
      case RKind::Uint64: return static_cast<int64_t>(*static_cast<const uint64_t*>(self.get()));
      default: panic("reflect: Int called on a non-integer Value");
    }
    return 0;
  }
  double Float() const {
    if (self) {
      if (static_cast<RKind>(kind) == RKind::Float32) return *static_cast<const float*>(self.get());
      if (static_cast<RKind>(kind) == RKind::Float64) return *static_cast<const double*>(self.get());
    }
    panic("reflect: Float called on a non-float Value");
    return 0;
  }
  bool Bool() const {
    if (!self || static_cast<RKind>(kind) != RKind::Bool) panic("reflect: Bool called on a non-bool Value");
    return *static_cast<const bool*>(self.get());
  }
  // Matches real Go: never panics -- returns the actual value for
  // Kind::String, a placeholder like "<21 Value>" for anything else.
  std::string String() const {
    if (self && static_cast<RKind>(kind) == RKind::String) {
      return *static_cast<const std::string*>(self.get());
    }
    return "<" + std::to_string(kind) + " Value>";
  }
  Any Type() const { return *this; }
  int64_t NumField() const;
  Any Field(int64_t i) const;
  std::string FieldName(int64_t i) const;
  bool CanSet() const { return self != nullptr; }
  void SetInt(int64_t n);
  void SetUint(uint64_t n);
  void SetFloat(double n);
  void SetBool(bool b);
  void SetString(const std::string& s);
  int64_t Len() const;
  Any Index(int64_t i) const;
};
inline bool is_nil(const Any& a) { return a.is_nil(); }
inline std::ostream& operator<<(std::ostream& os, const Any& a) {
  if (a.is_nil()) return os << "<nil>";
  if (a.print_fn) {
    a.print_fn(os, a.self.get());
    return os;
  }
  return os << "<any>";
}

// Now that Any is complete, FieldInfo (forward-declared above so Any's
// reflect_fields_fn member could name it) gets its real definition.
struct FieldInfo {
  const char* name;
  Any value;
};

// ---- reflect ------------------------------------------------------------
// stdlib/reflect/reflect.go's Value and Type are both just `any` under
// the hood (see NamedCppType's `t->pkg == "reflect"` case) -- Kind/
// NumField/Field/Interface/Int/Float/String/Bool are real member
// functions on Any itself, so a Go method call on a Value/Type routes
// through the ordinary struct-method emission path with no special-casing
// beyond that type mapping (the same trick os.File uses).
inline std::vector<FieldInfo> reflect_fields(const Any& v) {
  std::vector<FieldInfo> fields;
  if (v.reflect_fields_fn && v.self) v.reflect_fields_fn(v.self, fields);
  return fields;
}

inline int64_t Any::NumField() const {
  return static_cast<int64_t>(reflect_fields(*this).size());
}
inline Any Any::Field(int64_t i) const {
  auto fields = reflect_fields(*this);
  if (i < 0 || static_cast<size_t>(i) >= fields.size()) panic("reflect: Field index out of range");
  return fields[static_cast<size_t>(i)].value;
}
inline std::string Any::FieldName(int64_t i) const {
  auto fields = reflect_fields(*this);
  if (i < 0 || static_cast<size_t>(i) >= fields.size()) panic("reflect: Field index out of range");
  return std::string(fields[static_cast<size_t>(i)].name);
}

inline void Any::SetInt(int64_t n) {
  if (!self) panic("reflect: SetInt on zero Value");
  switch (static_cast<RKind>(kind)) {
    case RKind::Int8: *static_cast<int8_t*>(self.get()) = static_cast<int8_t>(n); return;
    case RKind::Int16: *static_cast<int16_t*>(self.get()) = static_cast<int16_t>(n); return;
    case RKind::Int32: *static_cast<int32_t*>(self.get()) = static_cast<int32_t>(n); return;
    case RKind::Int64: *static_cast<int64_t*>(self.get()) = n; return;
    default: panic("reflect: SetInt on a non-integer Value");
  }
}
inline void Any::SetUint(uint64_t n) {
  if (!self) panic("reflect: SetUint on zero Value");
  switch (static_cast<RKind>(kind)) {
    case RKind::Uint8: *static_cast<uint8_t*>(self.get()) = static_cast<uint8_t>(n); return;
    case RKind::Uint16: *static_cast<uint16_t*>(self.get()) = static_cast<uint16_t>(n); return;
    case RKind::Uint32: *static_cast<uint32_t*>(self.get()) = static_cast<uint32_t>(n); return;
    case RKind::Uint64: *static_cast<uint64_t*>(self.get()) = n; return;
    default: panic("reflect: SetUint on a non-unsigned Value");
  }
}
inline void Any::SetFloat(double n) {
  if (!self) panic("reflect: SetFloat on zero Value");
  if (static_cast<RKind>(kind) == RKind::Float32) {
    *static_cast<float*>(self.get()) = static_cast<float>(n);
    return;
  }
  if (static_cast<RKind>(kind) == RKind::Float64) {
    *static_cast<double*>(self.get()) = n;
    return;
  }
  panic("reflect: SetFloat on a non-float Value");
}
inline void Any::SetBool(bool b) {
  if (!self || static_cast<RKind>(kind) != RKind::Bool) panic("reflect: SetBool on a non-bool Value");
  *static_cast<bool*>(self.get()) = b;
}
inline void Any::SetString(const std::string& s) {
  if (!self || static_cast<RKind>(kind) != RKind::String) panic("reflect: SetString on a non-string Value");
  *static_cast<std::string*>(self.get()) = s;
}
inline int64_t Any::Len() const {
  if (slice_len_fn && self) return slice_len_fn(self);
  panic("reflect: Len on a non-slice Value");
  return 0;
}
inline Any Any::Index(int64_t i) const {
  if (slice_index_fn && self) return slice_index_fn(self, i);
  panic("reflect: Index on a non-slice Value");
  return {};
}

// func values without <functional> (that header pulls a broken <cctype> on
// wasi-sdk noeh). Same virtual type-erasure as DeferList.
template<class Sig>
struct Func;
template<class R, class... Args>
struct Func<R(Args...)> {
  struct Base {
    virtual ~Base() = default;
    virtual R call(Args...) = 0;
  };
  template<class F>
  struct Hold : Base {
    F f;
    explicit Hold(F fn) : f(std::move(fn)) {}
    R call(Args... a) override { return f(std::forward<Args>(a)...); }
  };
  std::shared_ptr<Base> p;
  Func() = default;
  template<class F>
  Func(F f) : p(std::make_shared<Hold<F>>(std::move(f))) {}
  R operator()(Args... a) const {
    if (!p) panic("call of nil func");
    return p->call(std::forward<Args>(a)...);
  }
  bool is_nil() const { return !p; }
};
template<class R, class... Args>
bool is_nil(const Func<R(Args...)>& f) {
  return f.is_nil();
}

// ---- Oilpan-lite ------------------------------------------------------------
// Blink/cppgc's API, not Go's collector: garbage-collected C++ objects
// inherit GarbageCollected<T>, on-heap edges are Member<T>, off-heap roots
// are Persistent<T>, Trace(Visitor&) lists the edges. wasm32-wasip1 is one
// thread so this is stop-the-world mark-sweep (an explicit grey stack, never
// recursive mark -- WASM has no growable stacks). Conservative stack scan
// is not portable here; Collect() retains only objects reachable from
// Persistent roots. Slice/Map stay shared_ptr until the generator emits
// Trace on structs.

namespace gc {

enum : uint8_t { kWhite = 0, kBlack = 1 };

class Visitor;

struct GCObject {
  virtual ~GCObject() = default;
  virtual void Trace(Visitor&) const {}
  uint8_t color = kWhite;
};

template<class T>
struct GarbageCollected : GCObject {};

template<class T>
class Member {
  T* p_ = nullptr;

 public:
  Member() = default;
  Member(T* p) : p_(p) {}
  Member& operator=(T* p) {
    p_ = p;
    return *this;
  }
  Member& operator=(std::nullptr_t) {
    p_ = nullptr;
    return *this;
  }
  T* get() const { return p_; }
  T* operator->() const { return p_; }
  T& operator*() const { return *p_; }
  explicit operator bool() const { return p_ != nullptr; }
};

class PersistentBase {
 public:
  virtual ~PersistentBase() = default;
  virtual GCObject* GetObject() const = 0;
};

class Heap {
 public:
  template<class T, class... Args>
  T* Make(Args&&... args) {
    static_assert(std::is_base_of<GCObject, T>::value,
                  "gc::Make requires a GarbageCollected<T> type");
    T* p = new T(std::forward<Args>(args)...);
    objects_.push_back(p);
    return p;
  }
  void AddRoot(PersistentBase* r) { roots_.push_back(r); }
  void RemoveRoot(PersistentBase* r) {
    for (size_t i = 0; i < roots_.size(); ++i) {
      if (roots_[i] == r) {
        roots_[i] = roots_.back();
        roots_.pop_back();
        return;
      }
    }
  }
  void Collect();
  std::size_t live() const { return objects_.size(); }

 private:
  std::vector<GCObject*> objects_;
  std::vector<PersistentBase*> roots_;
};

inline Heap& heap() {
  static Heap h;
  return h;
}

class Visitor {
 public:
  explicit Visitor(std::vector<GCObject*>& grey) : grey_(grey) {}
  void Trace(GCObject* o) {
    if (!o || o->color == kBlack) return;
    grey_.push_back(o);
  }
  template<class T>
  void Trace(const Member<T>& m) {
    Trace(static_cast<GCObject*>(m.get()));
  }

 private:
  std::vector<GCObject*>& grey_;
};

inline void Heap::Collect() {
  for (auto* o : objects_) o->color = kWhite;
  std::vector<GCObject*> grey;
  for (auto* r : roots_) {
    if (auto* o = r->GetObject()) grey.push_back(o);
  }
  Visitor v(grey);
  size_t i = 0;
  while (i < grey.size()) {
    GCObject* o = grey[i++];
    if (!o || o->color == kBlack) continue;
    o->color = kBlack;
    o->Trace(v);
  }
  std::vector<GCObject*> keep;
  keep.reserve(objects_.size());
  for (auto* o : objects_) {
    if (o->color == kBlack)
      keep.push_back(o);
    else
      delete o;
  }
  objects_.swap(keep);
}

template<class T>
class Persistent : public PersistentBase {
  T* p_ = nullptr;
  void attach() {
    if (p_) heap().AddRoot(this);
  }
  void detach() {
    if (p_) heap().RemoveRoot(this);
  }

 public:
  Persistent() = default;
  Persistent(T* p) : p_(p) { attach(); }
  ~Persistent() { detach(); }
  Persistent(const Persistent&) = delete;
  Persistent& operator=(const Persistent&) = delete;
  Persistent(Persistent&& o) noexcept : p_(o.p_) {
    o.detach();
    o.p_ = nullptr;
    attach();
  }
  Persistent& operator=(Persistent&& o) noexcept {
    if (this == &o) return *this;
    detach();
    p_ = o.p_;
    o.detach();
    o.p_ = nullptr;
    attach();
    return *this;
  }
  Persistent& operator=(T* p) {
    detach();
    p_ = p;
    attach();
    return *this;
  }
  Persistent& operator=(std::nullptr_t) {
    detach();
    p_ = nullptr;
    return *this;
  }
  T* get() const { return p_; }
  T* operator->() const { return p_; }
  T& operator*() const { return *p_; }
  explicit operator bool() const { return p_ != nullptr; }
  GCObject* GetObject() const override { return p_; }
};

}  // namespace gc

// ---- Slice<T> ---------------------------------------------------------------
// Shared backing store + (off, len) window. append copies only when cap is
// exhausted -- Go's aliasing, without exposing a raw pointer.

template<class T>
struct Slice {
  std::shared_ptr<std::vector<T>> buf;
  std::size_t off = 0;
  std::size_t len_ = 0;

  Slice() = default;

  Slice(std::initializer_list<T> xs) {
    buf = std::make_shared<std::vector<T>>(xs);
    off = 0;
    len_ = buf->size();
  }

  static Slice make(std::size_t n, std::size_t c) {
    if (c < n) panic("make: cap out of range");
    Slice s;
    s.buf = std::make_shared<std::vector<T>>(c);
    s.off = 0;
    s.len_ = n;
    return s;
  }

  bool is_nil() const { return !buf; }
  std::size_t size() const { return len_; }
  int64_t len() const { return static_cast<int64_t>(len_); }
  int64_t cap() const {
    if (!buf) return 0;
    return static_cast<int64_t>(buf->size() - off);
  }

  T& operator[](int64_t i) {
    if (i < 0 || static_cast<std::size_t>(i) >= len_) {
      panic("runtime error: index out of range");
    }
    return (*buf)[off + static_cast<std::size_t>(i)];
  }
  const T& operator[](int64_t i) const { return const_cast<Slice*>(this)->operator[](i); }

  Slice slice(int64_t low, int64_t high) const {
    const int64_t l = len();
    const int64_t c = cap();
    if (low < 0) low = 0;
    if (high < 0) high = l;
    if (low > high || high > c) panic("runtime error: slice bounds out of range");
    if (!buf) {
      if (low == 0 && high == 0) return Slice{};
      panic("runtime error: slice bounds out of range");
    }
    Slice s = *this;
    s.off = off + static_cast<std::size_t>(low);
    s.len_ = static_cast<std::size_t>(high - low);
    return s;
  }

  Slice slice3(int64_t low, int64_t high, int64_t maxv) const {
    const int64_t c = cap();
    if (low < 0 || high < low || maxv < high || maxv > c) {
      panic("runtime error: slice bounds out of range");
    }
    return slice(low, high);
  }
};

template<class T>
bool is_nil(const Slice<T>& s) {
  return s.is_nil();
}
template<class T>
int64_t len(const Slice<T>& s) {
  return s.len();
}
template<class T>
int64_t cap(const Slice<T>& s) {
  return s.cap();
}

template <class T>
struct is_wasigo_slice : std::false_type {};
template <class T>
struct is_wasigo_slice<Slice<T>> : std::true_type {};

template <class T>
inline void finish_any_kind(Any& a) {
  if constexpr (is_wasigo_slice<T>::value) {
    a.kind = static_cast<int>(RKind::Slice);
    a.slice_len_fn = +[](const std::shared_ptr<void>& self) -> int64_t {
      if (!self) return 0;
      return static_cast<const T*>(self.get())->len();
    };
    a.slice_index_fn = +[](const std::shared_ptr<void>& self, int64_t i) -> Any {
      auto* sl = static_cast<T*>(self.get());
      return Any::adapt((*sl)[i]);
    };
  }
}
inline int64_t len(const std::string& s) { return static_cast<int64_t>(s.size()); }
template<class T, std::size_t N>
int64_t len(const std::array<T, N>&) {
  return static_cast<int64_t>(N);
}
template<class T, std::size_t N>
int64_t cap(const std::array<T, N>&) {
  return static_cast<int64_t>(N);
}

inline int64_t copy(Slice<uint8_t> dst, const std::string& src) {
  int64_t n = dst.len();
  if (static_cast<int64_t>(src.size()) < n) n = static_cast<int64_t>(src.size());
  for (int64_t i = 0; i < n; ++i) dst[i] = static_cast<uint8_t>(src[static_cast<size_t>(i)]);
  return n;
}

inline Slice<std::string>& os_args_store() {
  static Slice<std::string> args;
  return args;
}
inline Slice<std::string> os_args() { return os_args_store(); }
inline void os_exit(int64_t code) { std::exit(static_cast<int>(code)); }
inline std::string os_getenv(const std::string& key) {
  const char* v = std::getenv(key.c_str());
  return v ? std::string(v) : std::string{};
}

// time.Now(): (unix seconds, nanosecond remainder). std::chrono is
// portable across the host build and wasi-sdk, unlike POSIX
// clock_gettime (not reliably available under MSVC) -- generated code
// builds the real time::Time struct from this pair (see EmitCall's
// "time.Now" case; this header is included before that struct exists).
inline std::pair<int64_t, int64_t> time_now() {
  auto now = std::chrono::system_clock::now();
  int64_t total_ns = std::chrono::duration_cast<std::chrono::nanoseconds>(now.time_since_epoch()).count();
  int64_t sec = total_ns / 1000000000LL;
  int64_t nsec = total_ns % 1000000000LL;
  if (nsec < 0) {
    nsec += 1000000000LL;
    sec -= 1;
  }
  return {sec, nsec};
}

// ---- os.File ------------------------------------------------------------
// A real Read/Write/Close backed by plain C <cstdio> -- wasi-libc already
// implements fopen/fread/fwrite/fclose via WASI's path_open/fd_read/
// fd_write, so there's no need to hand-roll WASI syscalls (the same reason
// fmt.Print already just uses std::cout). os.File isn't parsed from a
// stdlib/os/*.go the way most packages are (os is one of the three
// builtins, see IsBuiltinImport) -- cpp_generator.cc maps the Go type
// `os.File` straight to this struct (NamedCppType) and synthesizes
// FuncDecls for Open/Create/ReadFile/WriteFile/Read/Write/Close so the
// usual multi-return (`f, err := os.Open(...)`)/method-call machinery
// still works unmodified.
// A plain-destructor RAII wrapper around FILE*, not a std::shared_ptr
// custom-deleter lambda -- wasi-sdk's O2 wasm32 codegen has a real bug
// where a stateless lambda used as a std::shared_ptr<FILE> deleter, once
// its containing __shared_ptr_pointer::__on_zero_shared() gets inlined at
// two or more call sites (e.g. an implicit close when a local File goes
// out of scope, plus a later explicit File.Close()), can end up as a
// `call_indirect` to an empty wasm table slot ("wasm trap: uninitialized
// element") instead of a direct call. An ordinary non-template destructor
// isn't affected. `owns` is false for the process-standard streams, which
// must never be fclose'd.
struct FileHandle {
  FILE* raw = nullptr;
  bool owns = true;
  FileHandle() = default;
  FileHandle(FILE* r, bool o) : raw(r), owns(o) {}
  ~FileHandle() {
    if (raw && owns) std::fclose(raw);
  }
};

struct File {
  std::shared_ptr<FileHandle> fp;

  bool is_nil() const { return !fp || !fp->raw; }

  struct ReadResult {
    int64_t r0 = 0;
    Error r1{};
  };
  ReadResult Read(Slice<uint8_t> p) {
    if (!fp || !fp->raw) return {0, errors_new("file already closed")};
    if (p.size() == 0) return {0, Error()};
    size_t n = std::fread(&(*p.buf)[p.off], 1, p.size(), fp->raw);
    if (n == 0) return {0, errors_new("EOF")};
    return {static_cast<int64_t>(n), Error()};
  }

  struct WriteResult {
    int64_t r0 = 0;
    Error r1{};
  };
  WriteResult Write(Slice<uint8_t> p) {
    if (!fp || !fp->raw) return {0, errors_new("file already closed")};
    if (p.size() == 0) return {0, Error()};
    size_t n = std::fwrite(&(*p.buf)[p.off], 1, p.size(), fp->raw);
    if (n != p.size()) return {static_cast<int64_t>(n), errors_new("short write")};
    return {static_cast<int64_t>(n), Error()};
  }

  Error Close() {
    if (!fp || !fp->raw) return Error();
    bool ok = std::fclose(fp->raw) == 0;
    fp->raw = nullptr;
    fp->owns = false;
    if (!ok) return errors_new("close: I/O error");
    return Error();
  }
};

inline File os_stdout_file() {
  File f;
  f.fp = std::make_shared<FileHandle>(stdout, false);
  return f;
}
inline File os_stdin_file() {
  File f;
  f.fp = std::make_shared<FileHandle>(stdin, false);
  return f;
}
inline File os_stderr_file() {
  File f;
  f.fp = std::make_shared<FileHandle>(stderr, false);
  return f;
}

struct OsOpenResult {
  File r0;
  Error r1{};
};
inline OsOpenResult os_open(const std::string& name) {
  FILE* raw = std::fopen(name.c_str(), "rb");
  if (!raw) return {File{}, errors_new("open " + name + ": no such file or directory")};
  File f;
  f.fp = std::make_shared<FileHandle>(raw, true);
  return {f, Error()};
}
inline OsOpenResult os_create(const std::string& name) {
  FILE* raw = std::fopen(name.c_str(), "wb");
  if (!raw) return {File{}, errors_new("create " + name + ": cannot create file")};
  File f;
  f.fp = std::make_shared<FileHandle>(raw, true);
  return {f, Error()};
}

struct OsReadFileResult {
  Slice<uint8_t> r0;
  Error r1{};
};
inline OsReadFileResult os_read_file(const std::string& name) {
  auto opened = os_open(name);
  if (!opened.r1.is_nil()) return {Slice<uint8_t>{}, opened.r1};
  std::vector<uint8_t> data;
  uint8_t buf[4096];
  for (;;) {
    size_t n = std::fread(buf, 1, sizeof(buf), opened.r0.fp->raw);
    if (n > 0) data.insert(data.end(), buf, buf + n);
    if (n < sizeof(buf)) break;
  }
  Slice<uint8_t> out;
  out.buf = std::make_shared<std::vector<uint8_t>>(std::move(data));
  out.len_ = out.buf->size();
  return {out, Error()};
}

inline Error os_write_file(const std::string& name, Slice<uint8_t> data, int64_t /*perm*/) {
  auto created = os_create(name);
  if (!created.r1.is_nil()) return created.r1;
  if (data.size() > 0) {
    size_t n = std::fwrite(&(*data.buf)[data.off], 1, data.size(), created.r0.fp->raw);
    if (n != data.size()) return errors_new("short write");
  }
  return Error();
}

// ---- os.FileInfo / os.Stat ------------------------------------------------
// Backed by plain <sys/stat.h> `stat(2)` -- wasi-libc implements it via
// WASI's path_filestat_get the same way it implements fopen/fread above, and
// the same call compiles unmodified under a native (non-WASI) g++/clang++,
// which is what the CMake "_native" golden tests actually link against (see
// go++/CMakeLists.txt's wasigo_add_golden -- wasi-sdk only supplies the
// separate wasm proof). Bounded to what a directory-walking caller
// (goxx/uniloader/bundle.Collect, the first caller) needs: Name/Size/IsDir.
// No Mode()/ModTime()/Sys() -- those would need a real os.FileMode with
// String/Perm methods and a real time.Time built from a raw struct stat,
// more than any caller here exercises yet.
struct FileInfo {
  std::string name_;
  int64_t size_ = 0;
  bool is_dir_ = false;

  std::string Name() const { return name_; }
  int64_t Size() const { return size_; }
  bool IsDir() const { return is_dir_; }
};

struct OsStatResult {
  FileInfo r0;
  Error r1{};
};
inline OsStatResult os_stat(const std::string& name) {
  struct stat st;
  if (::stat(name.c_str(), &st) != 0) {
    return {FileInfo{}, errors_new("stat " + name + ": no such file or directory")};
  }
  FileInfo fi;
  size_t slash = name.find_last_of('/');
  fi.name_ = slash == std::string::npos ? name : name.substr(slash + 1);
  fi.size_ = static_cast<int64_t>(st.st_size);
  fi.is_dir_ = S_ISDIR(st.st_mode) != 0;
  return {fi, Error()};
}

// ---- os.DirEntry / os.ReadDir ----------------------------------------------
// Real directory listing via <dirent.h> opendir/readdir/closedir -- wasi-libc
// implements these on top of WASI preview 1's real fd_readdir, so this is
// genuine enumeration, not a stub (unlike os/exec's "not supported on
// wasm32-wasip1", where WASI truly has no such syscall at all). IsDir() is
// resolved with a follow-up `stat` per entry rather than trusting
// dirent::d_type: d_type is a BSD/glibc extension WASI's dirent shim does
// populate, but a native (non-WASI) libc used by the "_native" golden-test
// build (see os_stat's comment above) is not guaranteed to, and this
// function has to compile and run correctly under both. Entries come back
// sorted by name, matching real Go's os.ReadDir contract.
struct DirEntry {
  std::string name_;
  bool is_dir_ = false;

  std::string Name() const { return name_; }
  bool IsDir() const { return is_dir_; }
};

struct OsReadDirResult {
  Slice<DirEntry> r0;
  Error r1{};
};
inline OsReadDirResult os_read_dir(const std::string& name) {
  DIR* d = ::opendir(name.c_str());
  if (!d) {
    return {Slice<DirEntry>{}, errors_new("readdir " + name + ": no such directory")};
  }
  std::vector<DirEntry> entries;
  for (struct dirent* ent = ::readdir(d); ent != nullptr; ent = ::readdir(d)) {
    std::string n = ent->d_name;
    if (n == "." || n == "..") continue;
    struct stat st;
    DirEntry de;
    de.name_ = n;
    de.is_dir_ = ::stat((name + "/" + n).c_str(), &st) == 0 && S_ISDIR(st.st_mode);
    entries.push_back(std::move(de));
  }
  ::closedir(d);
  std::sort(entries.begin(), entries.end(),
            [](const DirEntry& a, const DirEntry& b) { return a.name_ < b.name_; });
  Slice<DirEntry> out;
  out.buf = std::make_shared<std::vector<DirEntry>>(std::move(entries));
  out.len_ = out.buf->size();
  return {out, Error()};
}

inline std::string string_from_bytes(const Slice<uint8_t>& s) {
  if (!s.buf || s.len_ == 0) return {};
  return std::string(reinterpret_cast<const char*>(&(*s.buf)[s.off]), s.len_);
}

inline Slice<uint8_t> bytes_from_string(const std::string& s) {
  Slice<uint8_t> out;
  out.buf = std::make_shared<std::vector<uint8_t>>(s.begin(), s.end());
  out.off = 0;
  out.len_ = s.size();
  return out;
}

// UTF-8 decode matching Go's range-over-string: invalid sequences become
// RuneError (U+FFFD) and consume one byte. size 0 means i is out of range.
struct DecodedRune {
  int32_t r = 0xFFFD;
  int64_t size = 1;
};

inline DecodedRune decode_rune(const std::string& s, int64_t i) {
  DecodedRune d;
  if (i < 0 || static_cast<size_t>(i) >= s.size()) {
    d.r = 0;
    d.size = 0;
    return d;
  }
  const auto* p = reinterpret_cast<const unsigned char*>(s.data() + static_cast<size_t>(i));
  const size_t n = s.size() - static_cast<size_t>(i);
  const unsigned char c0 = p[0];
  auto cont = [&](size_t k) { return k < n && (p[k] & 0xC0) == 0x80; };
  if (c0 < 0x80) {
    d.r = c0;
    d.size = 1;
    return d;
  }
  if ((c0 & 0xE0) == 0xC0 && n >= 2 && cont(1)) {
    int32_t r = (static_cast<int32_t>(c0 & 0x1F) << 6) | (p[1] & 0x3F);
    if (r >= 0x80) {
      d.r = r;
      d.size = 2;
      return d;
    }
  } else if ((c0 & 0xF0) == 0xE0 && n >= 3 && cont(1) && cont(2)) {
    int32_t r = (static_cast<int32_t>(c0 & 0x0F) << 12) | (static_cast<int32_t>(p[1] & 0x3F) << 6) |
                (p[2] & 0x3F);
    if (r >= 0x800 && (r < 0xD800 || r > 0xDFFF)) {
      d.r = r;
      d.size = 3;
      return d;
    }
  } else if ((c0 & 0xF8) == 0xF0 && n >= 4 && cont(1) && cont(2) && cont(3)) {
    int32_t r = (static_cast<int32_t>(c0 & 0x07) << 18) | (static_cast<int32_t>(p[1] & 0x3F) << 12) |
                (static_cast<int32_t>(p[2] & 0x3F) << 6) | (p[3] & 0x3F);
    if (r >= 0x10000 && r <= 0x10FFFF) {
      d.r = r;
      d.size = 4;
      return d;
    }
  }
  d.r = 0xFFFD;
  d.size = 1;
  return d;
}

inline Slice<int32_t> runes_from_string(const std::string& s) {
  std::vector<int32_t> tmp;
  int64_t i = 0;
  while (i < static_cast<int64_t>(s.size())) {
    auto d = decode_rune(s, i);
    if (d.size <= 0) break;
    tmp.push_back(d.r);
    i += d.size;
  }
  Slice<int32_t> out;
  out.buf = std::make_shared<std::vector<int32_t>>(std::move(tmp));
  out.off = 0;
  out.len_ = out.buf->size();
  return out;
}

inline std::string string_from_runes(const Slice<int32_t>& rs) {
  std::string out;
  for (int64_t i = 0; i < rs.len(); ++i) {
    uint32_t r = static_cast<uint32_t>(rs[i]);
    if (r < 0x80) {
      out.push_back(static_cast<char>(r));
    } else if (r < 0x800) {
      out.push_back(static_cast<char>(0xC0 | (r >> 6)));
      out.push_back(static_cast<char>(0x80 | (r & 0x3F)));
    } else if (r < 0x10000) {
      if (r >= 0xD800 && r <= 0xDFFF) r = 0xFFFD;
      out.push_back(static_cast<char>(0xE0 | (r >> 12)));
      out.push_back(static_cast<char>(0x80 | ((r >> 6) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | (r & 0x3F)));
    } else {
      if (r > 0x10FFFF) r = 0xFFFD;
      out.push_back(static_cast<char>(0xF0 | (r >> 18)));
      out.push_back(static_cast<char>(0x80 | ((r >> 12) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | ((r >> 6) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | (r & 0x3F)));
    }
  }
  return out;
}

template<class T, class... Xs>
Slice<T> append(Slice<T> s, Xs&&... xs) {
  const std::size_t add = sizeof...(xs);
  const std::size_t need = s.len_ + add;
  if (!s.buf || s.off + need > s.buf->size()) {
    std::size_t new_cap = need < 1 ? 1 : need;
    if (s.buf) {
      const std::size_t old_cap = s.buf->size() - s.off;
      if (old_cap * 2 > new_cap) new_cap = old_cap * 2;
    }
    auto nbuf = std::make_shared<std::vector<T>>();
    nbuf->resize(new_cap);
    if (s.buf && s.len_ > 0) {
      for (std::size_t i = 0; i < s.len_; ++i) (*nbuf)[i] = (*s.buf)[s.off + i];
    }
    s.buf = std::move(nbuf);
    s.off = 0;
  }
  auto put = [&](auto&& x) {
    (*s.buf)[s.off + s.len_] = std::forward<decltype(x)>(x);
    s.len_++;
  };
  (put(std::forward<Xs>(xs)), ...);
  return s;
}

template<class T>
Slice<T> append_range(Slice<T> s, Slice<T> extra) {
  for (int64_t i = 0; i < extra.len(); ++i) s = append(s, extra[i]);
  return s;
}
inline Slice<uint8_t> append_range(Slice<uint8_t> s, const std::string& extra) {
  for (char c : extra) s = append(s, static_cast<uint8_t>(c));
  return s;
}

template<class T>
int64_t copy(Slice<T> dst, Slice<T> src) {
  int64_t n = dst.len();
  if (src.len() < n) n = src.len();
  if (n <= 0) return 0;
  if (dst.buf == src.buf) {
    std::vector<T> tmp(static_cast<std::size_t>(n));
    for (int64_t i = 0; i < n; ++i) tmp[static_cast<std::size_t>(i)] = src[i];
    for (int64_t i = 0; i < n; ++i) dst[i] = tmp[static_cast<std::size_t>(i)];
  } else {
    for (int64_t i = 0; i < n; ++i) dst[i] = src[i];
  }
  return n;
}

template<class T>
Slice<T> make_slice(int64_t n, int64_t c = -1) {
  if (n < 0) panic("make: negative len");
  if (c < 0) c = n;
  if (c < n) panic("make: cap out of range");
  return Slice<T>::make(static_cast<std::size_t>(n), static_cast<std::size_t>(c));
}

// Arrays live on the stack; slicing copies the window into a Slice (WASM has
// no growable stacks to alias a stack array the way Go's heap-escape would).
template<class T, std::size_t N>
Slice<T> slice_array(const std::array<T, N>& a, int64_t low, int64_t high) {
  const int64_t n = static_cast<int64_t>(N);
  if (low < 0) low = 0;
  if (high < 0) high = n;
  if (low > high || high > n) panic("runtime error: slice bounds out of range");
  Slice<T> s = make_slice<T>(high - low);
  for (int64_t i = 0; i < high - low; ++i) {
    s[i] = a[static_cast<std::size_t>(low + i)];
  }
  return s;
}

template<class T>
T gmin(T a) {
  return a;
}
template<class T, class... Rest>
T gmin(T a, Rest... rest) {
  T b = gmin(rest...);
  return a < b ? a : b;
}
template<class T>
T gmax(T a) {
  return a;
}
template<class T, class... Rest>
T gmax(T a, Rest... rest) {
  T b = gmax(rest...);
  return a < b ? b : a;
}

template<class T>
void gclear(Slice<T> s) {
  for (int64_t i = 0; i < s.len(); ++i) s[i] = T{};
}

// ---- gocvm --------------------------------------------------------------
// The one dispatch gate between compiled Go++ code and a native host
// bridge (e.g. ~/shim_sandbox's real Winsock/Win32 backends, wired in
// under `goclang++.bat --shim-sandbox`). Not a generic FFI: gocvm.Call is
// the single Go-visible entry point (see cpp_generator.cc's "gocvm"
// EmitCall branch) -- every crossing goes through the same ABAC check and
// comes back as a normal wasigo::Error naming the topic, the same shape
// os/exec's, os/user's, and syscall's own stub errors already have.
namespace gocvm {

inline constexpr const char* kNoBridge =
    "no host bridge registered (build with goclang++.bat "
    "--shim-sandbox to link one)";

class HostBridge {
 public:
  virtual ~HostBridge() = default;
  // Synchronous dispatch inside the calling goroutine's coroutine frame.
  // wasigo is single-threaded cooperative, so blocking here is exactly
  // as safe as os_open's synchronous fread already is.
  //
  // ErrorState contract: gocvm::Call sets g_error_state to kBridgeActive
  // before calling this.  If anything inside the bridge (or code it
  // calls) hits panic(), the message is stashed on g_error_state and
  // panic_or_stash() returns true.  The bridge MUST check
  //   g_error_state().bridge_panicked()
  // after any operation that could panic and return false immediately.
  // gocvm::Call will then surface the stashed message as a wasigo::Error.
  //
  // Do NOT use C++ throw — noeh builds have no exception support.
  virtual bool Call(const std::string& topic, const std::string& payload,
                    std::string* reply_out, std::string* err_out) = 0;
};

class AbacHook {
 public:
  virtual ~AbacHook() = default;
  virtual bool Check(const std::string& topic) = 0;
};

namespace detail {
inline HostBridge*& bridge_slot() {
  static HostBridge* b = nullptr;
  return b;
}
inline AbacHook*& abac_slot() {
  static AbacHook* a = nullptr;
  return a;
}
}  // namespace detail

// A plain wasi-sdk build never calls these -- registration only happens
// from set_os_args's WASIGO_GOCVM_BRIDGE hook below, which only exists
// when goclang++.bat --shim-sandbox defined it.
inline void RegisterHostBridge(HostBridge* b) { detail::bridge_slot() = b; }
inline void RegisterAbacHook(AbacHook* a) { detail::abac_slot() = a; }

// Virtual-thread registry entry: one per goroutine actually spawned via
// go() (not per co_await'd helper coroutine -- same distinction Go's own
// `go` statement draws). cppgc-managed so its lifetime -- and, later, any
// GC-managed pending-call state it grows -- participates in the same
// wasigo::gc::Heap as everything else. Owned by a gc::Persistent held in
// the spawned coroutine's own promise_type (see the three go() overloads
// below), so it is rooted for exactly the coroutine frame's lifetime with
// no separate registry list to keep in sync by hand.
struct VThread : gc::GarbageCollected<VThread> {
  enum class State { kRunning, kAwaitingHost };
  uint64_t id = 0;
  // v0's Call() below is synchronous (no suspension point), so nothing
  // transitions this yet -- it's here so a future suspending dispatch
  // doesn't need a VThread redesign, not a currently-live state machine.
  State state = State::kRunning;
  void Trace(gc::Visitor&) const override {}
};

inline VThread* RegisterThread() {
  auto* t = gc::heap().Make<VThread>();
  static uint64_t next_id = 0;
  t->id = ++next_id;
  return t;
}

// (string, error) -- the same "rN field per return value" shape every
// multi-return Go++ function compiles to (see cpp_generator.cc's
// EmitAssign, and wasigo::OsOpenResult for the precedent), so `s, err :=
// gocvm.Call(...)` unpacks through the ordinary multi-return codegen path
// with no special-casing beyond the EmitCall branch that names this type.
struct CallResult {
  std::string r0;
  Error r1{};
};

// The one dispatch primitive. ABAC deny and "no bridge" both come back as
// a normal wasigo::Error naming the topic -- once a real bridge is
// registered these stop always being the same canned string, same as
// every existing stdlib stub's error already reads.
//
// The single SFI error boundary.  ErrorState transitions:
//   kClear → kBridgeActive (enter_bridge)
//   kBridgeActive → kPanic  (if bridge or anything it calls hits panic())
//   kBridgeActive → kClear  (leave_bridge, success)
//   kPanic → kClear         (consume_panic, surfaced as wasigo::Error)
//
// No C++ exceptions, no setjmp.  Works identically on noeh and native.
//
// BridgeScope RAII guarantees kBridgeActive is never leaked, even if a
// future maintainer adds an early return.
//
// Cooperative yield safety: bridge calls are synchronous run-to-completion
// (HostBridge::Call never co_awaits or yields back to the VThread
// scheduler), so a single thread-local ErrorState has zero risk of
// cross-coroutine contamination.  If a future bridge needs to suspend
// mid-call, ErrorState must move onto the VThread or become per-coroutine.
//
// recover() stays localised to explicit defer wrappers (PanicFrame +
// goto __wasigo_end) — the same-function-only semantics documented in
// language.md.  ErrorState does not change recover's reach; it only
// prevents abort at the SFI boundary so that gocvm::Call can surface
// bridge panics as wasigo::Error instead.
inline CallResult Call(const std::string& topic, const std::string& payload) {
  if (detail::abac_slot() && !detail::abac_slot()->Check(topic)) {
    return {std::string(), Error("gocvm: " + topic + ": abac deny")};
  }
  if (!detail::bridge_slot()) {
    return {std::string(), Error("gocvm: " + topic + ": " + std::string(kNoBridge))};
  }
  BridgeScope scope(g_error_state().enter_bridge());
  if (!scope.entered) {
    return {std::string(),
            Error("gocvm: " + topic + ": reentrant bridge call")};
  }

  std::string reply, err;
  bool ok = detail::bridge_slot()->Call(topic, payload, &reply, &err);

  // Check for a panic that fired inside the bridge (or anything it called).
  if (g_error_state().is_panic()) {
    std::string msg = g_error_state().consume_panic();
    scope.mark_consumed();
    return {std::string(),
            Error("gocvm: " + topic + ": bridge panic: " + msg)};
  }

  g_error_state().leave_bridge();
  scope.mark_consumed();

  if (!ok) {
    return {std::string(), Error("gocvm: " + topic + ": " + err)};
  }
  return {std::move(reply), Error()};
}

}  // namespace gocvm

#if defined(WASIGO_GOCVM_BRIDGE) && WASIGO_GOCVM_BRIDGE
// Defined by shim_sandbox (src/gocvm_bridge.cc), linked in only when
// goclang++.bat --shim-sandbox passed -DWASIGO_GOCVM_BRIDGE=1. (A
// linkage-specification like `extern "C"` is only valid at namespace
// scope, not inside a function body, hence the forward declaration up
// here rather than next to its one call site below.)
extern "C" void wasigo_gocvm_install_bridge();
#endif

inline void set_os_args(int argc, char** argv) {
  auto& a = os_args_store();
  a = make_slice<std::string>(argc < 0 ? 0 : argc);
  for (int i = 0; i < argc; ++i) a[i] = argv[i] ? argv[i] : "";
#if defined(WASIGO_GOCVM_BRIDGE) && WASIGO_GOCVM_BRIDGE
  wasigo_gocvm_install_bridge();
#endif
}

// ---- Map<K,V> ---------------------------------------------------------------
// unordered_map on purpose: Go's iteration order is unspecified, and C++ does
// not pretend otherwise. Nil vs empty is preserved; assign-to-nil panics.

template<class K, class V>
struct Map {
  using inner = std::unordered_map<K, V>;
  std::shared_ptr<inner> p;

  Map() = default;
  Map(std::initializer_list<std::pair<const K, V>> xs) : p(std::make_shared<inner>(xs)) {}

  static Map make() {
    Map m;
    m.p = std::make_shared<inner>();
    return m;
  }

  bool is_nil() const { return !p; }
  int64_t len() const { return p ? static_cast<int64_t>(p->size()) : 0; }
  std::size_t size() const { return p ? p->size() : 0; }

  V& operator[](const K& k) {
    if (!p) panic("assignment to entry in nil map");
    return (*p)[k];
  }

  std::pair<V, bool> lookup(const K& k) const {
    if (!p) return {V{}, false};
    auto it = p->find(k);
    if (it == p->end()) return {V{}, false};
    return {it->second, true};
  }

  void del(const K& k) {
    if (p) p->erase(k);
  }

  static inner& empty_inner() {
    static inner e;
    return e;
  }
  auto begin() { return p ? p->begin() : empty_inner().begin(); }
  auto end() { return p ? p->end() : empty_inner().end(); }
  auto begin() const { return p ? p->begin() : empty_inner().begin(); }
  auto end() const { return p ? p->end() : empty_inner().end(); }
};

template<class K, class V>
bool is_nil(const Map<K, V>& m) {
  return m.is_nil();
}
template<class K, class V>
int64_t len(const Map<K, V>& m) {
  return m.len();
}
template<class K, class V>
Map<K, V> make_map() {
  return Map<K, V>::make();
}
template<class K, class V>
void del(Map<K, V>& m, const K& k) {
  m.del(k);
}
template<class K, class V>
void gclear(Map<K, V> m) {
  if (m.p) m.p->clear();
}

template<class T>
T* New() {
  return new T{};
}

#ifdef WASIGO_NEED_CORO
// ---- cooperative scheduler + Task ------------------------------------------
// One runqueue. A Task is a C++20 coroutine. go() detaches it onto the queue;
// send/recv/select suspend the current handle onto a channel waiter list.
// There are no OS threads: this is the mapping that actually works on
// wasm32-wasip1 (wasi-threads / pthread would be forcing Go's OS-facing
// scheduler onto a target that does not have it).

struct Scheduler {
  std::deque<std::coroutine_handle<>> ready;
  int parked = 0;

  void enqueue(std::coroutine_handle<> h) { ready.push_back(h); }

  void run();  // defined after Task
};

inline Scheduler& scheduler() {
  static Scheduler s;
  return s;
}

struct Task {
  struct promise_type {
    std::coroutine_handle<> continuation{};
    bool detached = false;
    // Only set when go() actually spawns this coroutine as a goroutine
    // (see go() below) -- rooted for exactly this frame's lifetime, no
    // separate registry to keep in sync by hand.
    gc::Persistent<gocvm::VThread> vthread;

    Task get_return_object() {
      return Task{std::coroutine_handle<promise_type>::from_promise(*this)};
    }
    std::suspend_always initial_suspend() noexcept { return {}; }
    auto final_suspend() noexcept {
      struct FS {
        promise_type* p;
        bool await_ready() noexcept { return false; }
        std::coroutine_handle<> await_suspend(std::coroutine_handle<>) noexcept {
          if (p->continuation) return p->continuation;
          return std::noop_coroutine();
        }
        void await_resume() noexcept {}
      };
      return FS{this};
    }
    void return_void() {}
    void unhandled_exception() { std::abort(); }
  };

  std::coroutine_handle<promise_type> h{};

  Task() = default;
  explicit Task(std::coroutine_handle<promise_type> hh) : h(hh) {}
  Task(Task&& o) noexcept : h(o.h) { o.h = {}; }
  Task& operator=(Task&& o) noexcept {
    if (this != &o) {
      destroy_if_owned();
      h = o.h;
      o.h = {};
    }
    return *this;
  }
  Task(const Task&) = delete;
  Task& operator=(const Task&) = delete;
  ~Task() { destroy_if_owned(); }

  void destroy_if_owned() {
    if (!h) return;
    if (h.promise().detached) return;
    h.destroy();
    h = {};
  }

  bool await_ready() const noexcept { return !h || h.done(); }
  void await_suspend(std::coroutine_handle<> waiter) {
    h.promise().continuation = waiter;
    // Scheduler owns the frame until it completes; otherwise ~Task and
    // Scheduler::run both destroy it (heap corruption on MSVC).
    h.promise().detached = true;
    scheduler().enqueue(h);
  }
  void await_resume() {}
};

// Task with a result: a channel-using function that returns T becomes
// TaskT<T> rather than Task (void). Same runqueue, same stackless frames.
template<class T>
struct TaskT {
  struct promise_type {
    std::coroutine_handle<> continuation{};
    bool detached = false;
    T value{};
    // Only set when go() actually spawns this coroutine as a goroutine
    // (see go() below) -- rooted for exactly this frame's lifetime, no
    // separate registry to keep in sync by hand.
    gc::Persistent<gocvm::VThread> vthread;

    TaskT get_return_object() {
      return TaskT{std::coroutine_handle<promise_type>::from_promise(*this)};
    }
    std::suspend_always initial_suspend() noexcept { return {}; }
    auto final_suspend() noexcept {
      struct FS {
        promise_type* p;
        bool await_ready() noexcept { return false; }
        std::coroutine_handle<> await_suspend(std::coroutine_handle<>) noexcept {
          if (p->continuation) return p->continuation;
          return std::noop_coroutine();
        }
        void await_resume() noexcept {}
      };
      return FS{this};
    }
    void return_value(T v) { value = std::move(v); }
    void unhandled_exception() { std::abort(); }
  };

  std::coroutine_handle<promise_type> h{};

  TaskT() = default;
  explicit TaskT(std::coroutine_handle<promise_type> hh) : h(hh) {}
  TaskT(TaskT&& o) noexcept : h(o.h) { o.h = {}; }
  TaskT& operator=(TaskT&& o) noexcept {
    if (this != &o) {
      destroy_if_owned();
      h = o.h;
      o.h = {};
    }
    return *this;
  }
  TaskT(const TaskT&) = delete;
  TaskT& operator=(const TaskT&) = delete;
  ~TaskT() { destroy_if_owned(); }

  void destroy_if_owned() {
    if (!h) return;
    if (h.promise().detached) return;
    h.destroy();
    h = {};
  }

  bool await_ready() const noexcept { return !h || h.done(); }
  void await_suspend(std::coroutine_handle<> waiter) {
    h.promise().continuation = waiter;
    h.promise().detached = true;
    scheduler().enqueue(h);
  }
  T await_resume() { return std::move(h.promise().value); }
};

inline void Scheduler::run() {
  while (!ready.empty()) {
    auto h = ready.front();
    ready.pop_front();
    h.resume();
    if (h.done()) h.destroy();
  }
  if (parked > 0) panic("fatal error: all goroutines are asleep - deadlock!");
}

inline void go(Task t) {
  if (!t.h) return;
  t.h.promise().detached = true;
  t.h.promise().vthread = gocvm::RegisterThread();
  scheduler().enqueue(t.h);
  t.h = {};
}

template<class T>
void go(TaskT<T> t) {
  if (!t.h) return;
  t.h.promise().detached = true;
  t.h.promise().vthread = gocvm::RegisterThread();
  scheduler().enqueue(t.h);
  t.h = {};
}

template<class F>
void go(F f) {
  go([](F fn) -> Task {
    fn();
    co_return;
  }(std::move(f)));
}

// `go func(){ <uses channels> }()`: EmitGo cannot invoke the func literal
// immediately and pass the resulting Task straight to go() the way it
// does for a plain function or method call, because a *lambda* coroutine
// (unlike a free function) stores only a pointer back to its own closure
// object ("this") in its frame -- captured state itself (by value or by
// reference; capture mode makes no difference here) lives in the closure
// object, not the frame. `(closure)()` as an immediately-invoked
// temporary is destroyed at the end of that full expression, but the
// produced Task is only *initially suspended*: the scheduler resumes it
// later, by which point the closure is already gone -- a genuine
// use-after-free, not merely a style concern. (Contrast with go(F f)
// just above: its own inner `[](F fn) -> Task {...}(std::move(f))` is
// safe *because* that wrapper lambda is captureless -- fn is a real
// coroutine parameter living in the frame, not something read through a
// dangling "this".) Fix: pass the closure ITSELF (uninvoked) as this
// wrapper's own by-value parameter, so a live copy persists inside the
// wrapper's own frame for the whole call; only then invoke and co_await
// it. EmitGo selects GoAsyncLit for a literal with no results, GoAsyncLitT
// for exactly one (matching EmitFuncLit's own "cannot return multiple
// values" bound on a channel-using literal).
template<class F>
Task GoAsyncLit(F f) {
  co_await f();
  co_return;
}
template<class T, class F>
TaskT<T> GoAsyncLitT(F f) {
  co_return co_await f();
}

inline int run() {
  scheduler().run();
  return 0;
}

inline int run(Task t) {
  go(std::move(t));
  return run();
}

// ---- Chan<T> ----------------------------------------------------------------
// Reference type (shared_ptr state), like Go. A waiter may be a plain
// send/recv or a select case (cancelled + winner index). Completing a
// cancelled waiter is a no-op so the losing select cases do not steal later
// values.

template<class T>
struct Chan {
  struct RecvWaiter {
    std::coroutine_handle<> h{};
    T* slot = nullptr;
    bool* ok = nullptr;
    std::shared_ptr<bool> cancelled;
    std::shared_ptr<int> winner;
    int idx = -1;
    bool counted = true;
  };
  struct SendWaiter {
    std::coroutine_handle<> h{};
    T value{};
    std::shared_ptr<bool> cancelled;
    std::shared_ptr<int> winner;
    int idx = -1;
    bool counted = true;
  };
  struct State {
    std::deque<T> buf;
    std::size_t cap = 0;
    bool closed = false;
    std::deque<RecvWaiter> recvs;
    std::deque<SendWaiter> sends;
  };
  std::shared_ptr<State> st;

  Chan() = default;
  explicit Chan(std::size_t cap) : st(std::make_shared<State>()) { st->cap = cap; }

  bool is_nil() const { return !st; }

  static bool is_cancelled(const std::shared_ptr<bool>& c) { return c && *c; }

  static void complete_recv(RecvWaiter& w, T&& v, bool ok) {
    if (is_cancelled(w.cancelled)) return;
    if (w.cancelled) *w.cancelled = true;
    if (w.winner) *w.winner = w.idx;
    if (w.slot) *w.slot = std::move(v);
    if (w.ok) *w.ok = ok;
    if (w.counted) scheduler().parked--;
    scheduler().enqueue(w.h);
  }

  static void complete_send(SendWaiter& w) {
    if (is_cancelled(w.cancelled)) return;
    if (w.cancelled) *w.cancelled = true;
    if (w.winner) *w.winner = w.idx;
    if (w.counted) scheduler().parked--;
    scheduler().enqueue(w.h);
  }

  bool try_send(T& value) {
    if (!st) panic("send on nil channel");
    if (st->closed) panic("send on closed channel");
    while (!st->recvs.empty()) {
      RecvWaiter w = std::move(st->recvs.front());
      st->recvs.pop_front();
      if (is_cancelled(w.cancelled)) continue;
      complete_recv(w, std::move(value), true);
      return true;
    }
    if (st->buf.size() < st->cap) {
      st->buf.push_back(std::move(value));
      return true;
    }
    return false;
  }

  bool try_recv(T* slot, bool* ok) {
    if (!st) panic("receive from nil channel");
    if (!st->buf.empty()) {
      if (slot) *slot = std::move(st->buf.front());
      st->buf.pop_front();
      if (ok) *ok = true;
      while (!st->sends.empty()) {
        SendWaiter w = std::move(st->sends.front());
        st->sends.pop_front();
        if (is_cancelled(w.cancelled)) continue;
        st->buf.push_back(std::move(w.value));
        complete_send(w);
        break;
      }
      return true;
    }
    while (!st->sends.empty()) {
      SendWaiter w = std::move(st->sends.front());
      st->sends.pop_front();
      if (is_cancelled(w.cancelled)) continue;
      if (slot) *slot = std::move(w.value);
      if (ok) *ok = true;
      complete_send(w);
      return true;
    }
    if (st->closed) {
      if (slot) *slot = T{};
      if (ok) *ok = false;
      return true;
    }
    return false;
  }

  struct SendAwaiter {
    Chan* ch;
    T value;
    bool await_ready() { return ch->try_send(value); }
    void await_suspend(std::coroutine_handle<> h) {
      ch->st->sends.push_back(SendWaiter{h, std::move(value), nullptr, nullptr, -1, true});
      scheduler().parked++;
    }
    void await_resume() {
      if (ch->st && ch->st->closed) panic("send on closed channel");
    }
  };

  struct RecvAwaiter {
    Chan* ch;
    T value{};
    bool ok = false;
    bool await_ready() { return ch->try_recv(&value, &ok); }
    void await_suspend(std::coroutine_handle<> h) {
      ch->st->recvs.push_back(RecvWaiter{h, &value, &ok, nullptr, nullptr, -1, true});
      scheduler().parked++;
    }
    T await_resume() { return std::move(value); }
  };

  struct RecvOkAwaiter {
    Chan* ch;
    T value{};
    bool ok = false;
    bool await_ready() { return ch->try_recv(&value, &ok); }
    void await_suspend(std::coroutine_handle<> h) {
      ch->st->recvs.push_back(RecvWaiter{h, &value, &ok, nullptr, nullptr, -1, true});
      scheduler().parked++;
    }
    std::pair<T, bool> await_resume() { return {std::move(value), ok}; }
  };

  SendAwaiter send(T v) { return SendAwaiter{this, std::move(v)}; }
  RecvAwaiter recv() { return RecvAwaiter{this}; }
  RecvOkAwaiter recv_ok() { return RecvOkAwaiter{this}; }

  void close() {
    if (!st) panic("close of nil channel");
    if (st->closed) panic("close of closed channel");
    st->closed = true;
    while (!st->recvs.empty()) {
      RecvWaiter w = std::move(st->recvs.front());
      st->recvs.pop_front();
      if (is_cancelled(w.cancelled)) continue;
      complete_recv(w, T{}, false);
    }
    while (!st->sends.empty()) {
      SendWaiter w = std::move(st->sends.front());
      st->sends.pop_front();
      if (is_cancelled(w.cancelled)) continue;
      complete_send(w);
    }
  }

  void park_recv(std::coroutine_handle<> h, T* slot, bool* ok, std::shared_ptr<bool> cancelled,
                 std::shared_ptr<int> winner, int idx) {
    st->recvs.push_back(
        RecvWaiter{h, slot, ok, std::move(cancelled), std::move(winner), idx, /*counted=*/false});
  }
  void park_send(std::coroutine_handle<> h, T value, std::shared_ptr<bool> cancelled,
                 std::shared_ptr<int> winner, int idx) {
    st->sends.push_back(SendWaiter{h, std::move(value), std::move(cancelled), std::move(winner), idx,
                                  /*counted=*/false});
  }
};

template<class T>
bool is_nil(const Chan<T>& c) {
  return c.is_nil();
}
template<class T>
int64_t len(const Chan<T>& c) {
  return c.st ? static_cast<int64_t>(c.st->buf.size()) : 0;
}
template<class T>
int64_t cap(const Chan<T>& c) {
  return c.st ? static_cast<int64_t>(c.st->cap) : 0;
}
template<class T>
Chan<T> make_chan(int64_t n = 0) {
  if (n < 0) panic("make: negative chan cap");
  return Chan<T>(static_cast<std::size_t>(n));
}
template<class T>
void close(Chan<T>& c) {
  c.close();
}

// ---- Select -----------------------------------------------------------------
// Returns the index of the chosen case. Bodies stay in the caller so
// return/break mean what they meant in Go. Losing cases are cancelled on the
// channel waiter lists and cannot steal later values.

class GSelect {
 public:
  struct Case {
    virtual ~Case() = default;
    virtual bool try_now() = 0;
    virtual void park(std::coroutine_handle<> h, std::shared_ptr<bool> cancel,
                      std::shared_ptr<int> winner) = 0;
    bool is_default = false;
  };

  template<class T>
  struct RecvCase : Case {
    Chan<T>* ch;
    T* dst;
    bool* ok;
    int idx;
    RecvCase(Chan<T>* c, T* d, bool* o, int i) : ch(c), dst(d), ok(o), idx(i) {}
    bool try_now() override { return ch->try_recv(dst, ok); }
    void park(std::coroutine_handle<> h, std::shared_ptr<bool> cancel,
              std::shared_ptr<int> winner) override {
      ch->park_recv(h, dst, ok, std::move(cancel), std::move(winner), idx);
    }
  };

  template<class T>
  struct SendCase : Case {
    Chan<T>* ch;
    T value;
    int idx;
    SendCase(Chan<T>* c, T v, int i) : ch(c), value(std::move(v)), idx(i) {}
    bool try_now() override { return ch->try_send(value); }
    void park(std::coroutine_handle<> h, std::shared_ptr<bool> cancel,
              std::shared_ptr<int> winner) override {
      ch->park_send(h, std::move(value), std::move(cancel), std::move(winner), idx);
    }
  };

  struct DefaultCase : Case {
    DefaultCase() { is_default = true; }
    bool try_now() override { return true; }
    void park(std::coroutine_handle<>, std::shared_ptr<bool>, std::shared_ptr<int>) override {}
  };

  std::vector<std::unique_ptr<Case>> cases;

  template<class T>
  GSelect& recv(Chan<T>& ch, T* dst, bool* ok = nullptr) {
    const int idx = static_cast<int>(cases.size());
    cases.push_back(std::make_unique<RecvCase<T>>(&ch, dst, ok, idx));
    return *this;
  }

  template<class T>
  GSelect& send(Chan<T>& ch, T value) {
    const int idx = static_cast<int>(cases.size());
    cases.push_back(std::make_unique<SendCase<T>>(&ch, std::move(value), idx));
    return *this;
  }

  GSelect& deflt() {
    cases.push_back(std::make_unique<DefaultCase>());
    return *this;
  }

  struct Awaiter {
    std::vector<std::unique_ptr<Case>> cases;
    std::shared_ptr<int> winner = std::make_shared<int>(-1);
    int result = -1;
    bool did_park = false;

    bool await_ready() {
      for (int i = 0; i < static_cast<int>(cases.size()); ++i) {
        if (cases[static_cast<std::size_t>(i)]->is_default) continue;
        if (cases[static_cast<std::size_t>(i)]->try_now()) {
          result = i;
          return true;
        }
      }
      for (int i = 0; i < static_cast<int>(cases.size()); ++i) {
        if (cases[static_cast<std::size_t>(i)]->is_default) {
          result = i;
          return true;
        }
      }
      return false;
    }

    void await_suspend(std::coroutine_handle<> h) {
      did_park = true;
      auto cancel = std::make_shared<bool>(false);
      scheduler().parked++;
      for (auto& cs : cases) {
        if (cs->is_default) continue;
        cs->park(h, cancel, winner);
      }
    }

    int await_resume() {
      if (did_park) scheduler().parked--;
      if (result >= 0) return result;
      return *winner;
    }
  };

  Awaiter operator co_await() { return Awaiter{std::move(cases)}; }
};
#endif  // WASIGO_NEED_CORO

}  // namespace wasigo
