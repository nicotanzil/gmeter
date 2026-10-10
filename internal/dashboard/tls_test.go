package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTLSSettingSurvivesRunBundleAndRejectsMismatch(t *testing.T) {
	s := testServer(t)
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") }))
	defer target.Close()
	run := awaitRun(t, s, startRun(t, s, fmt.Sprintf(`{"url":%q,"insecure":true,"mode":"rate","rate":1,"duration":"40ms"}`, target.URL)))
	if !run.Config.Insecure || !run.Result.Insecure || !run.Result.Passed {
		t.Fatal("insecure setting not retained")
	}
	data, _ := json.Marshal(run)
	if w := request(s, "POST", "/api/runs/import", string(data)); w.Code != 201 {
		t.Fatalf("bundle roundtrip: %d %s", w.Code, w.Body)
	}
	run.Result.Insecure = false
	data, _ = json.Marshal(run)
	if w := request(s, "POST", "/api/runs/import", string(data)); w.Code != 400 {
		t.Fatal("mismatched TLS setting accepted")
	}
	if w := request(s, "GET", "/api/runs/"+run.ID+"/report.html", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "TLS verification disabled") {
		t.Fatal("TLS label omitted")
	}
}
