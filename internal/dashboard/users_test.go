package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestUserScheduleBundlesPreserveTimingAndRejectMismatches(t *testing.T) {
	s := testServer(t)
	var starters []SavedScenario
	json.Unmarshal(request(s, "GET", "/api/examples", "").Body.Bytes(), &starters)
	found := false
	for _, starter := range starters {
		if starter.ID == "users-ramp" {
			found = starter.Config.RampUp == "4s" && starter.Config.StartupDelay == "2s" && starter.Config.Iterations == 3
		}
	}
	if !found {
		t.Fatal("user ramp starter is missing")
	}
	var attempts atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1) }))
	defer target.Close()
	body := fmt.Sprintf(`{"url":%q,"vus":3,"iterations":2,"ramp_up":"150ms","startup_delay":"30ms","scenarios":[{"name":"Flow","steps":[{"name":"Loop","loop":{"count":2,"steps":[{"name":"HTTP","url":"/"}]}}]}]}`, target.URL)
	if w := request(s, "POST", "/api/validate", body); w.Code != 200 || attempts.Load() != 0 {
		t.Fatalf("validation: %d %s", w.Code, w.Body)
	}
	run := awaitRun(t, s, startRun(t, s, body))
	if run.Status != "passed" || run.Result.SchemaVersion != 8 || run.Result.Completed != 12 || run.Result.Journeys.Completed != 6 || attempts.Load() != 12 || run.Result.UserSchedule.StartupSeconds != .03 || run.Result.UserSchedule.Users.Started != 3 || run.Samples[0].UserSchedule == nil || !run.Samples[0].UserSchedule.Waiting {
		t.Fatalf("scheduled users result: %+v", run)
	}
	bundle, _ := json.Marshal(run)
	if w := request(s, "POST", "/api/runs/import", string(bundle)); w.Code != 201 || attempts.Load() != 12 {
		t.Fatalf("round trip: %d %s", w.Code, w.Body)
	}
	for _, mutate := range []func(*Run){
		func(r *Run) { r.Config.RampUp = "1s" }, func(r *Run) { r.Config.StartupDelay = "1s" }, func(r *Run) { r.Result.SchemaVersion = 7 }, func(r *Run) { r.Result.UserSchedule = nil },
		func(r *Run) { r.Result.UserSchedule.Users.Active = 1 }, func(r *Run) { r.Result.UserSchedule.Users.Started = 4 }, func(r *Run) { r.Result.UserSchedule.StartupSeconds = 0 },
		func(r *Run) { r.Result.TotalSeconds = r.Result.LoadSeconds }, func(r *Run) { r.Samples[0].UserSchedule = nil }, func(r *Run) { r.Samples[0].UserSchedule.Users.Started = 1 },
	} {
		var invalid Run
		json.Unmarshal(bundle, &invalid)
		mutate(&invalid)
		data, _ := json.Marshal(invalid)
		if w := request(s, "POST", "/api/runs/import", string(data)); w.Code != 400 {
			t.Fatalf("invalid schedule accepted: %d %s", w.Code, w.Body)
		}
	}
	// Reload persisted history through the same store boundary used on restart.
	stored, err := s.store.getRun(run.ID)
	if err != nil || stored.Result.UserSchedule.Users.Finished != 3 || stored.Config.StartupDelay != "30ms" {
		t.Fatalf("persistence: %v %+v", err, stored)
	}
}
