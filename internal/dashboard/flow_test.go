package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFlowBundlePersistsAndImportsWithoutTraffic(t *testing.T) {
	server := testServer(t)
	var attempts atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1); fmt.Fprint(w, "ok") }))
	defer target.Close()
	body := fmt.Sprintf(`{"name":"Flow","url":%q,"mode":"rate","rate":1,"duration":"20ms","max_in_flight":1,"scenarios":[{"name":"Journey","steps":[{"name":"Loop","loop":{"count":3,"steps":[{"name":"Request","url":"/"}]}}]}]}`, target.URL)
	if response := request(server, "POST", "/api/validate", body); response.Code != 200 || attempts.Load() != 0 {
		t.Fatalf("%s", response.Body)
	}
	run := awaitRun(t, server, startRun(t, server, body))
	if run.Status != "passed" || run.Result.SchemaVersion != 6 || attempts.Load() != 3 {
		t.Fatalf("%+v", run)
	}
	bundle, _ := json.Marshal(run)
	if response := request(server, "POST", "/api/runs/import", string(bundle)); response.Code != 201 || attempts.Load() != 3 {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	reopened, err := openStore(server.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.getRun(run.ID)
	if err != nil || saved.Result.Scenarios[0].Controls[0].Iterations != 3 {
		t.Fatalf("persist: %v %+v", err, saved)
	}
	run.Result.Scenarios[0].Steps[0].SkippedCondition = 1
	bundle, _ = json.Marshal(run)
	if response := request(server, "POST", "/api/runs/import", string(bundle)); response.Code != 400 || attempts.Load() != 3 {
		t.Fatalf("tampered flow accepted %d %s", response.Code, response.Body)
	}
}
