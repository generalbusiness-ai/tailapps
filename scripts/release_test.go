package scripts_test

import (
	"bytes"
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
	for _, name := range []string{
		"tailapps_" + testVersion + "_darwin_arm64.tar.gz",
		"tailapps_" + testVersion + "_darwin_amd64.tar.gz",
		"tailapps_" + testVersion + "_linux_arm64.tar.gz",
		"tailapps_" + testVersion + "_linux_amd64.tar.gz",
		"checksums.txt", "SHA256SUMS",
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
	if !bytes.Equal(checksums, sha256sums) || !strings.Contains(string(checksums), "install.sh") {
		t.Fatalf("checksum manifests do not cover the same release asset set")
	}

	releaseRoot := t.TempDir()
	releaseDir := filepath.Join(releaseRoot, "download", "v"+testVersion)
	if err := os.MkdirAll(releaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	archiveName := archiveForHost(t)
	for _, name := range []string{"checksums.txt", archiveName} {
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
	cosignLog := filepath.Join(t.TempDir(), "cosign.log")
	run(t, root, append(os.Environ(),
		"TAILAPPS_RELEASE_BASE_URL=file://"+releaseRoot,
		"TAILAPPS_INSTALL_ROOT="+installRoot,
		"TAILAPPS_COSIGN_LOG="+cosignLog,
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
	), "sh", installer)

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
