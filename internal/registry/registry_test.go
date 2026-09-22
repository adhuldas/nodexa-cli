package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCredentials_MapFormat(t *testing.T) {
	tmpDir := t.TempDir()
	content := `
ghcr.io:
  username: adhuldas
  password: my-token-123
docker.io:
  user: testuser
  token: my-docker-token
`
	p := filepath.Join(tmpDir, "registry.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	creds, err := LoadCredentials(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(creds))
	}
}

func TestLoadCredentials_WrappedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	content := `
registries:
  ghcr.io:
    username: adhuldas
    token: ghp_abc123
`
	p := filepath.Join(tmpDir, "registry.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	creds, err := LoadCredentials(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if creds[0].Registry != "ghcr.io" || creds[0].Username != "adhuldas" || creds[0].Password != "ghp_abc123" {
		t.Fatalf("unexpected cred: %+v", creds[0])
	}
}

func TestLoadCredentials_ListFormat(t *testing.T) {
	tmpDir := t.TempDir()
	content := `
- registry: ghcr.io
  username: adhuldas
  secret: secret123
`
	p := filepath.Join(tmpDir, "registry.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	creds, err := LoadCredentials(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 1 || creds[0].Registry != "ghcr.io" {
		t.Fatalf("unexpected creds: %+v", creds)
	}
}

func TestFindFile(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "registry.yml")
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	found, err := FindFile(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != p {
		t.Fatalf("expected %s, got %s", p, found)
	}
}
