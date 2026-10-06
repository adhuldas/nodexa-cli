package nodexcli

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVersionMatchesTheVersionFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Version(), strings.TrimSpace(string(raw)); got != want {
		t.Fatalf("Version() = %q, VERSION file says %q", got, want)
	}
}

func TestVersionIsSemver(t *testing.T) {
	// scripts/release.sh builds, tags and uploads whatever this says.
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$`).MatchString(Version()) {
		t.Fatalf("VERSION %q must look like 1.2.3 (optionally 1.2.3-rc.1)", Version())
	}
}
