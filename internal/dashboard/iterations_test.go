package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFiniteIterationBundlesPreserveStoppingContract(t *testing.T) {
	s := testServer(t)
	var attempts atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1) }))
	defer target.Close()
	for _, steps := range []string{"", `,"scenarios":[{"name":"Journey","steps":[{"name":"Repeat","loop":{"count":2,"steps":[{"name":"Request","url":"/"}]}}]}]`} {
		body := fmt.Sprintf(`{"url":%q,"vus":2,"iterations":3%s}`, target.URL, steps)
		before := attempts.Load()
		if response := request(s, "POST", "/api/validate", body); response.Code != 200 || attempts.Load() != before {
			t.Fatalf("validation: %d %s", response.Code, response.Body)
		}
		run := awaitRun(t, s, startRun(t, s, body))
		if run.Status != "passed" || run.Config.Duration != "" || run.Result.SchemaVersion != 7 || run.Result.StopReason != "iterations" || run.Result.Iterations != 3 {
			t.Fatalf("finite run: %+v", run)
		}
		bundle, _ := json.Marshal(run)
		before = attempts.Load()
		response := request(s, "POST", "/api/runs/import", string(bundle))
		if response.Code != 201 || attempts.Load() != before {
			t.Fatalf("bundle round trip: %d %s", response.Code, response.Body)
		}
		for _, mutate := range []func(*Run){
			func(r *Run) { r.Result.ConcurrencyLimit++ },
			func(r *Run) { r.Result.Iterations++ },
			func(r *Run) { r.Result.StopReason = "duration" },
			func(r *Run) { r.Result.SchemaVersion = 6 },
			func(r *Run) { r.Config.Iterations++ },
		} {
			var invalid Run
			json.Unmarshal(bundle, &invalid)
			mutate(&invalid)
			data, _ := json.Marshal(invalid)
			if response := request(s, "POST", "/api/runs/import", string(data)); response.Code != 400 {
				t.Fatalf("inconsistent finite bundle accepted: %d %s", response.Code, response.Body)
			}
		}
	}
}
