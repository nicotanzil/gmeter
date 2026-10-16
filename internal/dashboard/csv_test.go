package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCSVPreviewAndPortableRunBundles(t *testing.T) {
	s := testServer(t)
	var attempts atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1) }))
	defer target.Close()
	source := `{"name":"people.csv","content":"Full Name,item\nalice,tea\nbob,coffee","columns":{"Full Name":"username","item":""}}`
	for _, path := range []string{"/api/csv/preview", "/api/csv/validate"} {
		w := request(s, "POST", path, source)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"row_count":2`) || attempts.Load() != 0 {
			t.Fatalf("preview: %d %s", w.Code, w.Body)
		}
	}
	for _, body := range []string{`{"content":"a\n\"broken"}`, `{"content":"a\nb","path":"/etc/passwd"}`, `{"content":"a\nb"} {}`, `null`, `{"content":"user_id\nx"}`} {
		if w := request(s, "POST", "/api/csv/validate", body); w.Code != 400 {
			t.Fatalf("invalid CSV: %d %s", w.Code, w.Body)
		}
	}
	body := fmt.Sprintf(`{"url":%q,"vus":3,"iterations":5,"ramp_up":"30ms","on_error":"continue","scenarios":[{"name":"Data","csv":{"name":"people.csv","content":"Full Name,item\nalice,tea\nbob,coffee","columns":{"Full Name":"username","item":""},"on_eof":"stop_user"},"steps":[{"name":"Read","url":"/?name={{username}}"}]}]}`, target.URL)
	if w := request(s, "POST", "/api/scenarios", body); w.Code != 200 || attempts.Load() != 0 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	run := awaitRun(t, s, startRun(t, s, body))
	if run.Status != "passed" || run.Result.SchemaVersion != 11 || run.Result.StopReason != "data_exhausted" || run.Result.Completed != 2 || attempts.Load() != 2 || run.Result.Datasets[0].UsersExhausted != 3 || run.Samples[0].Datasets[0].RowsAssigned != 0 {
		t.Fatalf("CSV run: %+v", run)
	}
	bundle, _ := json.Marshal(run)
	if w := request(s, "POST", "/api/runs/import", string(bundle)); w.Code != 201 || attempts.Load() != 2 {
		t.Fatalf("roundtrip: %d %s", w.Code, w.Body)
	}
	for i, mutate := range []func(*Run){
		func(r *Run) { r.Result.SchemaVersion = 10 }, func(r *Run) { r.Result.Datasets = nil }, func(r *Run) { r.Config.Scenarios[0].CSV.OnEOF = "recycle" }, func(r *Run) { r.Result.Datasets[0].RowsAssigned = 3 }, func(r *Run) { r.Result.Datasets[0].UsersExhausted = 4 }, func(r *Run) { r.Result.Datasets[0].UsersExhausted = 0 }, func(r *Run) { r.Result.StopReason = "iterations" },
		func(r *Run) { r.Samples[0].Datasets = nil }, func(r *Run) { r.Samples[0].Datasets[0].RowsAssigned = 1 }, func(r *Run) { r.Samples[0].Datasets[0].Name = "forged" }, func(r *Run) { r.Samples[len(r.Samples)-1].Datasets[0].UsersExhausted = 2 },
	} {
		var invalid Run
		json.Unmarshal(bundle, &invalid)
		mutate(&invalid)
		data, _ := json.Marshal(invalid)
		if w := request(s, "POST", "/api/runs/import", string(data)); w.Code != 400 {
			t.Fatalf("invalid case %d: %d %s", i, w.Code, w.Body)
		}
	}
	stored, err := s.store.getRun(run.ID)
	if err != nil || stored.Config.Scenarios[0].CSV.Content != "Full Name,item\nalice,tea\nbob,coffee" || stored.Result.Datasets[0].RowsAssigned != 2 {
		t.Fatalf("persistence: %v %+v", err, stored)
	}
}
