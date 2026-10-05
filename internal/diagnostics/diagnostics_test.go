package diagnostics

import (
	"runtime"
	"testing"
	"time"
)

func TestSamplerUsesPerRunCPUDeltasWithoutClippingCores(t *testing.T) {
	start := time.Now()
	i := 0
	values := []reading{
		{at: start, cpuSeconds: 40, cpuAvailable: true, mem: runtime.MemStats{HeapAlloc: 100, Sys: 1000, NumGC: 3, PauseTotalNs: 2000000}, goroutines: 2},
		{at: start.Add(time.Second), cpuSeconds: 42, cpuAvailable: true, mem: runtime.MemStats{HeapAlloc: 300, Sys: 1500, NumGC: 4, PauseTotalNs: 5000000}, goroutines: 10},
		{at: start.Add(2 * time.Second), cpuSeconds: 43, cpuAvailable: true, mem: runtime.MemStats{HeapAlloc: 200, Sys: 1200, NumGC: 4, PauseTotalNs: 6000000}, goroutines: 3},
	}
	s := newSampler(func() reading { value := values[i]; i++; return value })
	current := s.Sample()
	if current.CPUPercent != 200 || current.CPUSeconds != 2 || current.GCCycles != 1 || current.GCPauseMS != 3 {
		t.Fatalf("bad live deltas %+v", current)
	}
	final := s.Finish()
	if final.AverageCPUPercent != 150 || final.PeakCPUPercent != 200 || final.CPUSeconds != 3 || final.PeakHeapBytes != 300 || final.PeakGoroutines != 10 || final.Samples != 3 || final.Final.GCPauseMS != 4 {
		t.Fatalf("bad final deltas %+v", final)
	}
}

func TestUnavailableCPUIsExplicitAndRuntimeMetricsRemain(t *testing.T) {
	i := 0
	at := time.Now()
	s := newSampler(func() reading {
		i++
		return reading{at: at.Add(time.Duration(i) * time.Second), mem: runtime.MemStats{HeapAlloc: 100}, goroutines: 1}
	})
	final := s.Finish()
	if final.CPUAvailable || final.Final.CPUAvailable || final.Final.RSSAvailable || final.PeakHeapBytes != 100 {
		t.Fatalf("unsupported counter handling %+v", final)
	}
}

func TestLiveProcessSampling(t *testing.T) {
	s := New()
	final := s.Finish()
	if final.Final.HeapBytes == 0 || final.Final.RuntimeBytes == 0 || final.Final.Goroutines < 1 || final.LogicalCPUs < 1 || final.GoVersion == "" {
		t.Fatalf("missing process metrics %+v", final)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if !final.CPUAvailable || !final.Final.RSSAvailable || final.Final.ProcessPeakRSSBytes == 0 {
			t.Fatalf("missing OS metrics %+v", final)
		}
	}
}
