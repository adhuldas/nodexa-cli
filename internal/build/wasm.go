package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Media types of a WebAssembly OCI artifact, as the Nodexa ESP32 firmware
// (and wasmtime/WasmEdge-style tooling) reads them.
const (
	WasmConfigMediaType = "application/vnd.wasm.config.v0+json"
	WasmLayerMediaType  = "application/wasm"
	ociManifestType     = "application/vnd.oci.image.manifest.v1+json"
)

var wasmHTTP = &http.Client{Timeout: 5 * time.Minute}

// PushWasm pushes the WebAssembly module at path to imageRef
// ("host/repo:tag") as a single-layer OCI artifact and returns the
// manifest digest. It talks to the registry's HTTP API directly (the
// token flow `docker login` would use), so Docker isn't needed.
func PushWasm(imageRef, path, username, password string) (string, error) {
	module, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if len(module) < 8 || !bytes.Equal(module[:4], []byte("\x00asm")) {
		return "", fmt.Errorf("%s is not a WebAssembly module", path)
	}
	host, repo, tag, err := splitRef(imageRef)
	if err != nil {
		return "", err
	}
	fmt.Printf("  → pushing %s (%s, %d bytes)\n", imageRef, path, len(module))

	r := &wasmRegistry{base: "https://" + host, repo: repo, username: username, password: password}
	config := []byte("{}")
	for _, blob := range [][]byte{config, module} {
		if err := r.uploadBlob(blob); err != nil {
			return "", err
		}
	}
	manifest, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     ociManifestType,
		"artifactType":  WasmConfigMediaType,
		"config":        descriptor(WasmConfigMediaType, config),
		"layers":        []any{descriptor(WasmLayerMediaType, module)},
	})
	resp, err := r.do(http.MethodPut, "/manifests/"+tag, ociManifestType, manifest)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", r.fail("pushing the manifest", resp)
	}
	return digestOf(manifest), nil
}

func descriptor(mediaType string, blob []byte) map[string]any {
	return map[string]any{"mediaType": mediaType, "digest": digestOf(blob), "size": len(blob)}
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func splitRef(ref string) (host, repo, tag string, err error) {
	slash := strings.IndexByte(ref, '/')
	if slash <= 0 {
		return "", "", "", fmt.Errorf("image reference %q has no registry host", ref)
	}
	host, rest := ref[:slash], ref[slash+1:]
	tag = "latest"
	if colon := strings.LastIndexByte(rest, ':'); colon > 0 {
		rest, tag = rest[:colon], rest[colon+1:]
	}
	return host, rest, tag, nil
}

type wasmRegistry struct {
	base, repo         string
	username, password string
	auth               string // "Bearer <token>" once logged in
}

// do sends a request under /v2/<repo>, logging in (and retrying once)
// when the registry answers 401 with a token challenge.
func (r *wasmRegistry) do(method, path, contentType string, body []byte) (*http.Response, error) {
	target := path
	if !strings.HasPrefix(path, "http") {
		target = r.base + "/v2/" + r.repo + path
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(method, target, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if r.auth != "" {
			req.Header.Set("Authorization", r.auth)
		}
		resp, err := wasmHTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, target, err)
		}
		if resp.StatusCode != http.StatusUnauthorized || attempt > 0 {
			return resp, nil
		}
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if err := r.login(challenge); err != nil {
			return nil, err
		}
	}
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

func (r *wasmRegistry) login(challenge string) error {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return fmt.Errorf("registry %s refused the credentials", r.base)
	}
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[m[1]] = m[2]
	}
	q := url.Values{}
	if params["service"] != "" {
		q.Set("service", params["service"])
	}
	q.Set("scope", "repository:"+r.repo+":pull,push")
	req, err := http.NewRequest(http.MethodGet, params["realm"]+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("registry token realm %q: %w", params["realm"], err)
	}
	req.SetBasicAuth(r.username, r.password)
	resp, err := wasmHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("logging into %s: %w", r.base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r.fail("logging in", resp)
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("logging into %s: %w", r.base, err)
	}
	if tok.Token == "" {
		tok.Token = tok.AccessToken
	}
	r.auth = "Bearer " + tok.Token
	return nil
}

func (r *wasmRegistry) uploadBlob(blob []byte) error {
	digest := digestOf(blob)
	if resp, err := r.do(http.MethodHead, "/blobs/"+digest, "", nil); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}
	resp, err := r.do(http.MethodPost, "/blobs/uploads/", "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return r.fail("starting an upload", resp)
	}
	loc, err := resp.Request.URL.Parse(resp.Header.Get("Location"))
	if err != nil {
		return fmt.Errorf("registry upload location: %w", err)
	}
	q := loc.Query()
	q.Set("digest", digest)
	loc.RawQuery = q.Encode()
	resp, err = r.do(http.MethodPut, loc.String(), "application/octet-stream", blob)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return r.fail("uploading "+digest, resp)
	}
	return nil
}

func (r *wasmRegistry) fail(what string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("%s on %s: %s %s", what, r.base, resp.Status, strings.TrimSpace(string(body)))
}
