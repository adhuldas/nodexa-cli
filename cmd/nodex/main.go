// nodex — CLI for pushing container images to nodexa-registry.
//
// Usage:
//
//	nodex push [--file nodexa.yml] [--fleet-id FLEET] [--registry HOST] [--token TOKEN]
//
// The push command:
//  1. Parses nodexa.yml from CWD (or --file path)
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

func main() {
	rootCmd := &cobra.Command{
		Use:   "nodex",
		Short: "Nodexa Deploy CLI",
		Long:  "CLI for building and pushing container images to a nodexa-registry instance.",
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
		fleetID      string
		registryHost string
		registryURL  string
		apiToken     string
	)

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Build and push images for a release",
		Long: `Reads a nodexa.yml manifest, reserves a release on nodexa-registry,
builds each service's Docker image, pushes it, and completes the release.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(manifestFile, fleetID, registryHost, registryURL, apiToken)
		},
	}

	cmd.Flags().StringVarP(&manifestFile, "file", "f", "nodexa.yml", "Path to the nodexa.yml manifest")
	cmd.Flags().StringVar(&fleetID, "fleet-id", envOrDefault("NODEXA_FLEET_ID", ""), "Fleet ID to push to (required)")
	cmd.Flags().StringVar(&registryHost, "registry", envOrDefault("NODEXA_REGISTRY_HOST", "localhost:5000"), "Registry host (for docker login)")
	cmd.Flags().StringVar(&registryURL, "registry-url", envOrDefault("NODEXA_REGISTRY_URL", "http://localhost:8000"), "nodexa-registry API base URL")
	cmd.Flags().StringVar(&apiToken, "token", envOrDefault("NODEXA_API_TOKEN", ""), "API token for authentication (required)")

	_ = cmd.MarkFlagRequired("fleet-id")
	_ = cmd.MarkFlagRequired("token")

	return cmd
}

func runPush(manifestFile, fleetID, registryHost, registryURL, apiToken string) error {
	// 1. Parse manifest.
	fmt.Printf("📋 Reading manifest: %s\n", manifestFile)
	m, err := manifest.Load(manifestFile)
	if err != nil {
		return err
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
