// Bounded runtime/metrics: same honest-no-op reasoning as runtime/debug
// itself -- this runtime tracks no metrics at all (single-threaded, no
// GC/scheduler instrumentation wired up), so `All()` returning no
// descriptions and `Read` leaving every sample at its zero value is the
// correct terminal shape, not a placeholder for later. Bounded: no
// `Float64Histogram` kind (real Go's `KindFloat64Histogram` plus the
// separate `Float64Histogram` type it returns) -- every metric this
// runtime could ever produce is scalar anyway, since there's nothing
// here to histogram. `Value` stores both a uint64 and a float64 field
// directly rather than real Go's single `unsafe.Pointer`/bit-pattern
// union, since `unsafe` isn't available here.
package metrics

type ValueKind int

const KindBad ValueKind = 0
const KindUint64 ValueKind = 1
const KindFloat64 ValueKind = 2

type Value struct {
	kind ValueKind
	u    uint64
	f    float64
}

func (v Value) Kind() ValueKind {
	return v.kind
}

func (v Value) Uint64() uint64 {
	if v.kind != KindUint64 {
		panic("runtime/metrics: called Uint64 on non-uint64 metric value")
	}
	return v.u
}

func (v Value) Float64() float64 {
	if v.kind != KindFloat64 {
		panic("runtime/metrics: called Float64 on non-float64 metric value")
	}
	return v.f
}

type Description struct {
	Name        string
	Description string
	Kind        ValueKind
	Cumulative  bool
}

func All() []Description {
	return nil
}

type Sample struct {
	Name  string
	Value Value
}

func Read(m []Sample) {
	i := 0
	for i < len(m) {
		m[i].Value = Value{}
		i = i + 1
	}
}
