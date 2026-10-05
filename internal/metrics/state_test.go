package metrics

import (
	"testing"
	"time"
)

func TestMergeUsesBucketsAndSampleWeights(t *testing.T) {
	a, b := New(), New()
	a.Start()
	a.Complete(Sample{Status: 200, Latency: 100 * time.Millisecond, ScheduledLatency: 100 * time.Millisecond})
	for range 19 {
		b.Start()
		b.Complete(Sample{Status: 200, Latency: time.Millisecond, ScheduledLatency: time.Millisecond})
	}
	b.Drop(3, true)
	merged, err := Merge([]State{a.State(), b.State()})
	if err != nil {
		t.Fatal(err)
	}
	s := merged.Snapshot(time.Second)
	if s.Scheduled != 23 || s.Completed != 20 || s.DroppedScheduler != 3 || s.Latency.Samples != 20 || s.Latency.P95MS > 1.01 || s.Latency.P99MS != 100 || s.Latency.MeanMS != 5.95 {
		t.Fatalf("global population was not merged correctly: %+v", s)
	}
	state := a.State()
	state.Counts.Statuses[200] = 999
	if a.State().Counts.Statuses[200] != 1 {
		t.Fatal("exported state aliases live maps")
	}
}

func TestInvalidWorkerHistogramsAreRejected(t *testing.T) {
	c := New()
	c.Start()
	c.Complete(Sample{Error: "timeout", Latency: time.Millisecond, ScheduledLatency: time.Millisecond})
	for _, change := range []func(*State){
		func(s *State) { s.Latency.Count++ },
		func(s *State) { s.Latency.Buckets[0].Index = -1 },
		func(s *State) { s.Latency.Buckets[0].Index = 0 },
		func(s *State) { s.Counts.Scheduled++ },
		func(s *State) { s.Counts.Errors["timeout"] = -1 },
		func(s *State) { s.HTTP = &HTTPState{} },
	} {
		s := c.State()
		change(&s)
		if _, err := Merge([]State{s}); err == nil {
			t.Fatal("invalid worker state accepted")
		}
	}
}
