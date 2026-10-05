package telemetry

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(url string) Config {
	return Config{URL: url, Interval: "250ms", Timeout: "100ms", Metrics: []Metric{{Name: "queue", Kind: "gauge"}, {Name: "jobs_total", Kind: "counter", Unit: "jobs"}}}
}
func TestConfigBoundsAndSecretSetup(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.URL = "http://u:p@host/metrics" }, func(c *Config) { c.URL = "http://host/metrics?token=x" }, func(c *Config) { c.URL = "http://host/metrics#" }, func(c *Config) { c.URL = "http://host:65536/metrics" }, func(c *Config) { c.URL = "file:///metrics" }, func(c *Config) { c.BearerAuth = true; c.URL = "http://localhost/metrics" },
		func(c *Config) { c.Interval = "249ms" }, func(c *Config) { c.Timeout = "6s" }, func(c *Config) { c.Metrics = nil }, func(c *Config) { c.Metrics = append(c.Metrics, c.Metrics[0]) }, func(c *Config) { c.Metrics[0].Kind = "histogram" }, func(c *Config) { c.Metrics[0].Title = "bad\nline" }, func(c *Config) { c.Metrics[0].Labels = map[string]string{"bad-name": "x"} },
	} {
		c := testConfig("http://127.0.0.1:1/metrics")
		change(&c)
		if _, err := c.Compile(); err == nil {
			t.Fatalf("accepted invalid telemetry: %+v", c)
		}
	}
	c := testConfig("https://example.test/metrics")
	c.BearerAuth = true
	if _, err := New(c, Options{}); err == nil {
		t.Fatal("missing token accepted")
	}
	if _, err := New(c, Options{Token: "bad\nvalue"}); err == nil {
		t.Fatal("unsafe header token accepted")
	}
	c.BearerAuth = false
	c.Interval = ""
	c.Timeout = ""
	p, err := c.Compile()
	if err != nil || p.Every != time.Second || p.Limit != time.Second {
		t.Fatal("default scrape settings changed")
	}
}
func TestSelectedSeriesParsingAndUnknownValues(t *testing.T) {
	metrics := []Metric{{Name: "queue", Kind: "gauge", Labels: map[string]string{"path": "C:\\tmp\n\"quoted\""}}, {Name: "jobs_total", Kind: "counter"}}
	text := "# TYPE queue gauge\n# TYPE jobs_total counter\nqueue{path=\"C:\\\\tmp\\n\\\"quoted\\\"\"} -2.5e1\njobs_total 0\nother{ignored=\"value\"} NaN\n"
	v, err := parse([]byte(text), metrics)
	if err != nil || v[0].Value == nil || *v[0].Value != -25 || v[1].Value == nil || *v[1].Value != 0 {
		t.Fatalf("escaped selectors/zero: %+v %v", v, err)
	}
	spaced, err := parse([]byte("queue \t{ path = \"C:\\\\tmp\\n\\\"quoted\\\"\", } -25\njobs_total {} 0\n"), metrics)
	if err != nil || spaced[0].Value == nil || *spaced[0].Value != -25 || spaced[1].Value == nil || *spaced[1].Value != 0 {
		t.Fatalf("whitespace and empty labels: %+v %v", spaced, err)
	}
	for _, body := range []string{"jobs_total NaN\n", "jobs_total +Inf\n", "jobs_total -1\n", "jobs_total 1\njobs_total 2\n", "jobs_total 1 1234\n", "jobs_total{status=\"ok\"} 4\n", "# TYPE jobs_total gauge\njobs_total 5\n"} {
		v, err := parse([]byte(body), metrics)
		if err != nil || v[1].Value != nil || v[1].Issue == "" {
			t.Fatalf("missing/invalid series was measured: %q %+v %v", body, v, err)
		}
	}
	for _, body := range []string{"jobs_total not-a-number\n", "jobs_total 5", "jobs_total{a=\"x\",a=\"x\"} 1\n", "jobs_total{a=\"bad\\t\"} 1\n", "jobs_total " + strings.Repeat("0", 70<<10) + "\n"} {
		if _, err := parse([]byte(body), metrics); err == nil {
			t.Fatalf("accepted malformed selected text: %q", body[:min(60, len(body))])
		}
	}
}
func TestCounterResetsAndGapsNeverInventTraffic(t *testing.T) {
	c := testConfig("http://127.0.0.1:1/metrics")
	s, err := New(c, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.start = time.Now()
	gauges := []float64{-2, 2, 0, 0, 4, 6}
	counters := []float64{100, 112, 2, 0, 6, 10}
	for i := range gauges {
		values := []Value{{Value: number(gauges[i])}, {Value: number(counters[i])}}
		problem := ""
		if i == 3 {
			problem = "telemetry returned HTTP 503"
			values = nil
		}
		s.record(s.start.Add(time.Duration(i+1)*time.Second), 0, values, problem)
	}
	r := s.Finish()
	counter := r.Series[1]
	if counter.Samples != 5 || counter.Missing != 1 || counter.Resets != 1 || *counter.ObservedIncrease != 16 || counter.CoveredSeconds != 2 || *counter.MeanRate != 8 || *r.Series[0].Mean != 2 || r.Complete {
		t.Fatalf("invalid observed accounting: %+v", r)
	}
	if r.Timeline[0].Values[1].Rate != nil || !r.Timeline[2].Values[1].Reset || r.Timeline[2].Values[1].Rate != nil || r.Timeline[3].Values[1].Value != nil || r.Timeline[4].Values[1].Rate != nil || *r.Timeline[5].Values[1].Rate != 4 {
		t.Fatal("reset/gap interval invented a counter rate")
	}
	if err := ValidateSummary(&c, &r, s.start, 6); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(r)
	var imported Summary
	if err := json.Unmarshal(data, &imported); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSummary(&c, &imported, s.start, 6); err != nil {
		t.Fatal("round-trip observations invalid:", err)
	}
	imported.Series[0].Samples++
	if ValidateSummary(&c, &imported, s.start, 6) == nil {
		t.Fatal("inconsistent imported sample accounting accepted")
	}
	live := s.Live()
	*live.Values[0].Value = 999
	if *s.Live().Values[0].Value == 999 {
		t.Fatal("live observations alias collector state")
	}
}

func TestOneTypeDeclarationSupportsSeveralLabelSelectors(t *testing.T) {
	metrics := []Metric{
		{Name: "jobs_total", Kind: "counter", Labels: map[string]string{"outcome": "completed"}},
		{Name: "jobs_total", Kind: "counter", Labels: map[string]string{"outcome": "rejected"}},
	}
	body := "# TYPE jobs_total counter\njobs_total{outcome=\"completed\"} 12\njobs_total{outcome=\"rejected\"} 3\n"
	values, err := parse([]byte(body), metrics)
	if err != nil || values[0].Value == nil || *values[0].Value != 12 || values[1].Value == nil || *values[1].Value != 3 {
		t.Fatalf("family metadata treated as repeated per selector: %+v %v", values, err)
	}
	if _, err := parse([]byte("# TYPE jobs_total counter\n"+body), metrics); err == nil {
		t.Fatal("duplicate exporter TYPE declaration accepted")
	}
}
func TestTimelineRetentionKeepsWholeRunStatistics(t *testing.T) {
	c := testConfig("http://127.0.0.1:1/metrics")
	s, _ := New(c, Options{})
	defer s.Close()
	s.start = time.Now()
	for i := range MaxPoints + 7 {
		s.record(s.start.Add(time.Duration(i+1)*time.Second), 0, []Value{{Value: number(float64(i))}, {Value: number(float64(i))}}, "")
	}
	r := s.Finish()
	if len(r.Timeline) != MaxPoints || !r.Truncated || r.Timeline[0].Sequence != 8 || r.Series[0].Samples != MaxPoints+7 || *r.Series[0].Min != 0 {
		t.Fatal("retention lost aggregates")
	}
	if err := ValidateSummary(&c, &r, s.start, MaxPoints+7); err != nil {
		t.Fatal(err)
	}
}

func TestWallClockAdjustmentPreservesElapsedObservationBounds(t *testing.T) {
	c := testConfig("http://127.0.0.1:1/metrics")
	s, err := New(c, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.start = time.Now()
	s.record(s.start.Add(time.Second), 0, []Value{{Value: number(0)}, {Value: number(7)}}, "")
	r := s.Finish()
	r.Timeline[0].ObservedAt = r.Timeline[0].ObservedAt.Add(-time.Minute)
	if err := ValidateSummary(&c, &r, s.start, 1); err != nil {
		t.Fatal("a UTC clock adjustment invalidated monotonic observations:", err)
	}
	r.Timeline[0].ElapsedSeconds = 1.1
	if ValidateSummary(&c, &r, s.start, 1) == nil {
		t.Fatal("an observation beyond total run time was accepted")
	}
}
func TestHTTPBoundariesTLSAndNoRedirectCredentials(t *testing.T) {
	const token = "test-telemetry-secret"
	var downstream atomic.Int64
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downstream.Add(1) }))
	defer redirect.Close()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "no", 401)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, redirect.URL, 302)
			return
		}
		w.Header().Set("Content-Type", "text/plain;version=0.0.4")
		fmt.Fprint(w, "queue 0\njobs_total 1\n")
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	c := testConfig(srv.URL)
	c.BearerAuth = true
	untrusted, _ := New(c, Options{Token: token})
	defer untrusted.Close()
	if _, err := untrusted.fetch(context.Background()); err == nil {
		t.Fatal("untrusted telemetry TLS accepted")
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	trusted, err := New(c, Options{Token: token, CAFile: ca})
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Close()
	if v, err := trusted.fetch(context.Background()); err != nil || v[0].Value == nil || *v[0].Value != 0 {
		t.Fatalf("trusted telemetry: %+v %v", v, err)
	}
	c.URL = srv.URL + "/redirect"
	client, _ := New(c, Options{Token: token, CAFile: ca})
	defer client.Close()
	if _, err := client.fetch(context.Background()); err == nil || downstream.Load() != 0 {
		t.Fatal("telemetry followed a redirect")
	}
}
func TestScrapeFailureBoundsAndCancellation(t *testing.T) {
	for _, mode := range []string{"timeout", "oversized", "type", "compression"} {
		t.Run(mode, func(t *testing.T) {
			var concurrent, peak atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := concurrent.Add(1)
				defer concurrent.Add(-1)
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
				w.Header().Set("Content-Type", "text/plain;version=0.0.4")
				switch mode {
				case "timeout":
					<-r.Context().Done()
				case "oversized":
					fmt.Fprint(w, strings.Repeat("x", MaxBody+1))
				case "type":
					w.Header().Set("Content-Type", "application/openmetrics-text")
					fmt.Fprint(w, "queue 1\n# EOF\n")
				case "compression":
					w.Header().Set("Content-Encoding", "gzip")
					fmt.Fprint(w, "queue 1\n")
				}
			}))
			defer srv.Close()
			s, _ := New(testConfig(srv.URL), Options{})
			defer s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			s.Start(ctx, time.Now())
			time.Sleep(130 * time.Millisecond)
			cancel()
			before := time.Now()
			r := s.Finish()
			if r.Attempts < 1 || r.Complete || r.FailedScrapes != r.Attempts || time.Since(before) > time.Second || peak.Load() > 1 || r.Series[0].Last != nil {
				t.Fatalf("failure not bounded/unknown: %+v", r)
			}
		})
	}
}

