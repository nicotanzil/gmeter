package config

import (
	"strings"
	"testing"
)

func TestIterationConfigurationDefaultsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		source, duration string
		valid            bool
	}{
		{`{"url":"http://example.test","iterations":5}`, "", true},
		{`{"url":"http://example.test","iterations":5,"duration":"1m"}`, "1m", true},
		{`{"url":"http://example.test","iterations":5,"duration":""}`, "", true},
		{`{"url":"http://example.test"}`, "10s", true},
		{`{"url":"http://example.test","duration":""}`, "", false},
		{`{"url":"http://example.test","iterations":-1}`, "10s", false},
		{`{"url":"http://example.test","iterations":1000001}`, "", false},
		{`{"url":"http://example.test","iterations":5,"mode":"rate","rate":10}`, "", false},
		{`{"url":"http://example.test","iterations":5,"duration":"0s"}`, "0s", false},
	} {
		c, err := Load(strings.NewReader(tc.source))
		if err != nil {
			t.Fatal(err)
		}
		if c.Duration != tc.duration {
			t.Fatalf("%s: duration=%q", tc.source, c.Duration)
		}
		_, err = c.Validate()
		if (err == nil) != tc.valid {
			t.Fatalf("%s: validation=%v", tc.source, err)
		}
	}
}
