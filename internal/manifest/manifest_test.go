package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectOrLoad_ExplicitManifest(t *testing.T) {
	tmpDir := t.TempDir()
	manifestContent := `
services:
  - name: web
    dockerfile: custom.Dockerfile
    context: ./src
`
	manifestPath := filepath.Join(tmpDir, "custom.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		ManifestPath: manifestPath,
		ExplicitFile: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "web" {
		t.Fatalf("expected web service, got %+v", m.Services)
	}
	if m.Services[0].Dockerfile != "custom.Dockerfile" || m.Services[0].Context != "./src" {
		t.Fatalf("unexpected service config: %+v", m.Services[0])
	}
	if desc != "file: "+manifestPath {
		t.Fatalf("unexpected desc: %s", desc)
	}
}

func TestDetectOrLoad_ExplicitComposeViaFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  app:
    build: .
`
	composePath := filepath.Join(tmpDir, "docker-compose.prod.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		ManifestPath: composePath,
		ExplicitFile: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "app" {
		t.Fatalf("expected app service, got %+v", m.Services)
	}
	if desc != "file: "+composePath {
		t.Fatalf("unexpected desc: %s", desc)
	}
}

func TestDetectOrLoad_ExplicitComposeFlag(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  worker:
    build: ./worker
`
	composePath := filepath.Join(tmpDir, "my-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		ComposePath: composePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "worker" {
		t.Fatalf("expected worker service, got %+v", m.Services)
	}
	if desc != "compose file: "+composePath {
		t.Fatalf("unexpected desc: %s", desc)
	}
}

func TestDetectOrLoad_DefaultNodexaYaml(t *testing.T) {
	tmpDir := t.TempDir()
	manifestContent := `
services:
  - name: api
`
	if err := os.WriteFile(filepath.Join(tmpDir, "nodexa.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "api" {
		t.Fatalf("expected api service, got %+v", m.Services)
	}
	if desc != "manifest: nodexa.yaml" {
		t.Fatalf("unexpected desc: %s", desc)
	}
}

func TestDetectOrLoad_DockerfileOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte("FROM alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(m.Services))
	}
	expectedName := sanitizeServiceName(filepath.Base(tmpDir))
	if m.Services[0].Name != expectedName {
		t.Fatalf("expected %s, got %s", expectedName, m.Services[0].Name)
	}
	if m.Services[0].Dockerfile != "Dockerfile" || m.Services[0].Context != "." {
		t.Fatalf("unexpected service config: %+v", m.Services[0])
	}
	if desc == "" {
		t.Fatal("expected non-empty desc")
	}
}

func TestDetectOrLoad_DockerfileWithServiceOverride(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte("FROM alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{
		WorkingDir:  tmpDir,
		ServiceName: "my-custom-service",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "my-custom-service" {
		t.Fatalf("expected my-custom-service, got %+v", m.Services)
	}
}

func TestDetectOrLoad_DockerComposeWithRootDockerfile(t *testing.T) {
	// Like Iot_backend_service: docker-compose has backend with image:, and Dockerfile in root
	tmpDir := t.TempDir()
	composeContent := `
services:
  backend:
    image: ghcr.io/adhuldas/iot_backend_service:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM python:3.12-slim\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, desc, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "backend" {
		t.Fatalf("expected backend service inferred from docker-compose.yml, got %+v", m.Services)
	}
	if m.Services[0].Dockerfile != "Dockerfile" || m.Services[0].Context != "." {
		t.Fatalf("unexpected service config: %+v", m.Services[0])
	}
	if desc != "compose file: docker-compose.yml" {
		t.Fatalf("unexpected desc: %s", desc)
	}
}

func TestDetectOrLoad_DockerComposeWithBuild(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  api:
    build:
      context: ./api
      dockerfile: custom.Dockerfile
  worker:
    build: ./worker
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 2 {
		t.Fatalf("expected 2 services, got %d: %+v", len(m.Services), m.Services)
	}

	names := m.ServiceNames()
	if names[0] != "api" || names[1] != "worker" {
		t.Fatalf("expected [api, worker], got %v", names)
	}
	if m.Services[0].Context != "./api" || m.Services[0].Dockerfile != "custom.Dockerfile" {
		t.Fatalf("unexpected api config: %+v", m.Services[0])
	}
	if m.Services[1].Context != "./worker" || m.Services[1].Dockerfile != "Dockerfile" {
		t.Fatalf("unexpected worker config: %+v", m.Services[1])
	}
}

func TestDetectOrLoad_DockerComposeSubdirDockerfiles(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  web:
    image: web:latest
  api:
    image: api:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "web", "Dockerfile"), []byte("FROM node:alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "api"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "api", "Dockerfile"), []byte("FROM python:alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(m.Services))
	}
	names := m.ServiceNames()
	if names[0] != "web" || names[1] != "api" {
		t.Fatalf("expected [web, api], got %v", names)
	}
}

func TestDetectOrLoad_DockerComposeWithServiceFilter(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  api:
    build: ./api
  worker:
    build: ./worker
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{
		WorkingDir:  tmpDir,
		ServiceName: "worker",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "worker" {
		t.Fatalf("expected 1 service (worker), got %+v", m.Services)
	}
}

func TestDetectOrLoad_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, _, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
	})
	if err == nil {
		t.Fatal("expected error when nothing deployable found, got nil")
	}
}

func TestDetectOrLoad_ManifestWithPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	manifestContent := `
platform: linux/arm/v7
services:
  - name: backend
    dockerfile: Dockerfile
`
	manifestPath := filepath.Join(tmpDir, "nodexa.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{WorkingDir: tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Platform != "linux/arm/v7" {
		t.Fatalf("expected platform linux/arm/v7, got %+v", m.Services)
	}
}

func TestDetectOrLoad_ComposeWithPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  backend:
    platform: linux/arm/v7
    build: .
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{WorkingDir: tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Platform != "linux/arm/v7" {
		t.Fatalf("expected platform linux/arm/v7 from compose, got %+v", m.Services)
	}
}

func TestDetectOrLoad_CLIPlatformOverride(t *testing.T) {
	tmpDir := t.TempDir()
	manifestContent := `
services:
  - name: backend
    platform: linux/amd64
`
	manifestPath := filepath.Join(tmpDir, "nodexa.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, _, err := DetectOrLoad(DetectOptions{
		WorkingDir: tmpDir,
		Platform:   "linux/arm/v7",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Services) != 1 || m.Services[0].Platform != "linux/arm/v7" {
		t.Fatalf("expected CLI platform linux/arm/v7 to override manifest, got %+v", m.Services)
	}
}

