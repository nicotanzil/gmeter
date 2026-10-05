package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsMalformedOrAmbiguousInput(t *testing.T) {
	for _, input := range []string{
		`{"url":"http://localhost","rtae":20}`,
		`{"url":"http://localhost"} {}`,
		`null`, `[]`, `{`,
		strings.Repeat(" ", MaxConfigBytes+1),
	} {
		if _, err := Load(strings.NewReader(input)); err == nil {
			t.Errorf("accepted invalid input %.60q", input)
		}
	}
}

func TestLoadKeepsDefaultsAndParsesStages(t *testing.T) {
	c, err := Load(strings.NewReader(`{"url":"http://localhost","mode":"rate","stages":[{"name":"load","duration":"2s","rate":20},{"name":"pause","duration":"1s","rate":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if p.LoadDuration != 3*time.Second || p.RequestTimeout != 5*time.Second || len(p.Phases) != 2 {
		t.Fatalf("incorrect plan: %+v", p)
	}
	if c.Thresholds.MaxDropPercent == nil || *c.Thresholds.MaxDropPercent != 0 {
		t.Fatal("missing default demand-delivery threshold")
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"relative URL", func(c *Config) { c.URL = "/api" }},
		{"unsupported scheme", func(c *Config) { c.URL = "ftp://localhost" }},
		{"credentials", func(c *Config) { c.URL = "http://user:password@localhost" }},
		{"fragment", func(c *Config) { c.URL += "#foo" }},
		{"port", func(c *Config) { c.URL = "http://localhost:99999" }},
		{"method", func(c *Config) { c.Method = "GET\r\n" }},
		{"header", func(c *Config) { c.Headers = map[string]string{"X-Test": "value\r\nInjected: yes"} }},
		{"header control", func(c *Config) { c.Headers = map[string]string{"X-Test": "value\x01"} }},
		{"framing header", func(c *Config) { c.Headers = map[string]string{"Content-Length": "99"} }},
		{"mode", func(c *Config) { c.Mode = "unknown" }},
		{"users", func(c *Config) { c.VUs = 0 }},
		{"capacity", func(c *Config) { c.MaxInFlight = 0 }},
		{"duration", func(c *Config) { c.Duration = "-1s" }},
		{"timeout", func(c *Config) { c.Timeout = "0s" }},
		{"response limit", func(c *Config) { c.MaxResponseBytes = 0 }},
		{"rate in users mode", func(c *Config) { c.Rate = 10 }},
		{"missing rate", func(c *Config) { c.Mode = "rate" }},
		{"think time in rate mode", func(c *Config) { c.Mode = "rate"; c.Rate = 10; c.ThinkTime = "1s" }},
		{"too long", func(c *Config) {
			c.Mode = "rate"
			c.Stages = []Stage{{Duration: "24h", Rate: 1}, {Duration: "1s", Rate: 1}}
		}},
		{"all pause", func(c *Config) { c.Mode = "rate"; c.Stages = []Stage{{Duration: "1s"}} }},
		{"two rate sources", func(c *Config) { c.Mode = "rate"; c.Rate = 1; c.Stages = []Stage{{Duration: "1s", Rate: 1}} }},
		{"percent", func(c *Config) { v := 101.0; c.Thresholds.MaxErrorPercent = &v }},
		{"status", func(c *Config) { c.SuccessStatuses = []int{999} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Defaults()
			c.URL = "http://localhost:8787/work"
			tt.edit(&c)
			if _, err := c.Validate(); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}
