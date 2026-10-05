package telemetry

import (
	"fmt"
	"maps"
	"math"
	"time"
)

// Validate stored observations without contacting a target. Importing does not
// verify the original server measurements or load traffic.
func ValidateSummary(c *Config, s *Summary, start time.Time, total float64) error {
	bad := func() error {
		return fmt.Errorf("target telemetry does not match configuration or has invalid observations")
	}
	if c == nil {
		if s != nil {
			return bad()
		}
		return nil
	}
	if s == nil {
		return bad()
	}
	p, err := c.Compile()
	if err != nil {
		return err
	}
	if s.CanceledScrapes < 0 || s.CanceledScrapes > 1 {
		return bad()
	}
	if s.URL != p.URL || s.IntervalSeconds != p.Every.Seconds() || s.TimeoutSeconds != p.Limit.Seconds() || s.Attempts < 0 || s.Attempts > 10000000 || s.FailedScrapes < 0 || s.IncompleteScrapes < 0 || s.FailedScrapes+s.IncompleteScrapes > s.Attempts || len(s.Series) != len(p.Metrics) || len(s.Timeline) != min(s.Attempts, MaxPoints) || s.Truncated != (s.Attempts > MaxPoints) || s.Complete != (s.Attempts > 0 && s.FailedScrapes == 0 && s.IncompleteScrapes == 0) {
		return bad()
	}
	missing := 0
	for i, series := range s.Series {
		m := p.Metrics[i]
		if series.Metric.Name != m.Name || series.Metric.Kind != m.Kind || series.Metric.Title != m.Title || series.Metric.Unit != m.Unit || !maps.Equal(series.Metric.Labels, m.Labels) || series.Samples < 0 || series.Missing < s.FailedScrapes || series.Samples+series.Missing != s.Attempts || series.Resets < 0 || series.Resets > max(0, series.Samples-1) || !finite(series.CoveredSeconds) || series.CoveredSeconds < 0 || series.CoveredSeconds > total {
			return bad()
		}
		missing += series.Missing
		if !finite(series.FirstElapsedSeconds) || !finite(series.LastElapsedSeconds) || series.FirstElapsedSeconds < 0 || series.LastElapsedSeconds < series.FirstElapsedSeconds || series.LastElapsedSeconds > total || (series.Samples == 0 && (series.FirstElapsedSeconds != 0 || series.LastElapsedSeconds != 0)) {
			return bad()
		}
		for _, value := range []*float64{series.First, series.Last, series.Min, series.Max, series.Mean} {
			if (series.Samples == 0) != (value == nil) || (value != nil && (!finite(*value) || math.Abs(*value) > 1e100)) {
				return bad()
			}
		}
		if series.Samples > 0 && (*series.Min > *series.Max || *series.First < *series.Min || *series.First > *series.Max || *series.Last < *series.Min || *series.Last > *series.Max || *series.Mean < *series.Min-1e-9*math.Abs(*series.Min) || *series.Mean > *series.Max+1e-9*math.Abs(*series.Max) || (series.Metric.Kind == "counter" && *series.Min < 0)) {
			return bad()
		}
		if series.Metric.Kind == "gauge" {
			if series.Resets != 0 || series.ObservedIncrease != nil || series.MeanRate != nil || series.CoveredSeconds != 0 {
				return bad()
			}
		} else {
			if (series.CoveredSeconds == 0) != (series.ObservedIncrease == nil) || (series.CoveredSeconds == 0) != (series.MeanRate == nil) {
				return bad()
			}
			if series.ObservedIncrease != nil && (!finite(*series.ObservedIncrease) || *series.ObservedIncrease < 0 || !finite(*series.MeanRate) || *series.MeanRate < 0 || math.Abs(*series.MeanRate-*series.ObservedIncrease/series.CoveredSeconds) > 1e-9*max(1, *series.MeanRate)) {
				return bad()
			}
		}
	}
	if missing < s.FailedScrapes*len(s.Series)+s.IncompleteScrapes {
		return bad()
	}
	previousElapsed := -1.0
	for i, point := range s.Timeline {
		if err := ValidateSnapshot(point, p.Metrics, start, total); err != nil || point.Sequence != s.Attempts-len(s.Timeline)+i+1 || point.ElapsedSeconds < previousElapsed {
			return bad()
		}
		previousElapsed = point.ElapsedSeconds
	}
	return nil
}

func ValidateSnapshot(p Snapshot, metrics []Metric, start time.Time, total float64) error {
	bad := func() error { return fmt.Errorf("invalid target telemetry timeline") }
	// Elapsed time bounds are authoritative. UTC wall time can move when the
	// system clock adjusts; it must not invalidate monotonic run observations.
	if start.IsZero() || p.Sequence < 1 || p.Sequence > 10000000 || !finite(p.ElapsedSeconds) || p.ElapsedSeconds < 0 || p.ElapsedSeconds > total || p.ObservedAt.IsZero() || !finite(p.DurationMS) || p.DurationMS < 0 || p.DurationMS > p.ElapsedSeconds*1000+1 || len(p.Error) > 256 || len(p.Values) != len(metrics) {
		return bad()
	}
	for i, v := range p.Values {
		if len(v.Issue) > 128 || (v.Value == nil) != (v.Issue != "") || (p.Error != "" && v.Value != nil) {
			return bad()
		}
		if v.Value == nil {
			if v.Rate != nil || v.Reset {
				return bad()
			}
			continue
		}
		if !finite(*v.Value) || math.Abs(*v.Value) > 1e100 || (metrics[i].Kind == "counter" && *v.Value < 0) || (metrics[i].Kind == "gauge" && (v.Rate != nil || v.Reset)) {
			return bad()
		}
		if v.Rate != nil && (!finite(*v.Rate) || *v.Rate < 0 || v.Reset) {
			return bad()
		}
	}
	return nil
}
