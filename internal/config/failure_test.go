package config

import "testing"

func TestFailureActions(t *testing.T) {
	for _, mode := range []string{"users", "rate"} {
		for _, action := range []string{"", NextIteration, ContinueJourney, StopUser, StopTest, StopTestNow, "unknown", "Continue"} {
			c := Defaults()
			c.URL, c.Mode, c.OnError = "http://example.test", mode, action
			if mode == "rate" {
				c.Rate = 1
			}
			_, err := c.Validate()
			valid := action == "" || action == NextIteration || (mode == "users" && (action == ContinueJourney || action == StopUser || action == StopTest || action == StopTestNow))
			if (err == nil) != valid {
				t.Fatalf("mode=%s action=%q: %v", mode, action, err)
			}
		}
	}
}
