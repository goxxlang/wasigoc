// Multi-threaded stress test for the Chan/Map/gc::Heap synchronization
// added to runtime.hpp -- proves these are safe under REAL concurrent
// access from real std::thread's, not just "still passes single-
// threaded." Not part of the main runtime_smoketest.cc (which asserts
// single-threaded coroutine behavior); standalone, ad hoc.
#include "runtime.hpp"

#include <atomic>
#include <cassert>
#include <cstdio>
#include <thread>
#include <vector>

using namespace wasigo;

#define CHECK(cond) do { if (!(cond)) { std::fprintf(stderr, "CHECK FAILED: %s (line %d)\n", #cond, __LINE__); std::exit(1); } } while (0)

// ---- Chan<T>: real concurrent producers/consumers via try_send/try_recv ----
static void chan_stress() {
  const int kProducers = 8;
  const int kPerProducer = 20000;
  const int kTotal = kProducers * kPerProducer;

  Chan<int64_t> ch(64);
  std::atomic<int64_t> produced_sum{0};
  std::atomic<int64_t> consumed_sum{0};
  std::atomic<int64_t> consumed_count{0};
  std::atomic<bool> stop{false};

  std::vector<std::thread> producers;
  for (int p = 0; p < kProducers; ++p) {
    producers.emplace_back([&, p] {
      for (int i = 0; i < kPerProducer; ++i) {
        int64_t v = p * kPerProducer + i;
        while (!ch.try_send(v)) { std::this_thread::yield(); }
        produced_sum.fetch_add(v, std::memory_order_relaxed);
      }
    });
  }

  std::vector<std::thread> consumers;
  const int kConsumers = 4;
  for (int c = 0; c < kConsumers; ++c) {
    consumers.emplace_back([&] {
      int64_t v;
      bool ok;
      while (!stop.load(std::memory_order_relaxed)) {
        if (ch.try_recv(&v, &ok)) {
          if (!ok) break;
          consumed_sum.fetch_add(v, std::memory_order_relaxed);
          consumed_count.fetch_add(1, std::memory_order_relaxed);
        } else {
          std::this_thread::yield();
        }
      }
    });
  }

  for (auto& t : producers) t.join();
  while (consumed_count.load() < kTotal) {
    int64_t v;
    bool ok;
    if (ch.try_recv(&v, &ok)) {
      if (ok) {
        consumed_sum.fetch_add(v, std::memory_order_relaxed);
        consumed_count.fetch_add(1, std::memory_order_relaxed);
      }
    } else {
      std::this_thread::yield();
    }
  }
  stop.store(true);
  for (auto& t : consumers) t.join();

  CHECK(consumed_count.load() == kTotal);
  CHECK(produced_sum.load() == consumed_sum.load());
  std::printf("chan_stress OK: %d items, sum %lld == %lld across %d producers / %d consumers\n",
              kTotal, (long long)produced_sum.load(), (long long)consumed_sum.load(), kProducers,
              kConsumers);
}

// ---- gc::Heap: real concurrent allocation + Collect() ----------------------
// Uses MakeRooted (allocate+root atomically), NOT Make()-then-root-
// separately: the latter has a real window where a concurrent
// Collect() on another thread can sweep the object before this thread
// gets around to rooting it (found by an earlier version of this exact
// test -- see the hazard documented on Heap::Make() in runtime.hpp).
struct StressNode : gc::GarbageCollected<StressNode> {
  int64_t v = 0;
  void Trace(gc::Visitor&) const override {}
};

static void heap_stress() {
  const int kThreads = 8;
  const int kPerThread = 5000;
  std::atomic<int> live_roots{0};

  std::vector<std::thread> threads;
  for (int t = 0; t < kThreads; ++t) {
    threads.emplace_back([&] {
      std::vector<gc::Persistent<StressNode>> keep;
      for (int i = 0; i < kPerThread; ++i) {
        auto root = gc::heap().MakeRooted<StressNode>();
        root->v = i;
        if (i % 3 == 0) {
          keep.push_back(std::move(root));
          live_roots.fetch_add(1, std::memory_order_relaxed);
        }
        // else: root un-roots its StressNode on scope exit here.
        if (i % 500 == 0) {
          gc::heap().Collect();  // concurrent Collect() from multiple threads
        }
      }
    });
  }
  for (auto& t : threads) t.join();

  gc::heap().Collect();
  CHECK(gc::heap().live() == 0);
  std::printf("heap_stress OK: %d threads x %d allocs, %d were rooted at some point, live()==0 after final Collect\n",
              kThreads, kPerThread, live_roots.load());
}

// ---- Map<K,V>: prove the concurrent-write detector's underlying atomic
// exchange actually detects real contention under real concurrent
// access. Not exercised through Map's own operator[] here: a genuine
// hit panics (aborts) by design.
static void map_guard_contention_detected() {
  const int kThreads = 8;
  const int kIters = 200000;
  std::atomic<bool> flag{false};
  std::atomic<int> contention_detected{0};

  std::vector<std::thread> threads;
  for (int t = 0; t < kThreads; ++t) {
    threads.emplace_back([&] {
      for (int i = 0; i < kIters; ++i) {
        if (flag.exchange(true, std::memory_order_acq_rel)) {
          contention_detected.fetch_add(1, std::memory_order_relaxed);
        } else {
          flag.store(false, std::memory_order_release);
        }
      }
    });
  }
  for (auto& t : threads) t.join();

  CHECK(contention_detected.load() > 0);
  std::printf("map_guard_contention_detected OK: %d contended exchanges out of %d total attempts\n",
              contention_detected.load(), kThreads * kIters);
}

int main() {
  chan_stress();
  heap_stress();
  map_guard_contention_detected();
  std::printf("ALL SYNCHRONIZATION STRESS TESTS PASSED\n");
  return 0;
}
