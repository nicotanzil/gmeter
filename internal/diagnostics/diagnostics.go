// Package diagnostics samples the generator process, including dashboard work
// in that process. It does not infer target-server health or generator capacity.
package diagnostics

import (
	"runtime"
	"sync"
	"time"
)

type Resources struct {
	ElapsedSeconds      float64 `json:"elapsed_seconds"`
	CPUAvailable        bool    `json:"cpu_available"`
	CPUSeconds          float64 `json:"cpu_seconds"`
	CPUPercent          float64 `json:"cpu_percent"`
	RSSAvailable        bool    `json:"rss_available"`
	ProcessPeakRSSBytes uint64  `json:"process_peak_rss_bytes"`
	HeapBytes           uint64  `json:"heap_bytes"`
	RuntimeBytes        uint64  `json:"runtime_bytes"`
	Goroutines          int     `json:"goroutines"`
	GOMAXPROCS          int     `json:"gomaxprocs"`
	GCCycles            uint32  `json:"gc_cycles"`
	GCPauseMS           float64 `json:"gc_pause_ms"`
}

type Summary struct {
	Platform          string    `json:"platform"`
	GoVersion         string    `json:"go_version"`
	LogicalCPUs       int       `json:"logical_cpus"`
	Samples           int       `json:"samples"`
	CPUAvailable      bool      `json:"cpu_available"`
	CPUSeconds        float64   `json:"cpu_seconds"`
	AverageCPUPercent float64   `json:"average_cpu_percent"`
	PeakCPUPercent    float64   `json:"peak_cpu_percent"`
	PeakHeapBytes     uint64    `json:"peak_heap_bytes"`
	PeakRuntimeBytes  uint64    `json:"peak_runtime_bytes"`
	PeakGoroutines    int       `json:"peak_goroutines"`
	Initial           Resources `json:"initial"`
	Final             Resources `json:"final"`
}

type reading struct {
	at                         time.Time
	cpuSeconds                 float64
	cpuAvailable, rssAvailable bool
	peakRSS                    uint64
	mem                        runtime.MemStats
	goroutines, gomaxprocs     int
}

type Sampler struct {
	mu                sync.Mutex
	initial, previous reading
	summary           Summary
	read              func() reading
}

func readProcess() reading {
	r := reading{at: time.Now(), goroutines: runtime.NumGoroutine(), gomaxprocs: runtime.GOMAXPROCS(0)}
	r.cpuSeconds, r.peakRSS, r.cpuAvailable, r.rssAvailable = processUsage()
	runtime.ReadMemStats(&r.mem)
	return r
}

func New() *Sampler { return newSampler(readProcess) }
func newSampler(read func() reading) *Sampler {
	initial := read()
	s := &Sampler{initial: initial, previous: initial, read: read, summary: Summary{Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), LogicalCPUs: runtime.NumCPU(), CPUAvailable: initial.cpuAvailable}}
	s.summary.Initial = s.resource(initial)
	s.update(s.summary.Initial)
	return s
}

func (s *Sampler) resource(r reading) Resources {
	value := Resources{ElapsedSeconds: r.at.Sub(s.initial.at).Seconds(), CPUAvailable: r.cpuAvailable && s.initial.cpuAvailable && s.previous.cpuAvailable, RSSAvailable: r.rssAvailable, ProcessPeakRSSBytes: r.peakRSS, HeapBytes: r.mem.HeapAlloc, RuntimeBytes: r.mem.Sys, Goroutines: r.goroutines, GOMAXPROCS: r.gomaxprocs, GCCycles: r.mem.NumGC - s.initial.mem.NumGC, GCPauseMS: float64(r.mem.PauseTotalNs-s.initial.mem.PauseTotalNs) / 1e6}
	if value.CPUAvailable {
		value.CPUSeconds = max(0, r.cpuSeconds-s.initial.cpuSeconds)
		if seconds := r.at.Sub(s.previous.at).Seconds(); seconds > 0 {
			value.CPUPercent = 100 * max(0, r.cpuSeconds-s.previous.cpuSeconds) / seconds
		}
	}
	return value
}
func (s *Sampler) update(value Resources) {
	s.summary.Samples++
	s.summary.CPUAvailable = s.summary.CPUAvailable && value.CPUAvailable
	s.summary.PeakCPUPercent = max(s.summary.PeakCPUPercent, value.CPUPercent)
	s.summary.PeakHeapBytes = max(s.summary.PeakHeapBytes, value.HeapBytes)
	s.summary.PeakRuntimeBytes = max(s.summary.PeakRuntimeBytes, value.RuntimeBytes)
	s.summary.PeakGoroutines = max(s.summary.PeakGoroutines, value.Goroutines)
	s.summary.Final = value
}
func (s *Sampler) Sample() Resources {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.read()
	value := s.resource(r)
	s.update(value)
	s.previous = r
	return value
}
func (s *Sampler) Finish() Summary {
	s.Sample()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summary.CPUAvailable {
		s.summary.CPUSeconds = s.summary.Final.CPUSeconds
		if s.summary.Final.ElapsedSeconds > 0 {
			s.summary.AverageCPUPercent = 100 * s.summary.CPUSeconds / s.summary.Final.ElapsedSeconds
		}
	}
	return s.summary
}
