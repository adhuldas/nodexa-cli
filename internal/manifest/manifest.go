// Package manifest parses nodexa.yml manifests or auto-detects services
// from standard application repositories (Dockerfile, docker-compose.yml).
// The format mirrors nodexa-backend's Deployment.services shape — each
// service maps to one container image.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Service describes a single container image to build or pull within a release.
type Service struct {
	// Name is the service name (e.g. "web", "worker", "backend"), matching
	// nodexa-backend's ServiceSpec.name.
	Name string `yaml:"name"`
	// Dockerfile is the path to the Dockerfile, relative to the
	// context or working directory (default: "Dockerfile").
	Dockerfile string `yaml:"dockerfile,omitempty"`
	// Context is the Docker build context directory (default: ".").
	Context string `yaml:"context,omitempty"`
	// Image is an existing pre-built container image reference (e.g. "ghcr.io/org/repo:tag").
	// When specified without a build block, nodex pulls, tags, and pushes this image
	// without rebuilding from a Dockerfile.
	Image string `yaml:"image,omitempty"`
	// Platform is the target platform for the container image (e.g. "linux/arm/v7", "linux/arm64").
	Platform string `yaml:"platform,omitempty"`
}

// Manifest is the top-level structure of a nodexa.yml file or auto-detected release.
type Manifest struct {
	Platform string    `yaml:"platform,omitempty"`
	Services []Service `yaml:"services"`
}

// DetectOptions holds configuration for finding or synthesizing a Manifest.
type DetectOptions struct {
	ManifestPath string // User-provided path via -f/--file, or default "nodexa.yml"
	ExplicitFile bool   // True if user explicitly passed -f/--file flag
	ComposePath  string // User-provided compose file path via -c/--compose-file
	ServiceName  string // User-provided service name via -s/--service
	Dockerfile   string // User-provided Dockerfile path via --dockerfile
	Context      string // User-provided build context via --context
	Platform     string // User-provided platform via -p/--platform
	WorkingDir   string // Working directory to search (defaults to "." if empty)
}

// Load reads and parses a manifest or docker-compose file from the given path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	// Check if this is a docker-compose format (services is a mapping)
	// or nodexa.yml format (services is a sequence).
	dir := filepath.Dir(path)
	if isComposeFormat(&root) {
		return LoadComposeFromNode(&root, dir, "")
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
		if m.Services[i].Image == "" {
			if m.Services[i].Dockerfile == "" {
				m.Services[i].Dockerfile = "Dockerfile"
			}
			if m.Services[i].Context == "" {
				m.Services[i].Context = "."
			}
		}
		if m.Services[i].Platform == "" && m.Platform != "" {
			m.Services[i].Platform = m.Platform
		}
	}

	return &m, nil
}

// LoadCompose reads a docker-compose file and extracts buildable services.
func LoadCompose(path string, targetService string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading compose file %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	return LoadComposeFromNode(&root, dir, targetService)
}

