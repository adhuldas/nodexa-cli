// Package manifest parses the nodexa.yml file that describes which services
// to build and push. The format mirrors nodexa-backend's Deployment.services
// shape on purpose — each service maps to one container image.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Service describes a single container image to build within a release.
type Service struct {
	// Name is the service name (e.g. "web", "worker"), matching
	// nodexa-backend's ServiceSpec.name.
	Name string `yaml:"name"`
	// Dockerfile is the path to the Dockerfile, relative to the
	// nodexa.yml location (default: "Dockerfile").
	Dockerfile string `yaml:"dockerfile"`
	// Context is the Docker build context directory, relative to the
	// nodexa.yml location (default: ".").
	Context string `yaml:"context"`
}

// Manifest is the top-level structure of a nodexa.yml file.
type Manifest struct {
	Services []Service `yaml:"services"`
}

// Load reads and parses a nodexa.yml file from the given path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	if len(m.Services) == 0 {
		return nil, fmt.Errorf("manifest %s: at least one service is required", path)
	}

	// Apply defaults.
	for i := range m.Services {
		if m.Services[i].Name == "" {
			return nil, fmt.Errorf("manifest %s: service[%d].name is required", path, i)
		}
		if m.Services[i].Dockerfile == "" {
			m.Services[i].Dockerfile = "Dockerfile"
		}
		if m.Services[i].Context == "" {
			m.Services[i].Context = "."
		}
	}

	return &m, nil
}

// ServiceNames returns a slice of just the service names, in order.
func (m *Manifest) ServiceNames() []string {
	names := make([]string, len(m.Services))
	for i, s := range m.Services {
		names[i] = s.Name
	}
	return names
}
