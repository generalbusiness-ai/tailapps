package scripts_test

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testVersion = "0.0.0-test"

func TestReleaseAssetsAreVersionPinnedAndInstallable(t *testing.T) {
	root := repoRoot(t)
	dist := t.TempDir()
	run(t, root, nil, "sh", "scripts/release.sh", "v"+testVersion, dist)

	installer := filepath.Join(dist, "install.sh")
	installerBytes, err := os.ReadFile(installer)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(installerBytes), "@TAILAPPS_VERSION@") || !strings.Contains(string(installerBytes), "tailapps_version='"+testVersion+"'") {
		t.Fatalf("installer is not version pinned:\n%s", installerBytes)
	}
	upgradeBytes, err := os.ReadFile(filepath.Join(dist, "upgrade.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(upgradeBytes), "@TAILAPPS_VERSION@") || !strings.Contains(string(upgradeBytes), "tailapps_version='"+testVersion+"'") {
		t.Fatalf("upgrade asset is not version pinned:\n%s", upgradeBytes)
	}
	for _, name := range []string{
		"tailapps_" + testVersion + "_darwin_arm64.tar.gz",
		"tailapps_" + testVersion + "_darwin_amd64.tar.gz",
		"tailapps_" + testVersion + "_linux_arm64.tar.gz",
		"tailapps_" + testVersion + "_linux_amd64.tar.gz",
		"upgrade.sh", "checksums.txt", "SHA256SUMS",
	} {
		if _, err := os.Stat(filepath.Join(dist, name)); err != nil {
			t.Fatalf("release asset %s: %v", name, err)
		}
	}
	checksums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sha256sums, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checksums, sha256sums) || !strings.Contains(string(checksums), "install.sh") || !strings.Contains(string(checksums), "upgrade.sh") {
		t.Fatalf("checksum manifests do not cover the same release asset set")
	}

	releaseRoot := t.TempDir()
	releaseDir := filepath.Join(releaseRoot, "download", "v"+testVersion)
	if err := os.MkdirAll(releaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	archiveName := archiveForHost(t)
	for _, name := range []string{"checksums.txt", "install.sh", archiveName} {
		if err := os.Link(filepath.Join(dist, name), filepath.Join(releaseDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"checksums.txt.sig", "checksums.txt.bundle"} {
		if err := os.WriteFile(filepath.Join(releaseDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	cosign := filepath.Join(fakeBin, "cosign")
	if err := os.WriteFile(cosign, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >\"$TAILAPPS_COSIGN_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(t.TempDir(), "local")
	tailappHome, err := os.MkdirTemp("/tmp", "tailapps-release-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(tailappHome); err != nil {
			t.Error(err)
		}
	})
	cosignLog := filepath.Join(t.TempDir(), "cosign.log")
	installOutput := run(t, root, append(os.Environ(),
		"TAILAPPS_RELEASE_BASE_URL=file://"+releaseRoot,
		"TAILAPPS_INSTALL_ROOT="+installRoot,
		"TAILAPP_HOME="+tailappHome,
		"TAILAPPS_COSIGN_LOG="+cosignLog,
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
	), "sh", installer, "--bundles", "none", "--no-service")
	if !strings.Contains(string(installOutput), "no bundles were requested; no resident was started") {
		t.Fatalf("bundle-free install started an unnecessary resident:\n%s", installOutput)
	}

	target, err := os.Readlink(filepath.Join(installRoot, "bin", "tailapp"))
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(installRoot, "lib", "tailapp", "tailapp-"+testVersion)
	if target != wantTarget {
		t.Fatalf("installed link = %q, want %q", target, wantTarget)
	}
	if _, err := os.Stat(wantTarget); err != nil {
		t.Fatal(err)
	}
	// Inspect the actual archive-installed binary: a workspace build loses the
	// public core version and checksum even when its source currently matches.
	info, err := buildinfo.ReadFile(wantTarget)
	if err != nil {
		t.Fatal(err)
	}
	foundCore := false
	for _, dep := range info.Deps {
		if dep.Replace != nil {
			t.Fatalf("release archive dependency has replacement: %+v", dep)
		}
		if dep.Path == jsonataddlModule {
			foundCore = true
			if dep.Version != "v0.2.0" || dep.Sum != "h1:rD0TyYRPHT+DapFEacbd+jLQKHo6I+mi2ufpcCd+eKY=" {
				t.Fatalf("release archive must name the public core version and checksum: %+v", dep)
			}
		}
	}
	if !foundCore {
		t.Fatal("release archive has no jsonataddl build information")
	}
	cosignArgs, err := os.ReadFile(cosignLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cosignArgs), "--bundle") || !strings.Contains(string(cosignArgs), "refs/tags/v"+testVersion+"$") {
		t.Fatalf("cosign verification = %q", cosignArgs)
	}
	output := run(t, root, nil, wantTarget, "version")
	var version map[string]any
	if err := json.Unmarshal(output, &version); err != nil {
		t.Fatalf("installed version JSON: %v; %s", err, output)
	}
	if version["version"] != testVersion {
		t.Fatalf("installed version = %#v", version)
	}

	// A present-but-unusable host service manager must not discard the verified
	// binary or selected bundles. The release installer makes the persistence
	// gap explicit and returns nonzero unless --no-service was selected.
	launchctl := filepath.Join(fakeBin, "launchctl")
	serviceManager := "launchctl"
	if runtime.GOOS == "linux" {
		serviceManager = "systemctl"
	}
	unusableServiceManager := filepath.Join(fakeBin, serviceManager)
	if err := os.WriteFile(unusableServiceManager, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fallbackRoot := filepath.Join(t.TempDir(), "local")
	fallbackHome, err := os.MkdirTemp("/tmp", "tailapps-fallback-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(fallbackHome); err != nil {
			t.Error(err)
		}
	})
	fakeUserHome := t.TempDir()
	fallback, err := runFailure(t, root, append(os.Environ(),
		"HOME="+fakeUserHome,
		"TAILAPPS_RELEASE_BASE_URL=file://"+releaseRoot,
		"TAILAPPS_INSTALL_ROOT="+fallbackRoot,
		"TAILAPP_HOME="+fallbackHome,
		"TAILAPPS_COSIGN_LOG="+filepath.Join(t.TempDir(), "fallback-cosign.log"),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
	), "sh", installer, "--bundles", "none")
	if err == nil || !strings.Contains(string(fallback), "persistent setup is incomplete") {
		t.Fatalf("unusable launchd outcome = %v\n%s", err, fallback)
	}
	if _, err := os.Stat(filepath.Join(fallbackRoot, "lib", "tailapp", "tailapp-"+testVersion)); err != nil {
		t.Fatalf("fallback discarded verified binary: %v", err)
	}

	// Exercise the published upgrade surface with a mocked service-manager boundary:
	// it restarts the stable link, returns machine-readable readiness, and can
	// restore the recorded known-good target without touching Tailapps.
	upgradeRoot := filepath.Join(t.TempDir(), "local")
	upgradeHome, err := os.MkdirTemp("/tmp", "tailapps-upgrade-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(upgradeHome); err != nil {
			t.Error(err)
		}
	})
	upgradeUserHome := t.TempDir()
	oldDir := filepath.Join(upgradeRoot, "lib", "tailapp")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldBinary := filepath.Join(oldDir, "tailapp-old")
	run(t, root, nil, "go", "build", "-o", oldBinary, "./cmd/tailapp")
	if err := os.MkdirAll(filepath.Join(upgradeRoot, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeLink := filepath.Join(upgradeRoot, "bin", "tailapp")
	if err := os.Symlink(oldBinary, upgradeLink); err != nil {
		t.Fatal(err)
	}
	unloadedMessage := "LaunchAgent is not loaded"
	if runtime.GOOS == "linux" {
		unitDir := filepath.Join(upgradeUserHome, ".config", "systemd", "user")
		if err := os.MkdirAll(unitDir, 0o700); err != nil {
			t.Fatal(err)
		}
		unit := "[Service]\nEnvironment=TAILAPP_HOME=" + upgradeHome + "\nExecStart=" + upgradeLink + " serve --otlp-http 127.0.0.1:4318\n"
		if err := os.WriteFile(filepath.Join(unitDir, "tailapp.service"), []byte(unit), 0o600); err != nil {
			t.Fatal(err)
		}
		unloadedMessage = "systemd --user is not available"
	} else {
		plistDir := filepath.Join(upgradeUserHome, "Library", "LaunchAgents")
		if err := os.MkdirAll(plistDir, 0o700); err != nil {
			t.Fatal(err)
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>ProgramArguments</key><array><string>` + upgradeLink + `</string></array><key>EnvironmentVariables</key><dict><key>TAILAPP_HOME</key><string>` + upgradeHome + `</string></dict></dict></plist>`
		if err := os.WriteFile(filepath.Join(plistDir, "ai.generalbusiness.tailapp.plist"), []byte(plist), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	failNextStart := filepath.Join(t.TempDir(), "fail-next-start")
	unloaded := filepath.Join(t.TempDir(), "unloaded")
	serviceCommand := launchctl
	serviceScript := `#!/bin/sh
case "$1" in
print)
  [ ! -f "$TAILAPPS_TEST_UNLOADED" ]
  exit
  ;;
kickstart)
  exec curl --fail --silent --show-error "$TAILAPPS_TEST_RESTART"
  ;;
esac
exit 1
`
	if runtime.GOOS == "linux" {
		serviceCommand = filepath.Join(fakeBin, "systemctl")
		serviceScript = `#!/bin/sh
case "$2" in
show-environment)
  [ ! -f "$TAILAPPS_TEST_UNLOADED" ]
  exit
  ;;
daemon-reload)
  exit 0
  ;;
restart)
  exec curl --fail --silent --show-error "$TAILAPPS_TEST_RESTART"
  ;;
