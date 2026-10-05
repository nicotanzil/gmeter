package distributed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gmeter/internal/telemetry"
)

func TestOnlyCoordinatorCollectsTargetTelemetry(t *testing.T) {
	wa, a := testWorker(t, "A", testToken)
	wb, b := testWorker(t, "B", testToken)
	var attempts, scrapes atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			scrapes.Add(1)
			if r.Header.Get("Authorization") != "Bearer telemetry-only-secret" {
				http.Error(w, "unauthenticated", 401)
				return
			}
			w.Header().Set("Content-Type", "text/plain;version=0.0.4")
			fmt.Fprintf(w, "jobs_total %d\n", attempts.Load())
			return
		}
		attempts.Add(1)
		fmt.Fprint(w, "ok")
	}))
	defer target.Close()
	c := rateConfig(target.URL, a.URL, b.URL)
	c.Telemetry = &telemetry.Config{URL: target.URL + "/metrics", Interval: "250ms", BearerAuth: true, Metrics: []telemetry.Metric{{Name: "jobs_total", Kind: "counter"}}}
	opts := testOptions()
	opts.Telemetry.Token = "telemetry-only-secret"
	r, err := Run(context.Background(), c, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Passed || r.SchemaVersion != 5 || r.Completed != 4 || !r.Distribution.Complete || !r.Telemetry.Complete || (int(scrapes.Load()) < r.Telemetry.Attempts || int(scrapes.Load()) > r.Telemetry.Attempts+r.Telemetry.CanceledScrapes) || r.HTTP.FirstByte.Samples != 4 {
		t.Fatalf("fleet telemetry/accounting: %+v", r)
	}
	for _, w := range []*Worker{wa, wb} {
		w.mu.Lock()
		hasTelemetry := w.job.config.Telemetry != nil || w.job.Result.Telemetry != nil
		w.mu.Unlock()
		if hasTelemetry {
			t.Fatal("coordinator telemetry forwarded to a worker")
		}
	}
	if err := telemetry.ValidateSummary(c.Telemetry, r.Telemetry, r.StartedAt, r.TotalSeconds); err != nil {
		t.Fatal(err)
	}
}
