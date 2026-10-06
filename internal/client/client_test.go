package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adhuldas/nodexa-cli/internal/manifest"
)

func reserve(t *testing.T, healthchecks map[string]*manifest.Healthcheck) map[string]any {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/fleets/f1/releases" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"fleet_id":"f1","revision":1,"status":"pending","pushed_by":"u","services":[],"created_at":"","completed_at":null}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "tok").ReserveRelease("f1", []string{"web", "worker"}, healthchecks); err != nil {
		t.Fatalf("ReserveRelease: %v", err)
	}
	return got
}

func TestReserveReleaseSendsHealthchecks(t *testing.T) {
	got := reserve(t, map[string]*manifest.Healthcheck{
		"web": {Test: []string{"HTTP", "http://127.0.0.1:80/"}, Interval: 5, Retries: 2},
	})
	hc, ok := got["healthchecks"].(map[string]any)
	if !ok {
		t.Fatalf("healthchecks missing from %v", got)
	}
	web := hc["web"].(map[string]any)
	if web["interval"] != float64(5) || web["retries"] != float64(2) {
		t.Fatalf("web = %v", web)
	}
	if _, set := web["timeout"]; set {
		t.Fatalf("an unset time must be left out so the default applies: %v", web)
	}
	if _, has := hc["worker"]; has {
		t.Fatal("a service without a healthcheck must not be sent one")
	}
}

func TestReserveReleaseOmitsHealthchecksWhenNone(t *testing.T) {
	// An older registry rejects fields it doesn't know.
	if _, has := reserve(t, nil)["healthchecks"]; has {
		t.Fatal("healthchecks must be left out when no service has one")
	}
}