esac
exit 1
`
	}
	if err := os.WriteFile(serviceCommand, []byte(serviceScript), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeLog := filepath.Join(t.TempDir(), "upgrade-resident.log")
	service := newTestService(t, upgradeLog, failNextStart, func() *exec.Cmd {
		cmd := exec.Command(upgradeLink, "serve", "--otlp-http", "127.0.0.1:0")
		cmd.Env = append(os.Environ(), "TAILAPP_HOME="+upgradeHome)
		return cmd
	})
	upgradeEnv := append(os.Environ(),
		"HOME="+upgradeUserHome,
		"XDG_CONFIG_HOME="+filepath.Join(upgradeUserHome, ".config"),
		"TAILAPPS_RELEASE_BASE_URL=file://"+releaseRoot,
		"TAILAPPS_INSTALL_ROOT="+upgradeRoot,
		"TAILAPP_HOME="+upgradeHome,
		"TAILAPPS_TEST_RESTART="+service.URL,
		"TAILAPPS_TEST_UNLOADED="+unloaded,
		"TAILAPPS_COSIGN_LOG="+filepath.Join(t.TempDir(), "upgrade-cosign.log"),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
	)
	if err := os.WriteFile(failNextStart, []byte("fail"), 0o600); err != nil {
		t.Fatal(err)
	}
	failed, err := runFailure(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"))
	if err == nil || !strings.Contains(string(failed), "restored the known-good prior binary") {
		t.Fatalf("failed-start upgrade outcome = %v\n%s", err, failed)
	}
	target, err = os.Readlink(upgradeLink)
	if err != nil || target != oldBinary {
		t.Fatalf("failed-start rollback target = %q, %v; want %q", target, err, oldBinary)
	}
	run(t, root, upgradeEnv, oldBinary, "health")
	leftovers, err := filepath.Glob(filepath.Join(oldDir, ".tailapps-upgrade-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("failed upgrade temporary files = %v, %v", leftovers, err)
	}
	if err := os.Remove(filepath.Join(oldDir, "tailapp-"+testVersion)); err != nil {
		t.Fatal(err)
	}

	upgraded := run(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"))
	if !strings.Contains(string(upgraded), `"control_plane":"healthy"`) || !strings.Contains(string(upgraded), `"ingestion_ready":true`) {
		t.Fatalf("upgrade result = %s", upgraded)
	}
	if err := os.WriteFile(unloaded, []byte("unloaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	notLoaded, err := runFailure(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"))
	if err == nil || !strings.Contains(string(notLoaded), unloadedMessage) {
		t.Fatalf("unloaded user service outcome = %v\n%s", err, notLoaded)
	}
	if err := os.Remove(unloaded); err != nil {
		t.Fatal(err)
	}
	upToDate := run(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"))
	if !strings.Contains(string(upToDate), `"action":"up_to_date"`) {
		t.Fatalf("same-version upgrade result = %s", upToDate)
	}
	rolledBack := run(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"), "--rollback")
	if !strings.Contains(string(rolledBack), `"action":"rolled_back"`) {
		t.Fatalf("rollback result = %s", rolledBack)
	}
	target, err = os.Readlink(upgradeLink)
	if err != nil || target != oldBinary {
		t.Fatalf("rollback target = %q, %v; want %q", target, err, oldBinary)
	}
	if err := os.Remove(filepath.Join(oldDir, "tailapp-"+testVersion)); err != nil {
		t.Fatal(err)
	}
	writePendingRelease(t, root, releaseDir, installer, archiveName)
	pending := run(t, root, upgradeEnv, "sh", filepath.Join(dist, "upgrade.sh"))
	if !strings.Contains(string(pending), `"ingestion_ready":false`) ||
		!strings.Contains(string(pending), `"action":"upgrade_pending"`) ||
		!strings.Contains(string(pending), "apps status; follow docs/reference/cli.md#upgrading-an-existing-resident") {
		t.Fatalf("upgrade-pending result = %s", pending)
	}
}

func writePendingRelease(t *testing.T, root, releaseDir, installer, archiveName string) {
	t.Helper()
	stage := t.TempDir()
	stub := `#!/bin/sh
