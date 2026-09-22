// nodex — CLI for pushing container images to nodexa-registry.
//
// Usage:
//
//	nodex push --fleet-id <fleet-id> --token <token> [--service <name>]
//
// The push command:
//  1. Resolves services from nodexa.yml or auto-detects from repository (Dockerfile, compose)
//  2. Reserves a release via POST /v1/fleets/{fleet_id}/releases
//  3. Logs into the registry with the API token
//  4. For each service: docker build → docker push → capture digest
//  5. Completes the release via PATCH .../complete with digests
//     On any error → PATCH .../fail
package main

import (
	"fmt"
	"os"

	"github.com/adhuldas/nodexa-cli/internal/build"
	"github.com/adhuldas/nodexa-cli/internal/client"
	"github.com/adhuldas/nodexa-cli/internal/manifest"
	"github.com/spf13/cobra"
)

var Version = "0.1.2"

const (
	DefaultRegistryHost = "nodexa.elzora.tech"
	DefaultRegistryURL  = "https://nodexa.elzora.tech/registry"
)

func main() {
	rootCmd := &cobra.Command{
		Use:          "nodex",
		Short:        "Nodexa Deploy CLI",
		Long:         "CLI for building and pushing container images to a nodexa-registry instance.",
		Version:      Version,
		SilenceUsage: true,
	}

	pushCmd := newPushCmd()
	rootCmd.AddCommand(pushCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newPushCmd() *cobra.Command {
	var (
		manifestFile string
		composeFile  string
		serviceName  string
		dockerfile   string
		contextDir   string
		fleetID      string
		registryHost string
		registryURL  string
		apiToken     string
	)

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Build and push images for a release",
		Long: `Builds each service's Docker image, pushes it to the Nodexa registry,
and completes the release. Automatically detects Dockerfile or compose files
in your repository, or reads a nodexa.yml manifest.`,
		Example: `  nodex push --fleet-id 6aa7090a7f1a3400237fa78c --token <api-token>
  nodex push --fleet-id 6aa7090a7f1a3400237fa78c --service backend
  nodex push --fleet-id 6aa7090a7f1a3400237fa78c -c docker-compose.yml
  nodex push --fleet-id 6aa7090a7f1a3400237fa78c -f custom-nodexa.yml`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			explicitFile := cmd.Flags().Changed("file")
			return runPush(manifest.DetectOptions{
				ManifestPath: manifestFile,
				ExplicitFile: explicitFile,
				ComposePath:  composeFile,
				ServiceName:  serviceName,
				Dockerfile:   dockerfile,
				Context:      contextDir,
			}, fleetID, registryHost, registryURL, apiToken)
		},
	}

	cmd.Flags().StringVarP(&manifestFile, "file", "f", "nodexa.yml", "Path to the nodexa.yml manifest (optional if Dockerfile exists)")
	cmd.Flags().StringVarP(&composeFile, "compose-file", "c", "", "Path to docker-compose.yml file")
	cmd.Flags().StringVarP(&serviceName, "service", "s", "", "Service name override (for single-container repos without nodexa.yml)")
	cmd.Flags().StringVar(&dockerfile, "dockerfile", "", "Path to Dockerfile (defaults to Dockerfile in current directory)")
	cmd.Flags().StringVar(&contextDir, "context", "", "Docker build context directory (defaults to .)")
	cmd.Flags().StringVar(&fleetID, "fleet-id", envOrDefault("NODEXA_FLEET_ID", ""), "Fleet ID to push to (required)")
	cmd.Flags().StringVar(&apiToken, "token", envOrDefault("NODEXA_API_TOKEN", ""), "API token for authentication (required)")

	// Native registry defaults — official Nodexa cloud targets are built-in natively.
	cmd.Flags().StringVar(&registryHost, "registry", envOrDefault("NODEXA_REGISTRY_HOST", DefaultRegistryHost), "Registry host for image tags")
	cmd.Flags().StringVar(&registryURL, "registry-url", envOrDefault("NODEXA_REGISTRY_URL", DefaultRegistryURL), "nodexa-registry API base URL")
	_ = cmd.Flags().MarkHidden("registry")
	_ = cmd.Flags().MarkHidden("registry-url")

	_ = cmd.MarkFlagRequired("fleet-id")
	_ = cmd.MarkFlagRequired("token")

	return cmd
}

func runPush(opts manifest.DetectOptions, fleetID, registryHost, registryURL, apiToken string) error {
	// 1. Resolve manifest (explicit file, default nodexa.yml/yaml, or auto-detected).
	m, desc, err := manifest.DetectOrLoad(opts)
	if err != nil {
		return err
	}

	if opts.ExplicitFile || desc == "manifest: nodexa.yml" || desc == "manifest: nodexa.yaml" {
		fmt.Printf("📋 Reading manifest: %s\n", desc)
	} else {
		fmt.Printf("🔍 %s\n", desc)
	}
	fmt.Printf("   Found %d service(s): %v\n", len(m.Services), m.ServiceNames())

	// 2. Reserve release.
	fmt.Printf("\n📝 Reserving release for fleet %s...\n", fleetID)
	c := client.New(registryURL, apiToken)
	release, err := c.ReserveRelease(fleetID, m.ServiceNames())
	if err != nil {
		return fmt.Errorf("reserving release: %w", err)
	}
	fmt.Printf("   Reserved revision %d\n", release.Revision)

	// Build a name→imageRef lookup from the release response.
	imageRefs := make(map[string]string, len(release.Services))
	for _, s := range release.Services {
		imageRefs[s.Name] = s.ImageRef
	}

	// 3. Docker login.
	fmt.Printf("\n🔑 Logging into registry %s...\n", registryHost)
	if err := build.Login(registryHost, "nodex", apiToken); err != nil {
		_ = failRelease(c, fleetID, release.Revision)
		return err
	}

	// 4. Build + push each service.
	fmt.Printf("\n🔨 Building and pushing %d service(s)...\n", len(m.Services))
	digests := make(map[string]string, len(m.Services))
	for _, svc := range m.Services {
		ref, ok := imageRefs[svc.Name]
		if !ok {
			_ = failRelease(c, fleetID, release.Revision)
			return fmt.Errorf("no image ref returned for service %q", svc.Name)
		}

		if err := build.Build(ref, svc.Dockerfile, svc.Context); err != nil {
			_ = failRelease(c, fleetID, release.Revision)
			return err
		}

		digest, err := build.Push(ref)
		if err != nil {
			_ = failRelease(c, fleetID, release.Revision)
			return err
		}
		digests[svc.Name] = digest
		fmt.Printf("   ✅ %s → %s\n", svc.Name, digest)
	}

	// 5. Complete release.
	fmt.Printf("\n✅ Completing release revision %d...\n", release.Revision)
	completed, err := c.CompleteRelease(fleetID, release.Revision, digests)
	if err != nil {
		return fmt.Errorf("completing release: %w", err)
	}
	fmt.Printf("   Release %d is now %s\n", completed.Revision, completed.Status)
	fmt.Println("\n🎉 Push complete!")

	return nil
}

func failRelease(c *client.Client, fleetID string, revision int) error {
	fmt.Fprintf(os.Stderr, "\n❌ Marking release revision %d as failed...\n", revision)
	_, err := c.FailRelease(fleetID, revision)
	if err != nil {
		fmt.Fprintf(os.Stderr, "   Warning: failed to mark release as failed: %v\n", err)
	}
	return err
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
