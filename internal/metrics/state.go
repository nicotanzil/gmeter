package metrics

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

// State carries histogram buckets, never rounded percentiles. Sparse buckets
// keep worker messages bounded without allocating a sample per response.
type Bucket struct {
	Index int   `json:"index"`
	Count int64 `json:"count"`
}
type HistogramState struct {
	Buckets []Bucket `json:"buckets"`
	Count   int64    `json:"count"`
	SumNS   float64  `json:"sum_ns"`
	MinNS   int64    `json:"min_ns"`
	MaxNS   int64    `json:"max_ns"`
}
type HTTPState struct {
	Counts  HTTPMetrics      `json:"counts"`
	Timings []HistogramState `json:"timings"`
}
type State struct {
	Counts           Counts         `json:"counts"`
	Latency          HistogramState `json:"latency"`
	ScheduledLatency HistogramState `json:"scheduled_latency"`
	StartDelay       HistogramState `json:"start_delay"`
	HTTP             *HTTPState     `json:"http,omitempty"`
}

const maxMetricCount = int64(1e14)

func (h *Histogram) state() HistogramState {
	s := HistogramState{Count: h.count, SumNS: h.sum, MinNS: int64(h.min), MaxNS: int64(h.max)}
	for i, count := range h.buckets {
		if count > 0 {
			s.Buckets = append(s.Buckets, Bucket{i, count})
		}
	}
	return s
}
func (s HistogramState) validate() error {
	if s.Count < 0 || s.Count > maxMetricCount || len(s.Buckets) > bucketCount || s.MinNS < 0 || s.MaxNS < s.MinNS || math.IsNaN(s.SumNS) || math.IsInf(s.SumNS, 0) || s.SumNS < 0 {
		return fmt.Errorf("invalid histogram metadata")
	}
	var count int64
	previous := -1
	index := func(ns int64) int {
		if ns <= 1 {
			return 0
		}
		return min(bucketCount-1, int(math.Ceil(math.Log(float64(ns))/logBase)))
	}
	for _, bucket := range s.Buckets {
		if bucket.Index <= previous || bucket.Index >= bucketCount || bucket.Index < index(s.MinNS) || bucket.Index > index(s.MaxNS) || bucket.Count <= 0 || bucket.Count > maxMetricCount-count {
			return fmt.Errorf("invalid histogram buckets")
		}
		count += bucket.Count
		previous = bucket.Index
	}
	if count != s.Count || (count == 0 && (s.SumNS != 0 || s.MinNS != 0 || s.MaxNS != 0)) || (count > 0 && (s.SumNS < float64(s.MinNS)*float64(count)*(1-1e-9) || s.SumNS > float64(s.MaxNS)*float64(count)*(1+1e-9))) {
		return fmt.Errorf("histogram counts or bounds disagree")
	}
	return nil
}
func (h *Histogram) merge(s HistogramState) {
	if s.Count == 0 {
		return
	}
	if h.count == 0 || time.Duration(s.MinNS) < h.min {
		h.min = time.Duration(s.MinNS)
	}
	h.max = max(h.max, time.Duration(s.MaxNS))
	h.count += s.Count
	h.sum += s.SumNS
	for _, bucket := range s.Buckets {
		h.buckets[bucket.Index] += bucket.Count
	}
}

func copyCounts(n Counts) Counts {
	n.Statuses = copyMap(n.Statuses)
	n.Errors = copyMap(n.Errors)
	return n
}
func copyMap[K comparable](in map[K]int64) map[K]int64 {
	out := make(map[K]int64, len(in))
	for k, value := range in {
		out[k] = value
	}
	return out
}
func (c *Collector) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := State{Counts: copyCounts(c.counts), Latency: c.latency.state(), ScheduledLatency: c.scheduledLatency.state(), StartDelay: c.startDelay.state()}
	if h := c.http; h != nil {
		counts := h.counts
		counts.Protocols = copyMap(counts.Protocols)
		s.HTTP = &HTTPState{Counts: counts, Timings: []HistogramState{h.wait.state(), h.dns.state(), h.connect.state(), h.tls.state(), h.write.state(), h.firstByte.state(), h.body.state()}}
	}
	return s
}

