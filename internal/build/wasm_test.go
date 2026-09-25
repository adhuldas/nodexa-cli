package build

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRegistry is a registry:2-style API behind token auth, keeping what's
// pushed to it.
func fakeRegistry(t *testing.T) (*httptest.Server, map[string][]byte) {
	blobs := map[string][]byte{}
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			user, pass, _ := r.BasicAuth()
			if user != "nodex" || pass != "api-token" || r.URL.Query().Get("scope") != "repository:fleet/sensor:pull,push" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"token":"tok"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/fleet/sensor/blobs/uploads/":
			w.Header().Set("Location", "/v2/fleet/sensor/blobs/uploads/u1?state=x")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v2/fleet/sensor/blobs/uploads/"):
			d := r.URL.Query().Get("digest")
			if d != digestOf(body) || r.URL.Query().Get("state") != "x" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			blobs[d] = body
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/v2/fleet/sensor/manifests/7":
			blobs["manifest"] = body
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	old := wasmHTTP
	wasmHTTP = srv.Client()
	t.Cleanup(func() { wasmHTTP = old })
	return srv, blobs
}

func TestPushWasm(t *testing.T) {
	srv, blobs := fakeRegistry(t)
	module := []byte("\x00asm\x01\x00\x00\x00")
	path := filepath.Join(t.TempDir(), "sensor.wasm")
	if err := os.WriteFile(path, module, 0o644); err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(srv.URL, "https://")

	digest, err := PushWasm(host+"/fleet/sensor:7", path, "nodex", "api-token")
	if err != nil {
		t.Fatal(err)
	}
	if digest != digestOf(blobs["manifest"]) {
		t.Fatalf("digest %s is not the manifest's", digest)
	}
	var m struct {
		Config struct{ MediaType, Digest string }
		Layers []struct {
			MediaType, Digest string
			Size              int
		}
	}
	if err := json.Unmarshal(blobs["manifest"], &m); err != nil {
		t.Fatal(err)
	}
	if m.Config.MediaType != WasmConfigMediaType || blobs[m.Config.Digest] == nil {
		t.Fatalf("bad config: %+v", m.Config)
	}
	if len(m.Layers) != 1 || m.Layers[0].MediaType != WasmLayerMediaType ||
		string(blobs[m.Layers[0].Digest]) != string(module) || m.Layers[0].Size != len(module) {
		t.Fatalf("bad layers: %+v", m.Layers)
	}
}

func TestPushWasmRejectsNonWasm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.wasm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PushWasm("example.com/a/b:1", path, "u", "p"); err == nil {
		t.Fatal("expected an error for a non-WebAssembly file")
	}
}
