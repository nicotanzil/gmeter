// Package metrics stores bounded aggregates rather than every request sample.
package metrics

import (
	"math"
	"sync"
	"time"
)

const bucketCount = 4096

var logBase = math.Log(1.01)

// Histogram rounds positive durations up into logarithmic buckets. Quantiles
// have at most 1% bucket error (plus nanosecond rounding); Min/Max/Mean are exact.
type Histogram struct {
	buckets  [bucketCount]int64
	count    int64
	sum      float64
	min, max time.Duration
}

func (h *Histogram) Record(d time.Duration) {
	if d < 0 {
		d = 0
	}
	i := 0
	if d > 1 {
		i = int(math.Ceil(math.Log(float64(d)) / logBase))
	}
	if i >= bucketCount {
		i = bucketCount - 1
	}
	h.buckets[i]++
	h.count++
	h.sum += float64(d)
	if h.count == 1 || d < h.min {
		h.min = d
	}
	if d > h.max {
		h.max = d
	}
}

func (h *Histogram) quantile(q float64) time.Duration {
	if h.count == 0 {
		return 0
	}
	wanted := int64(math.Ceil(q * float64(h.count)))
	if wanted < 1 {
		wanted = 1
	}
	var n int64
	for i, count := range h.buckets {
		n += count
		if n >= wanted {
			bound := math.Ceil(math.Exp(float64(i) * logBase))
			if bound >= float64(h.max) {
				return h.max
			}
			return time.Duration(bound)
		}
	}
	return h.max
}

type Latency struct {
	Samples int64   `json:"samples"`
	MinMS   float64 `json:"min_ms"`
	MeanMS  float64 `json:"mean_ms"`
	P50MS   float64 `json:"p50_ms"`
	P95MS   float64 `json:"p95_ms"`
	P99MS   float64 `json:"p99_ms"`
	MaxMS   float64 `json:"max_ms"`
}

func (h *Histogram) snapshot() Latency {
	if h.count == 0 {
		return Latency{}
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	return Latency{h.count, ms(h.min), h.sum / float64(h.count) / 1e6, ms(h.quantile(.50)), ms(h.quantile(.95)), ms(h.quantile(.99)), ms(h.max)}
}

type Counts struct {
	Scheduled        int64            `json:"scheduled"`
	Started          int64            `json:"started"`
	Completed        int64            `json:"completed"`
	Succeeded        int64            `json:"succeeded"`
	Failed           int64            `json:"failed"`
	DroppedCapacity  int64            `json:"dropped_capacity"`
	DroppedScheduler int64            `json:"dropped_scheduler"`
	BytesRead        int64            `json:"bytes_read"`
	PeakInFlight     int64            `json:"peak_in_flight"`
	InFlight         int64            `json:"in_flight"`
	Statuses         map[int]int64    `json:"status_codes"`
	Errors           map[string]int64 `json:"errors"`
}

type Snapshot struct {
	Counts
	ErrorPercent     float64      `json:"error_percent"`
	DropPercent      float64      `json:"drop_percent"`
	StartedRPS       float64      `json:"started_rps"`
	Latency          Latency      `json:"latency"`
	ScheduledLatency Latency      `json:"scheduled_latency"`
	StartDelay       Latency      `json:"start_delay"`
	HTTP             *HTTPMetrics `json:"http,omitempty"`
}

type Sample struct {
	Status                                int
	Error                                 string
	Bytes                                 int64
	Latency, ScheduledLatency, StartDelay time.Duration
}

type Collector struct {
	mu                                    sync.Mutex
	counts                                Counts
	latency, scheduledLatency, startDelay Histogram
	http                                  *httpHistograms
}

func New() *Collector {
	return &Collector{counts: Counts{Statuses: map[int]int64{}, Errors: map[string]int64{}}}
}

func (c *Collector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts.Scheduled++
	c.counts.Started++
	c.counts.InFlight++
	if c.counts.InFlight > c.counts.PeakInFlight {
		c.counts.PeakInFlight = c.counts.InFlight
	}
}

func (c *Collector) Drop(n int64, scheduler bool) {
	if n <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts.Scheduled += n
	if scheduler {
		c.counts.DroppedScheduler += n
	} else {
		c.counts.DroppedCapacity += n
	}
}

func (c *Collector) Complete(s Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts.Completed++
	c.counts.InFlight--
	c.counts.BytesRead += s.Bytes
	if s.Status != 0 {
		c.counts.Statuses[s.Status]++
	}
	if s.Error == "" {
		c.counts.Succeeded++
	} else {
		c.counts.Failed++
		c.counts.Errors[s.Error]++
	}
	c.latency.Record(s.Latency)
	c.scheduledLatency.Record(s.ScheduledLatency)
	c.startDelay.Record(s.StartDelay)
}

func (c *Collector) Snapshot(loadDuration time.Duration) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.counts
	n.Statuses = make(map[int]int64, len(c.counts.Statuses))
	for k, v := range c.counts.Statuses {
		n.Statuses[k] = v
	}
	n.Errors = make(map[string]int64, len(c.counts.Errors))
	for k, v := range c.counts.Errors {
		n.Errors[k] = v
	}
	s := Snapshot{Counts: n, Latency: c.latency.snapshot(), ScheduledLatency: c.scheduledLatency.snapshot(), StartDelay: c.startDelay.snapshot(), HTTP: c.http.snapshot()}
	if n.Completed > 0 {
		s.ErrorPercent = 100 * float64(n.Failed) / float64(n.Completed)
	}
	if n.Scheduled > 0 {
		s.DropPercent = 100 * float64(n.DroppedCapacity+n.DroppedScheduler) / float64(n.Scheduled)
	}
	if loadDuration > 0 {
		s.StartedRPS = float64(n.Started) / loadDuration.Seconds()
	}
	return s
}