func TestStoppingPendingScrapeIsNotAnExporterOutage(t *testing.T) {
	started := make(chan struct{})
	var requests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "text/plain;version=0.0.4")
		fmt.Fprint(w, "queue 0\njobs_total 5\n")
	}))
	defer target.Close()
	c := testConfig(target.URL)
	c.Timeout = "5s"
	s, err := New(c, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Start(context.Background(), time.Now())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first scrape did not start")
	}
	r := s.Finish()
	if requests.Load() != 2 || r.Attempts != 1 || r.CanceledScrapes != 1 || r.FailedScrapes != 0 || !r.Complete || *r.Series[0].Last != 0 || r.Series[1].MeanRate != nil {
		t.Fatalf("stopping invented a failure or a rate: %+v", r)
	}
	if err := ValidateSummary(&c, &r, s.start, time.Since(s.start).Seconds()); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationBeforeSharedStartDoesNotScrape(t *testing.T) {
	var requests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer target.Close()
	c := testConfig(target.URL)
	s, err := New(c, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx, time.Now().Add(5*time.Second))
	cancel()
	r := s.Finish()
	if requests.Load() != 0 || r.Attempts != 0 || r.Complete || r.Series[0].Last != nil {
		t.Fatal("canceling before admission fabricated observations or contacted the exporter")
	}
	if err := ValidateSummary(&c, &r, s.start, 0); err != nil {
		t.Fatal(err)
	}
}
func FuzzSelectedTelemetryText(f *testing.F) {
	for _, seed := range []string{"queue 0\n", "queue{a=\"b\"} 5\n", "queue NaN\n", "queue{a=\"bad\\\"\"} 1\n", "# TYPE queue gauge\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > MaxBody {
			return
		}
		values, err := parse([]byte(data), []Metric{{Name: "queue", Kind: "gauge"}})
		if err == nil {
			if len(values) != 1 || (values[0].Value != nil && !finite(*values[0].Value)) {
				t.Fatal("parser returned invalid numeric data")
			}
		}
	})
}
