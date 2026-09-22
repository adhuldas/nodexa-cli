// Package build wraps docker CLI commands for building and pushing images.
// Follows nodexa-agent's established pattern of shelling out to purpose-built
// tools rather than reimplementing OCI image handling.
package build

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// resolveDockerfile returns the Dockerfile path to pass to docker CLI.
// If dockerfile is a relative path and filepath.Join(context, dockerfile) exists,
// it uses filepath.Join(context, dockerfile) so docker CLI doesn't mistakenly resolve
// -f relative to CWD instead of the build context directory.
func resolveDockerfile(context, dockerfile string) string {
	if filepath.IsAbs(dockerfile) {
		return dockerfile
	}
	if candidate := filepath.Join(context, dockerfile); candidate != dockerfile {
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
	}
	return dockerfile
}

// Build runs `docker build` for the given image reference.
// If platform is non-empty, --platform is passed to docker build.
func Build(imageRef, dockerfile, context, platform string) error {
	dockerfile = resolveDockerfile(context, dockerfile)
	var args []string
	if platform != "" {
		fmt.Printf("  → building %s (platform=%s, dockerfile=%s, context=%s)\n", imageRef, platform, dockerfile, context)
		args = []string{"build", "--platform", platform, "-t", imageRef, "-f", dockerfile, context}
	} else {
		fmt.Printf("  → building %s (dockerfile=%s, context=%s)\n", imageRef, dockerfile, context)
		args = []string{"build", "-t", imageRef, "-f", dockerfile, context}
	}
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed for %s: %w", imageRef, err)
	}
	return nil
}

// BuildxPush runs `docker buildx build --platform ... --push` for multi-platform images,
// writes metadata to a temp file, and extracts the manifest digest.
func BuildxPush(imageRef, dockerfile, context, platform string) (string, error) {
	dockerfile = resolveDockerfile(context, dockerfile)
	fmt.Printf("  → building & pushing multi-platform %s (platform=%s)\n", imageRef, platform)
	tmpFile, err := os.CreateTemp("", "nodex-buildx-*.json")
	if err != nil {
		return "", fmt.Errorf("creating temp file for buildx metadata: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	args := []string{
		"buildx", "build",
		"--platform", platform,
		"-t", imageRef,
		"-f", dockerfile,
		"--push",
		"--metadata-file", tmpPath,
		context,
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker buildx build --push failed for %s: %w", imageRef, err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("reading buildx metadata: %w", err)
	}

	var meta struct {
		Digest string `json:"containerimage.digest"`
	}
	if err := json.Unmarshal(data, &meta); err == nil && meta.Digest != "" {
		return meta.Digest, nil
	}

	digest := extractDigest(string(data))
	if digest != "" {
		return digest, nil
	}

	return "", fmt.Errorf("could not extract digest from buildx metadata for %s", imageRef)
}

var digestRegex = regexp.MustCompile(`sha256:[a-f0-9]{64}`)

func extractDigest(s string) string {
	return digestRegex.FindString(s)
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

// PullTagPush pulls an existing image from an external registry, tags it for
// the target release repository, and pushes it to the Nodexa registry, returning the digest.
func PullTagPush(sourceImage, targetRef, platform string) (string, error) {
	var pullArgs []string
	if platform != "" {
		fmt.Printf("  → pulling %s (platform=%s)\n", sourceImage, platform)
		pullArgs = []string{"pull", "--platform", platform, sourceImage}
	} else {
		fmt.Printf("  → pulling %s\n", sourceImage)
		pullArgs = []string{"pull", sourceImage}
	}

	pullCmd := exec.Command("docker", pullArgs...)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return "", fmt.Errorf("docker pull failed for %s: %w", sourceImage, err)
	}

	fmt.Printf("  → tagging %s → %s\n", sourceImage, targetRef)
	tagCmd := exec.Command("docker", "tag", sourceImage, targetRef)
	tagCmd.Stdout = os.Stdout
	tagCmd.Stderr = os.Stderr
	if err := tagCmd.Run(); err != nil {
		return "", fmt.Errorf("docker tag failed from %s to %s: %w", sourceImage, targetRef, err)
	}

	return Push(targetRef)
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