func (s State) Validate() error {
	n := s.Counts
	for _, v := range []int64{n.Scheduled, n.Started, n.Completed, n.Succeeded, n.Failed, n.DroppedCapacity, n.DroppedScheduler, n.InFlight, n.PeakInFlight, n.BytesRead} {
		if v < 0 || v > maxMetricCount {
			return fmt.Errorf("invalid metric count")
		}
	}
	if n.Scheduled != n.Started+n.DroppedCapacity+n.DroppedScheduler || n.Started != n.Completed+n.InFlight || n.Completed != n.Succeeded+n.Failed || n.PeakInFlight < n.InFlight || len(n.Errors) > 64 || len(n.Statuses) > 500 {
		return fmt.Errorf("metric accounting disagrees")
	}
	var errors, statuses int64
	for key, count := range n.Errors {
		if len(key) > 64 || count < 0 || count > n.Failed-errors {
			return fmt.Errorf("invalid error counts")
		}
		errors += count
	}
	for key, count := range n.Statuses {
		if key < 100 || key > 599 || count < 0 || count > n.Completed-statuses {
			return fmt.Errorf("invalid status counts")
		}
		statuses += count
	}
	if errors != n.Failed {
		return fmt.Errorf("error counts disagree")
	}
	for _, h := range []HistogramState{s.Latency, s.ScheduledLatency, s.StartDelay} {
		if err := h.validate(); err != nil {
			return err
		}
		if h.Count != n.Completed {
			return fmt.Errorf("latency samples disagree with completed count")
		}
	}
	if s.HTTP != nil {
		if len(s.HTTP.Timings) != 7 || len(s.HTTP.Counts.Protocols) > 4 {
			return fmt.Errorf("invalid HTTP timing shape")
		}
		v := reflect.ValueOf(s.HTTP.Counts)
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.Kind() == reflect.Int64 && (f.Int() < 0 || f.Int() > maxMetricCount) {
				return fmt.Errorf("invalid HTTP event count")
			}
		}
		for _, h := range s.HTTP.Timings {
			if err := h.validate(); err != nil {
				return err
			}
		}
		for protocol, count := range s.HTTP.Counts.Protocols {
			if (protocol != "HTTP/1.0" && protocol != "HTTP/1.1" && protocol != "HTTP/2.0" && protocol != "other") || count < 0 || count > maxMetricCount {
				return fmt.Errorf("invalid protocol counts")
			}
		}
	}
	return nil
}

// Merge sums worker high-water marks as a conservative upper bound. It is not
// an observed simultaneous peak; distributed reports label this distinction.
func Merge(states []State) (*Collector, error) {
	c := New()
	for _, s := range states {
		if err := s.Validate(); err != nil {
			return nil, err
		}
		n, dst := s.Counts, &c.counts
		dst.Scheduled += n.Scheduled
		dst.Started += n.Started
		dst.Completed += n.Completed
		dst.Succeeded += n.Succeeded
		dst.Failed += n.Failed
		dst.DroppedCapacity += n.DroppedCapacity
		dst.DroppedScheduler += n.DroppedScheduler
		dst.BytesRead += n.BytesRead
		dst.InFlight += n.InFlight
		dst.PeakInFlight += n.PeakInFlight
		for k, v := range n.Errors {
			dst.Errors[k] += v
		}
		for k, v := range n.Statuses {
			dst.Statuses[k] += v
		}
		c.latency.merge(s.Latency)
		c.scheduledLatency.merge(s.ScheduledLatency)
		c.startDelay.merge(s.StartDelay)
		if s.HTTP != nil {
			if c.http == nil {
				c.http = &httpHistograms{counts: HTTPMetrics{Protocols: map[string]int64{}}}
			}
			h := c.http
			a, b := reflect.ValueOf(&h.counts).Elem(), reflect.ValueOf(s.HTTP.Counts)
			for i := 0; i < a.NumField(); i++ {
				if a.Field(i).Kind() == reflect.Int64 {
					a.Field(i).SetInt(a.Field(i).Int() + b.Field(i).Int())
				}
			}
			for k, v := range s.HTTP.Counts.Protocols {
				h.counts.Protocols[k] += v
			}
			for i, hist := range []*Histogram{&h.wait, &h.dns, &h.connect, &h.tls, &h.write, &h.firstByte, &h.body} {
				hist.merge(s.HTTP.Timings[i])
			}
		}
	}
	return c, nil
}
