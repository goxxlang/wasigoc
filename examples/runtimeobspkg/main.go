package main

import (
	"bytes"
	"fmt"
	"runtime/coverage"
	"runtime/metrics"
	"runtime/pprof"
	"runtime/trace"
)

func main() {
	fmt.Println(len(metrics.All()) == 0)
	samples := make([]metrics.Sample, 1)
	samples[0].Name = "/nonexistent"
	metrics.Read(samples)
	fmt.Println(samples[0].Value.Kind() == metrics.KindBad)

	var buf bytes.Buffer
	fmt.Println(pprof.StartCPUProfile(&buf) != nil)
	pprof.StopCPUProfile()
	fmt.Println(pprof.WriteHeapProfile(&buf) != nil)

	p := pprof.NewProfile("custom")
	fmt.Println(p.Name() == "custom")
	fmt.Println(p.Count() == 0)
	fmt.Println(pprof.Lookup("custom") == p)
	fmt.Println(pprof.Lookup("nope") == nil)
	fmt.Println(p.WriteTo(&buf, 0) != nil)

	fmt.Println(trace.Start(&buf) != nil)
	trace.Stop()

	fmt.Println(coverage.WriteMeta(&buf) != nil)
	fmt.Println(coverage.WriteCounters(&buf) != nil)
	fmt.Println(coverage.ClearCounters() != nil)
}
