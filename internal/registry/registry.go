package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Credential holds login information for an external container registry.
type Credential struct {
	Registry string `yaml:"registry"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// FindFile searches for a registry credentials YAML file.
// If explicitPath is given, it verifies existence.
// Otherwise, it checks standard filenames in workingDir.
func FindFile(workingDir, explicitPath string) (string, error) {
	if explicitPath != "" {
		p := explicitPath
		if !filepath.IsAbs(p) && workingDir != "" && workingDir != "." {
			p = filepath.Join(workingDir, p)
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("registry file %q not found: %w", explicitPath, err)
		}
		return p, nil
	}

	dir := workingDir
	if dir == "" {
		dir = "."
	}

	standardNames := []string{
		"registry.yml",
		"registry.yaml",
		"registries.yml",
		"registries.yaml",
		".registry.yml",
		".registry.yaml",
		".registries.yml",
		".registries.yaml",
	}

	for _, name := range standardNames {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	return "", nil
}

// LoadCredentials parses credentials from a registry YAML file.
// It supports multiple formats:
// 1. Map of registry host to creds:
//      ghcr.io:
//        username: user
//        password: pass (or token / auth)
// 2. Wrapped in registries: or auths:
//      registries:
//        ghcr.io:
//          username: user
//          password: pass
// 3. List of registry objects:
//      - registry: ghcr.io
//        username: user
//        password: pass
func LoadCredentials(path string) ([]Credential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading registry file %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing registry file %s: %w", path, err)
	}

	if len(root.Content) == 0 {
		return nil, nil
	}

	doc := root.Content[0]

	// Format 3a: top-level sequence/list
	if doc.Kind == yaml.SequenceNode {
		return parseCredentialList(doc)
	}

	// Mapping node
	if doc.Kind == yaml.MappingNode {
		// Check for wrapped keys: "registries" or "auths"
		for i := 0; i < len(doc.Content)-1; i += 2 {
			k := doc.Content[i].Value
			val := doc.Content[i+1]
			if k == "registries" || k == "auths" {
				if val.Kind == yaml.SequenceNode {
					return parseCredentialList(val)
				}
				if val.Kind == yaml.MappingNode {
					return parseCredentialMap(val)
				}
			}
		}

		// Otherwise top-level map of host -> creds
		return parseCredentialMap(doc)
	}

	return nil, fmt.Errorf("unsupported registry file format in %s", path)
}

func parseCredentialMap(mapNode *yaml.Node) ([]Credential, error) {
	var creds []Credential
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		regHost := mapNode.Content[i].Value
		valNode := mapNode.Content[i+1]

		if valNode.Kind != yaml.MappingNode {
			continue
		}

		var username, password string
		for j := 0; j < len(valNode.Content)-1; j += 2 {
			key := valNode.Content[j].Value
			v := valNode.Content[j+1].Value
			switch key {
			case "username", "user":
				username = v
			case "password", "token", "auth", "secret":
				password = v
			}
		}

		if username != "" && password != "" {
			creds = append(creds, Credential{
				Registry: regHost,
				Username: username,
				Password: password,
			})
		}
	}
	return creds, nil
}

func parseCredentialList(seqNode *yaml.Node) ([]Credential, error) {
	var creds []Credential
	for _, item := range seqNode.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}

		var regHost, username, password string
		for j := 0; j < len(item.Content)-1; j += 2 {
			key := item.Content[j].Value
			v := item.Content[j+1].Value
			switch key {
			case "registry", "host", "server":
				regHost = v
			case "username", "user":
				username = v
			case "password", "token", "auth", "secret":
				password = v
			}
		}

		if regHost != "" && username != "" && password != "" {
			creds = append(creds, Credential{
				Registry: regHost,
				Username: username,
				Password: password,
			})
		}
	}
	return creds, nil
}
