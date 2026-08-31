package buildinfo

import (
	"runtime/debug"
	"testing"
)

func synthetic(mainVersion, mainPath string, settings map[string]string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	info.Main.Version = mainVersion
	info.Main.Path = mainPath
	for key, value := range settings {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: key, Value: value})
	}
	return info
}

func TestVersionDerivation(t *testing.T) {
	revision := "70952f24f5901084f4eaefe9d2aa734e3cd3bf89"
	cases := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"no build info", nil, false, "0.1.0"},
		{"real tag wins", synthetic("v1.2.3", "m", nil), true, "1.2.3"},
		{"pseudo-version falls through to revision", synthetic("v0.0.0-20260830102107-70952f24f590", "m", map[string]string{"vcs.revision": revision, "vcs.modified": "false"}), true, "0.1.0+70952f24f590"},
		{"devel with clean revision", synthetic("(devel)", "m", map[string]string{"vcs.revision": revision, "vcs.modified": "false"}), true, "0.1.0+70952f24f590"},
		{"dirty build marked", synthetic("(devel)", "m", map[string]string{"vcs.revision": revision, "vcs.modified": "true"}), true, "0.1.0+70952f24f590.dirty"},
		{"no vcs stamps", synthetic("(devel)", "m", nil), true, "0.1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := version(tc.info, tc.ok); got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStampedReleaseVersionWinsOnlyWhenValid(t *testing.T) {
	info := synthetic("(devel)", "m", map[string]string{"vcs.revision": "70952f24f5901084f4eaefe9d2aa734e3cd3bf89"})
	if got := versionWithStamp("1.2.3", info, true); got != "1.2.3" {
		t.Fatalf("valid stamped version = %q", got)
	}
	if got := versionWithStamp("v1.2.3", info, true); got != "0.1.0+70952f24f590" {
		t.Fatalf("invalid stamped version must fall back, got %q", got)
	}
}

func TestSourceURLDerivation(t *testing.T) {
	module := "github.com/generalbusiness-ai/tailapps"
	cases := []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"valid stamp wins", "https://github.com/generalbusiness-ai/tailapps", nil, false, "https://github.com/generalbusiness-ai/tailapps"},
		{"invalid stamp falls back to module path", "https://github.com/only-owner", synthetic("(devel)", module, nil), true, "https://github.com/generalbusiness-ai/tailapps"},
		{"credentialed stamp rejected", "https://user:token@github.com/o/r", synthetic("(devel)", module, nil), true, "https://github.com/generalbusiness-ai/tailapps"},
		{"module path derives owner/repo only", "", synthetic("(devel)", module+"/cmd/tailapp", nil), true, "https://github.com/generalbusiness-ai/tailapps"},
		{"unknown forge omits", "", synthetic("(devel)", "example.org/x/y", nil), true, ""},
		{"nothing known omits", "", nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceURL(tc.stamped, tc.info, tc.ok); got != tc.want {
				t.Fatalf("sourceURL = %q, want %q", got, tc.want)
			}
		})
	}
}
