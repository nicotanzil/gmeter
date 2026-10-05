package metrics

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestHistogramQuantilesStayWithinDocumentedError(t *testing.T) {
	var h Histogram
	values := make([]time.Duration, 1000)
	for i := range values {
		values[i] = time.Duration(math.Exp(float64(i) / 999 * math.Log(float64(time.Hour))))
		h.Record(values[i])
	}
	for _, q := range []float64{.01, .5, .95, .99, 1} {
		want := values[int(math.Ceil(q*float64(len(values))))-1]
		got := h.quantile(q)
		if got < want || float64(got) > float64(want)*1.01+1 {
			t.Errorf("q=%v: got %v, exact %v", q, got, want)
		}
	}
	s := h.snapshot()
	if s.Samples != 1000 || s.MinMS != float64(values[0])/1e6 || s.MaxMS != float64(values[999])/1e6 {
		t.Fatalf("incorrect aggregate: %+v", s)
	}
}

func TestCollectorConcurrentAccountingAndSnapshotIsolation(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Start()
			s := Sample{Status: 200, Bytes: 10, Latency: time.Millisecond}
			if i%2 == 0 {
				s.Status = 503
				s.Error = "http_status"
			}
			c.Complete(s)
		}(i)
	}
	wg.Wait()
	c.Drop(5, false)
	c.Drop(5, true)
	s := c.Snapshot(time.Second)
	if s.Scheduled != 50 || s.Started != 40 || s.Completed != 40 || s.InFlight != 0 || s.BytesRead != 400 || s.ErrorPercent != 50 || s.DropPercent != 20 || s.StartedRPS != 40 {
		t.Fatalf("incorrect accounting: %+v", s)
	}
	s.Statuses[200] = 999
	if c.Snapshot(time.Second).Statuses[200] != 20 {
		t.Fatal("snapshot aliases collector maps")
	}
}
