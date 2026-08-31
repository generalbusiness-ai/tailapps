package buildinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func TestSanitizeRemoteScript(t *testing.T) {
	script := filepath.Join(repoRoot(t), "scripts", "sanitize-remote.sh")
	cases := []struct {
		name   string
		remote string
		want   string // empty means the script must refuse
	}{
		{"ssh scp form", "git@github.com:generalbusiness-ai/tailapps.git", "https://github.com/generalbusiness-ai/tailapps"},
		{"ssh url form", "ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"https", "https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"https with .git", "https://github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"https with user", "https://user@github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"https with credentials", "https://user:token@github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"other host refused", "https://gitlab.com/owner/repo.git", ""},
		{"local path refused", "/Users/someone/play/tailapp", ""},
		{"deep path refused", "https://github.com/owner/repo/extra", ""},
		{"owner only refused", "https://github.com/owner", ""},
		{"empty refused", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := exec.Command(script, tc.remote).Output()
			got := strings.TrimSpace(string(output))
			if tc.want == "" {
				if err == nil {
					t.Fatalf("sanitize accepted %q as %q; must refuse", tc.remote, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitize refused %q: %v", tc.remote, err)
			}
			if got != tc.want {
				t.Fatalf("sanitize(%q) = %q, want %q", tc.remote, got, tc.want)
			}
			if !githubURL.MatchString(got) {
				t.Fatalf("script output %q fails the runtime validator; the two halves disagree", got)
			}
		})
	}
}

// TestBuildScriptStampsSanitizedOrigin runs the release build against this
// checkout and asserts the wiring end to end: when the repository's origin
// sanitizes to a GitHub URL, the built binary carries exactly that ldflags
// stamp; when it does not, the binary is unstamped. Either way the producer
// and the runtime validator agree.
func TestBuildScriptStampsSanitizedOrigin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	root := repoRoot(t)
	remoteBytes, _ := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	expected, _ := exec.Command(filepath.Join(root, "scripts", "sanitize-remote.sh"), strings.TrimSpace(string(remoteBytes))).Output()
	want := strings.TrimSpace(string(expected))

	output := filepath.Join(t.TempDir(), "tailapp")
	build := exec.Command(filepath.Join(root, "scripts", "build.sh"), output)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build.sh: %v", err)
	}
	info, err := exec.Command("go", "version", "-m", output).Output()
	if err != nil {
		t.Fatal(err)
	}
	stamped := strings.Contains(string(info), "buildinfo.stampedSourceURL="+want)
	if want != "" && !stamped {
		t.Fatalf("origin sanitizes to %q but the built binary carries no such stamp:\n%s", want, info)
	}
	if want == "" && strings.Contains(string(info), "stampedSourceURL=") {
		t.Fatalf("origin does not sanitize but the binary is stamped:\n%s", info)
	}
}

func TestBuildScriptStampsExplicitReleaseVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	root := repoRoot(t)
	output := filepath.Join(t.TempDir(), "tailapp")
	build := exec.Command(filepath.Join(root, "scripts", "build.sh"), output)
	build.Env = append(os.Environ(), "TAILAPPS_VERSION=1.2.3")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build.sh: %v", err)
	}
	info, err := exec.Command("go", "version", "-m", output).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "buildinfo.stampedVersion=1.2.3") {
		t.Fatalf("release version stamp missing:\n%s", info)
	}
}