case "$1" in
  version) printf '%s\n' '{"version":"pending-test"}' ;;
  init) exit 0 ;;
  health) printf '%s\n' '{"control_plane":"healthy","ingestion_ready":false}' ;;
  serve) while :; do /bin/sleep 60; done ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(stage, "tailapp"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(releaseDir, archiveName)
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	run(t, root, nil, "tar", "-C", stage, "-czf", archive, "tailapp")
	installerBytes, err := os.ReadFile(installer)
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("%x  install.sh\n%x  %s\n", sha256.Sum256(installerBytes), sha256.Sum256(archiveBytes), archiveName)
	checksums := filepath.Join(releaseDir, "checksums.txt")
	if err := os.Remove(checksums); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksums, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}

func archiveForHost(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("release installer supports darwin and linux only")
	}
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		t.Skip("release installer supports amd64 and arm64 only")
	}
	return fmt.Sprintf("tailapps_%s_%s_%s.tar.gz", testVersion, runtime.GOOS, arch)
}

func run(t *testing.T, directory string, env []string, program string, args ...string) []byte {
	t.Helper()
	command := exec.Command(program, args...)
	command.Dir = directory
	if env != nil {
		command.Env = env
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", program, strings.Join(args, " "), err, output)
	}
	return output
}

func runFailure(t *testing.T, directory string, env []string, program string, args ...string) ([]byte, error) {
	t.Helper()
	command := exec.Command(program, args...)
	command.Dir = directory
	command.Env = env
	return command.CombinedOutput()
}