// LoadComposeFromNode extracts buildable services from a parsed YAML Node of a compose file.
func LoadComposeFromNode(root *yaml.Node, workingDir string, targetService string) (*Manifest, error) {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid compose file: root must be a mapping")
	}

	doc := root.Content[0]
	var servicesNode *yaml.Node
	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Value == "services" {
			servicesNode = doc.Content[i+1]
			break
		}
	}

	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid compose file: missing or invalid 'services' section")
	}

	var services []Service
	var nonBuildServiceNames []string
	svcPlatforms := make(map[string]string)

	for i := 0; i < len(servicesNode.Content)-1; i += 2 {
		svcName := servicesNode.Content[i].Value
		svcVal := servicesNode.Content[i+1]

		// Filter by target service if specified
		if targetService != "" && svcName != targetService {
			continue
		}

		var buildNode *yaml.Node
		var svcPlatform string
		var svcImage string
		if svcVal.Kind == yaml.MappingNode {
			for j := 0; j < len(svcVal.Content)-1; j += 2 {
				key := svcVal.Content[j].Value
				if key == "build" {
					buildNode = svcVal.Content[j+1]
				} else if key == "platform" {
					svcPlatform = svcVal.Content[j+1].Value
				} else if key == "image" {
					svcImage = svcVal.Content[j+1].Value
				}
			}
		}
		if svcPlatform != "" {
			svcPlatforms[svcName] = svcPlatform
		}

		if buildNode != nil {
			svc := Service{
				Name:       svcName,
				Dockerfile: "Dockerfile",
				Context:    ".",
				Platform:   svcPlatform,
			}

			if buildNode.Kind == yaml.ScalarNode {
				svc.Context = buildNode.Value
			} else if buildNode.Kind == yaml.MappingNode {
				for k := 0; k < len(buildNode.Content)-1; k += 2 {
					key := buildNode.Content[k].Value
					val := buildNode.Content[k+1].Value
					switch key {
					case "context":
						svc.Context = val
					case "dockerfile":
						svc.Dockerfile = val
					case "platform":
						svc.Platform = val
					}
				}
			}

			services = append(services, svc)
		} else if svcImage != "" {
			// Service has an existing pre-built image reference
			services = append(services, Service{
				Name:     svcName,
				Image:    svcImage,
				Platform: svcPlatform,
			})
		} else {
			// Check if a subdirectory matching service name has a Dockerfile
			subDirDocker := filepath.Join(workingDir, svcName, "Dockerfile")
			if fi, err := os.Stat(subDirDocker); err == nil && !fi.IsDir() {
				services = append(services, Service{
					Name:       svcName,
					Dockerfile: "Dockerfile",
					Context:    svcName,
					Platform:   svcPlatform,
				})
			} else {
				nonBuildServiceNames = append(nonBuildServiceNames, svcName)
			}
		}
	}

	// Check if any service already uses root context
	hasRootContext := false
	for _, s := range services {
		if s.Context == "." || s.Context == "" {
			hasRootContext = true
			break
		}
	}

	// If there's a root Dockerfile and no service is already using the root context,
	// match a non-build service from compose to it (e.g. main app in repo).
	rootDocker := filepath.Join(workingDir, "Dockerfile")
	if fi, err := os.Stat(rootDocker); err == nil && !fi.IsDir() && !hasRootContext && len(nonBuildServiceNames) > 0 {
		var matchedName string
		dirName := sanitizeServiceName(dirBaseName(workingDir))

		if targetService != "" {
			for _, name := range nonBuildServiceNames {
				if name == targetService {
					matchedName = name
					break
				}
			}
		} else {
			// 1. Check if any non-build service matches the directory name (e.g. smart-printer-firmware)
			for _, name := range nonBuildServiceNames {
				if sanitizeServiceName(name) == dirName {
					matchedName = name
					break
				}
			}
			// 2. If not matched, but there's only 1 non-build service
			if matchedName == "" && len(nonBuildServiceNames) == 1 {
				matchedName = nonBuildServiceNames[0]
			}
			// 3. Substring match with directory name
			if matchedName == "" {
				for _, name := range nonBuildServiceNames {
					sn := sanitizeServiceName(name)
					if strings.Contains(dirName, sn) || strings.Contains(sn, dirName) {
						matchedName = name
						break
					}
				}
			}
			// 4. If no other services had build blocks at all, fallback to first non-build service
			if matchedName == "" && len(services) == 0 {
				matchedName = nonBuildServiceNames[0]
			}
		}

		if matchedName != "" {
			services = append(services, Service{
				Name:       matchedName,
				Dockerfile: "Dockerfile",
				Context:    ".",
				Platform:   svcPlatforms[matchedName],
			})
		}
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no buildable services or Dockerfiles found in compose file")
	}

	// Sort services in the order they appeared in compose file
	order := make(map[string]int)
	idx := 0
	for i := 0; i < len(servicesNode.Content)-1; i += 2 {
		order[servicesNode.Content[i].Value] = idx
		idx++
	}
	sort.SliceStable(services, func(i, j int) bool {
		return order[services[i].Name] < order[services[j].Name]
	})

	return &Manifest{Services: services}, nil
}

