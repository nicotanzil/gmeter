// Package telemetry collects selected server metrics independently of load traffic.
package telemetry

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxMetrics = 16
const MaxPoints = 900
const MaxBody = 1 << 20

type Metric struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Kind   string            `json:"kind"`
	Unit   string            `json:"unit,omitempty"`
	Title  string            `json:"title,omitempty"`
}
type Config struct {
	URL        string   `json:"url"`
	Interval   string   `json:"interval,omitempty"`
	Timeout    string   `json:"timeout,omitempty"`
	BearerAuth bool     `json:"bearer_auth,omitempty"`
	Metrics    []Metric `json:"metrics"`
}
type Plan struct {
	Config
	Every time.Duration
	Limit time.Duration
}

var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func printable(s string, limit int) bool {
	if len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (c Config) Compile() (Plan, error) {
	p := Plan{Config: c}
	u, err := url.Parse(c.URL)
	if err != nil || len(c.URL) > 2048 || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(c.URL, "#") || u.Opaque != "" {
		return p, fmt.Errorf("telemetry.url must be an HTTP(S) endpoint without credentials, query, or fragment")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return p, fmt.Errorf("telemetry.url has an invalid port")
		}
	}
	if c.BearerAuth && u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return p, fmt.Errorf("authenticated telemetry requires HTTPS or a literal loopback IP")
		}
	}
	if p.Interval == "" {
		p.Interval = "1s"
	}
	if p.Timeout == "" {
		p.Timeout = "1s"
	}
	p.Every, err = time.ParseDuration(p.Interval)
	if err != nil || p.Every < 250*time.Millisecond || p.Every > time.Minute {
		return p, fmt.Errorf("telemetry.interval must be between 250ms and 1m")
	}
	p.Limit, err = time.ParseDuration(p.Timeout)
	if err != nil || p.Limit < 100*time.Millisecond || p.Limit > 5*time.Second {
		return p, fmt.Errorf("telemetry.timeout must be between 100ms and 5s")
	}
	if len(c.Metrics) < 1 || len(c.Metrics) > MaxMetrics {
		return p, fmt.Errorf("telemetry.metrics must select 1–16 scalar series")
	}
	p.Metrics = make([]Metric, len(c.Metrics))
	seen := map[string]bool{}
	for i, m := range c.Metrics {
		if !metricName.MatchString(m.Name) || len(m.Name) > 128 || (m.Kind != "gauge" && m.Kind != "counter") || !printable(m.Title, 80) || !printable(m.Unit, 24) || len(m.Labels) > 16 {
			return p, fmt.Errorf("telemetry.metrics[%d] needs a legacy Prometheus name, gauge/counter kind, and bounded title/unit/labels", i)
		}
		labels := make(map[string]string, len(m.Labels))
		for k, v := range m.Labels {
			if !labelName.MatchString(k) || len(k) > 128 || len(v) > 256 || !utf8.ValidString(v) {
				return p, fmt.Errorf("telemetry.metrics[%d] has an invalid label", i)
			}
			labels[k] = v
		}
		m.Labels = labels
		identity, _ := json.Marshal(struct {
			Name   string
			Labels map[string]string
		}{m.Name, m.Labels})
		if seen[string(identity)] {
			return p, fmt.Errorf("telemetry metric selectors must be unique")
		}
		seen[string(identity)] = true
		if m.Title == "" {
			m.Title = m.Name
		}
		p.Metrics[i] = m
	}
	return p, nil
}
