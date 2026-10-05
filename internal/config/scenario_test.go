package config

import (
	"net/url"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func scenarioConfig() Config {
	c := Defaults()
	c.URL = "http://example.test"
	c.Mode = "rate"
	c.Rate = 10
	c.Scenarios = []Scenario{{Name: "Login", Steps: []Step{{URL: "/login", Extract: map[string]Extraction{"token": {JSONPointer: ptr("/token")}}}, {URL: "/account", Headers: map[string]string{"Authorization": "Bearer {{token}}"}}}}}
	return c
}

func TestInvalidScenariosAreRejectedBeforeExecution(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"empty steps", func(c *Config) { c.Scenarios[0].Steps = nil }},
		{"duplicate scenario", func(c *Config) { c.Scenarios = append(c.Scenarios, c.Scenarios[0]) }},
		{"zero weight", func(c *Config) { c.Scenarios[0].Weight = ptr(0) }},
		{"unknown variable", func(c *Config) { c.Scenarios[0].Steps[0].Body = "{{future}}" }},
		{"future extraction", func(c *Config) { c.Scenarios[0].Steps[0].Headers = map[string]string{"Authorization": "{{token}}"} }},
		{"reserved variable", func(c *Config) { c.Variables = map[string]string{"user_id": "other"} }},
		{"mismatched data", func(c *Config) { c.Scenarios[0].Data = []map[string]string{{"a": "1"}, {"b": "2"}} }},
		{"empty data row", func(c *Config) { c.Scenarios[0].Data = []map[string]string{{}} }},
		{"ambiguous extraction", func(c *Config) {
			c.Scenarios[0].Steps[0].Extract["token"] = Extraction{JSONPointer: ptr("/token"), Header: "X-Token"}
		}},
		{"invalid pointer", func(c *Config) { c.Scenarios[0].Steps[0].Extract["token"] = Extraction{JSONPointer: ptr("token")} }},
		{"invalid assertion", func(c *Config) { c.Scenarios[0].Steps[0].Assertions = []Assertion{{Header: "X-Token"}} }},
		{"two assertion selectors", func(c *Config) {
			c.Scenarios[0].Steps[0].Assertions = []Assertion{{Header: "X-Token", JSONPointer: ptr("/token"), Equals: ptr("value")}}
		}},
		{"cross origin", func(c *Config) { c.Scenarios[0].Steps[0].URL = "https://other.test/login" }},
		{"duplicate step", func(c *Config) { c.Scenarios[0].Steps[0].Name = "same"; c.Scenarios[0].Steps[1].Name = "same" }},
		{"bad distribution", func(c *Config) { c.ArrivalDistribution = "poisson" }},
		{"jitter in users mode", func(c *Config) { c.Mode = "users"; c.Rate = 0; c.ArrivalDistribution = "jitter" }},
		{"journey timeout", func(c *Config) { c.JourneyTimeout = "0s" }},
		{"journey threshold", func(c *Config) { c.JourneyThresholds.MaxErrorPercent = ptr(101.0) }},
		{"step pause", func(c *Config) { c.Scenarios[0].Steps[0].Pause = "31s" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := scenarioConfig()
			tt.change(&c)
			if _, err := c.Validate(); err == nil {
				t.Fatal("invalid scenario accepted")
			}
		})
	}
}

func TestWeightedRangeMatchesIndividualSelections(t *testing.T) {
	c := scenarioConfig()
	c.Scenarios = []Scenario{{Name: "A", Weight: ptr(2), Steps: []Step{{URL: "/"}}}, {Name: "B", Weight: ptr(3), Steps: []Step{{URL: "/"}}}, {Name: "C", Steps: []Step{{URL: "/"}}}}
	p, err := c.Validate()
	if err != nil {
		t.Fatal(err)
	}
	for start := int64(0); start < 12; start++ {
		for n := int64(0); n < 40; n++ {
			got := p.ScenarioRange(start, n)
			want := make([]int64, 3)
			for id := start; id < start+n; id++ {
				want[p.ScenarioAt(id)]++
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("start=%d n=%d: got %v, want %v", start, n, got, want)
				}
			}
		}
	}
	got := p.ScenarioRange(0, 60000000000)
	if got[0] != 20000000000 || got[1] != 30000000000 || got[2] != 10000000000 {
		t.Fatalf("large range: %v", got)
	}
}

func TestSameOriginAllowsDefaultPortsAndRejectsDestinationChanges(t *testing.T) {
	base, _ := url.Parse("http://example.test/api/")
	if got, err := ResolveStepURL(base, "http://EXAMPLE.test:80/profile"); err != nil || got == "" {
		t.Fatalf("same origin rejected: %v", err)
	}
	for _, raw := range []string{"//evil.test/", "http://example.test:81/", "https://example.test/", "http://user:pass@example.test/", "/profile#fragment"} {
		if _, err := ResolveStepURL(base, raw); err == nil {
			t.Errorf("destination escape accepted: %q", raw)
		}
	}
}

func TestJourneyOptionsCannotBeSilentlyIgnored(t *testing.T) {
	c := Defaults()
	c.URL = "http://example.test"
	c.JourneyThresholds.P95 = "1ms"
	if _, err := c.Validate(); err == nil {
		t.Fatal("journey threshold accepted without scenarios")
	}
}