// DetectOrLoad resolves a Manifest by loading a specified manifest / compose file
// or auto-detecting services from the repository layout (nodexa.yml, compose, Dockerfile).
func DetectOrLoad(opts DetectOptions) (*Manifest, string, error) {
	workingDir := opts.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}

	// 1. Explicit compose file requested via -c/--compose-file.
	if opts.ComposePath != "" {
		targetPath := opts.ComposePath
		if !filepath.IsAbs(targetPath) && workingDir != "." {
			targetPath = filepath.Join(workingDir, targetPath)
		}
		m, err := LoadCompose(targetPath, opts.ServiceName)
		if err != nil {
			return nil, "", err
		}
		return applyPlatform(m, opts.Platform), fmt.Sprintf("compose file: %s", opts.ComposePath), nil
	}

	// 2. Explicit manifest file requested via -f/--file.
	if opts.ExplicitFile {
		targetPath := opts.ManifestPath
		if !filepath.IsAbs(targetPath) && workingDir != "." {
			targetPath = filepath.Join(workingDir, targetPath)
		}
		m, err := Load(targetPath)
		if err != nil {
			return nil, "", err
		}
		return applyPlatform(m, opts.Platform), fmt.Sprintf("file: %s", opts.ManifestPath), nil
	}

	// 3. Discover default nodexa.yml or nodexa.yaml in working directory.
	for _, name := range []string{"nodexa.yml", "nodexa.yaml"} {
		p := filepath.Join(workingDir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			m, err := Load(p)
			if err != nil {
				return nil, "", err
			}
			return applyPlatform(m, opts.Platform), fmt.Sprintf("manifest: %s", name), nil
		}
	}

	// 4. Auto-detect from docker-compose files in working directory.
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		p := filepath.Join(workingDir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			m, err := LoadCompose(p, opts.ServiceName)
			if err == nil && len(m.Services) > 0 {
				return applyPlatform(m, opts.Platform), fmt.Sprintf("compose file: %s", name), nil
			}
		}
	}

	// 5. Auto-detect from Dockerfile in working directory or user flag.
	dockerfileName := "Dockerfile"
	if opts.Dockerfile != "" {
		dockerfileName = opts.Dockerfile
	}
	dockerfilePath := filepath.Join(workingDir, dockerfileName)
	if fi, err := os.Stat(dockerfilePath); err == nil && !fi.IsDir() {
		serviceName := opts.ServiceName
		var detectedVia string

		if serviceName != "" {
			detectedVia = fmt.Sprintf("Dockerfile (service: %s)", serviceName)
		} else {
			serviceName = sanitizeServiceName(dirBaseName(workingDir))
			detectedVia = fmt.Sprintf("Dockerfile (service: %s)", serviceName)
		}

		contextDir := "."
		if opts.Context != "" {
			contextDir = opts.Context
		}

		return applyPlatform(&Manifest{
			Services: []Service{
				{
					Name:       serviceName,
					Dockerfile: dockerfileName,
					Context:    contextDir,
				},
			},
		}, opts.Platform), detectedVia, nil
	}

	// 6. If user supplied both --service and --dockerfile flags, allow building even if not in workingDir.
	if opts.ServiceName != "" && opts.Dockerfile != "" {
		contextDir := "."
		if opts.Context != "" {
			contextDir = opts.Context
		}
		return applyPlatform(&Manifest{
			Services: []Service{
				{
					Name:       opts.ServiceName,
					Dockerfile: opts.Dockerfile,
					Context:    contextDir,
				},
			},
		}, opts.Platform), "CLI flags (--service, --dockerfile)", nil
	}

	// 7. Nothing found — return a clear and helpful error message.
	absDir, _ := filepath.Abs(workingDir)
	return nil, "", fmt.Errorf("no nodexa.yml manifest, docker-compose.yml, or Dockerfile found in %s\n\n"+
		"To push a release:\n"+
		"  • Run inside a repository containing a Dockerfile or docker-compose.yml\n"+
		"  • Provide a compose file with: nodex push -c docker-compose.yml\n"+
		"  • Provide a manifest with: nodex push -f nodexa.yml\n"+
		"  • Specify build flags: nodex push --service <name> --dockerfile <path>", absDir)
}

// ServiceNames returns a slice of just the service names, in order.
func (m *Manifest) ServiceNames() []string {
	names := make([]string, len(m.Services))
	for i, s := range m.Services {
		names[i] = s.Name
	}
	return names
}

// isComposeFormat checks whether the parsed YAML has a `services:` mapping (compose style)
// rather than a sequence (nodexa.yml style).
func isComposeFormat(root *yaml.Node) bool {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return false
	}
	doc := root.Content[0]
	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Value == "services" {
			return doc.Content[i+1].Kind == yaml.MappingNode
		}
	}
	return false
}

var invalidCharRegex = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizeServiceName formats a string to be a valid OCI/Nodexa service name:
// lowercase alphanumeric, underscores, and hyphens (max 63 chars).
func sanitizeServiceName(name string) string {
	name = strings.ToLower(name)
	name = invalidCharRegex.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-_.")
	if len(name) > 63 {
		name = name[:63]
		name = strings.Trim(name, "-_.")
	}
	if name == "" {
		return "app"
	}
	return name
}

// dirBaseName returns the base name of a directory.
func dirBaseName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "app"
	}
	base := filepath.Base(abs)
	if base == "." || base == "/" || base == "\\" {
		return "app"
	}
	return base
}

func applyPlatform(m *Manifest, platform string) *Manifest {
	if m == nil || platform == "" {
		return m
	}
	for i := range m.Services {
		m.Services[i].Platform = platform
	}
	return m
}
