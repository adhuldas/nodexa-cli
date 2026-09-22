// Package build wraps docker CLI commands for building and pushing images.
// Follows nodexa-agent's established pattern of shelling out to purpose-built
// tools rather than reimplementing OCI image handling.
package build

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Build runs `docker build` for the given image reference.
func Build(imageRef, dockerfile, context string) error {
	fmt.Printf("  → building %s (dockerfile=%s, context=%s)\n", imageRef, dockerfile, context)
	cmd := exec.Command("docker", "build", "-t", imageRef, "-f", dockerfile, context)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed for %s: %w", imageRef, err)
	}
	return nil
}

// Push runs `docker push` and parses the digest from the output.
// Docker outputs a line like "digest: sha256:abc123..." which we capture.
func Push(imageRef string) (string, error) {
	fmt.Printf("  → pushing %s\n", imageRef)
	cmd := exec.Command("docker", "push", imageRef)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("docker push failed to start for %s: %w", imageRef, err)
	}

	var digest string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		// Docker outputs: "<tag>: digest: sha256:... size: ..."
		if idx := strings.Index(line, "digest: sha256:"); idx != -1 {
			// Extract "sha256:..." up to the next space.
			rest := line[idx+len("digest: "):]
			if spaceIdx := strings.Index(rest, " "); spaceIdx != -1 {
				digest = rest[:spaceIdx]
			} else {
				digest = rest
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("docker push failed for %s: %w", imageRef, err)
	}

	if digest == "" {
		return "", fmt.Errorf("could not parse digest from docker push output for %s", imageRef)
	}
	return digest, nil
}

// Login runs `docker login` with the given credentials via --password-stdin.
func Login(registry, username, password string) error {
	cmd := exec.Command("docker", "login", registry, "-u", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(password)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker login to %s failed: %w", registry, err)
	}
	return nil
}
