package config

import (
	"testing"
	"time"
)

func TestUserScheduleOffsetsAndValidation(t *testing.T) {
	c := Defaults()
	c.URL = "http://example.test"
	c.VUs = 4
	c.RampUp = "4s"
	c.StartupDelay = "2s"
	p, err := c.Validate()
	if err != nil {
		t.Fatal(err)
	}
	for user := 0; user < 4; user++ {
		if got := p.UserStartOffset(user); got != time.Duration(2+user)*time.Second {
			t.Fatalf("user %d: %s", user, got)
		}
	}
	c.VUs = 100000
	c.RampUp = "24h"
	c.StartupDelay = "24h"
	p, err = c.Validate()
	if err != nil || p.UserStartOffset(99999) < 47*time.Hour {
		t.Fatalf("large offsets overflowed: %v %+v", err, p)
	}
	for _, tc := range []struct {
		mode, ramp, delay string
		valid             bool
	}{
		{"users", "0s", "0", true}, {"users", "1.5s", "200ms", true}, {"users", "-1s", "", false}, {"users", "1s", "-1s", false}, {"users", "25h", "", false}, {"users", "bad", "", false}, {"rate", "1s", "", false}, {"rate", "", "1s", false}, {"rate", "0s", "0s", true},
	} {
		c := Defaults()
		c.URL = "http://example.test"
		c.Mode = tc.mode
		c.RampUp = tc.ramp
		c.StartupDelay = tc.delay
		if c.Mode == "rate" {
			c.Rate = 1
		}
		_, err := c.Validate()
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}
