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

func TestTelemetryPersistenceAndImportWithoutTrafficOrSecrets(t *testing.T) {
	s := testServer(t)
	s.telemetryToken = "test-private-telemetry-token"
	var attempts, scrapes atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			scrapes.Add(1)
			if r.Header.Get("Authorization") != "Bearer "+s.telemetryToken {
				http.Error(w, "auth", 401)
				return
			}
			w.Header().Set("Content-Type", "text/plain;version=0.0.4")
			fmt.Fprintf(w, "jobs_total %d\nqueue 0\n", attempts.Load())
			return
		}
		attempts.Add(1)
		fmt.Fprint(w, "ok")
	}))
	defer target.Close()
	body := fmt.Sprintf(`{"name":"Server telemetry","url":%q,"mode":"rate","rate":5,"duration":"1250ms","telemetry":{"url":%q,"interval":"250ms","bearer_auth":true,"metrics":[{"name":"jobs_total","kind":"counter"},{"name":"queue","kind":"gauge"}]}}`, target.URL, target.URL+"/metrics")
	if response := request(s, "POST", "/api/validate", body); response.Code != 200 || attempts.Load() != 0 || scrapes.Load() != 0 {
		t.Fatal("validation contacted target/exporter")
	}
	id := startRun(t, s, body)
	run := awaitRun(t, s, id)
	if run.Status != "passed" || run.Result == nil || run.Result.SchemaVersion != 5 || run.Result.Completed != 7 || !run.Result.Telemetry.Complete || run.Samples[1].Telemetry == nil {
		t.Fatalf("dashboard telemetry result: %+v", run)
	}
	bundle, _ := json.Marshal(run)
	if strings.Contains(string(bundle), s.telemetryToken) {
		t.Fatal("bundle contains runtime telemetry credential")
	}
	countBefore := scrapes.Load()
	response := request(s, "POST", "/api/runs/import", string(bundle))
	if response.Code != 201 || attempts.Load() != 7 || scrapes.Load() != countBefore {
		t.Fatalf("schema 5 import: %d %s", response.Code, response.Body)
	}
	reopened, err := openStore(s.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.getRun(id)
	if err != nil || saved.Result.Telemetry.Attempts != run.Result.Telemetry.Attempts || saved.Samples[1].Telemetry == nil {
		t.Fatal("telemetry did not survive reopening workspace")
	}
	run.Result.Telemetry.Series[0].Missing++
	bad, _ := json.Marshal(run)
	if response := request(s, "POST", "/api/runs/import", string(bad)); response.Code != 400 {
		t.Fatal("inconsistent telemetry import accepted")
	}
}
func TestTelemetryAuthSetupRejectedBeforeRunReservation(t *testing.T) {
	s := testServer(t)
	s.telemetryToken = ""
	body := `{"url":"http://127.0.0.1:8787/work","telemetry":{"url":"http://127.0.0.1:8787/metrics","bearer_auth":true,"metrics":[{"name":"queue","kind":"gauge"}]}}`
	if response := request(s, "POST", "/api/validate", body); response.Code != 200 {
		t.Fatal("validation requires runtime credentials")
	}
	if response := request(s, "POST", "/api/runs", body); response.Code != 400 || len(s.store.runs) != 0 {
		t.Fatal("missing telemetry token reserved a run")
	}
}
