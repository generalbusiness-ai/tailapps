// Package buildinfo derives the binary's version and source identity from Go
// build information plus an optional build-time stamp. It never reads git
// state at run time; when metadata is absent every value falls back
// deterministically.
package buildinfo

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// baseVersion is the release base when no module tag names one.
const baseVersion = "0.1.0"

// stampedSourceURL is set by release builds via
//
//	-ldflags "-X github.com/generalbusiness-ai/tailapps/internal/buildinfo.stampedSourceURL=https://github.com/OWNER/REPO"
//
// where the build script sanitizes `git remote get-url origin`: only a
// github.com remote is accepted, normalized to the bare https form with
// user-info, credentials, ports, and any .git suffix stripped. A plain
// `go build` leaves it empty and the module path supplies the fallback.
var stampedSourceURL string

var githubURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Version reports the binary's version: a real module tag when one names the
// build, otherwise the base version annotated with the VCS revision and a
// .dirty marker, otherwise the bare base version.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	return version(info, ok)
}

func version(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil {
		return baseVersion
	}
	if main := info.Main.Version; main != "" && main != "(devel)" && !strings.HasPrefix(main, "v0.0.0-") {
		return strings.TrimPrefix(main, "v")
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return baseVersion
	}
	short := revision
	if len(short) > 12 {
		short = short[:12]
	}
	derived := baseVersion + "+" + short
	if modified {
		derived += ".dirty"
	}
	return derived
}

// SourceURL reports the public source location: the sanitized build-time
// stamp when a release build provided a valid one, otherwise a URL derived
// from the module path when it names a known forge host, otherwise empty.
// Omission is preferred over fabrication.
func SourceURL() string {
	info, ok := debug.ReadBuildInfo()
	return sourceURL(stampedSourceURL, info, ok)
}

func sourceURL(stamped string, info *debug.BuildInfo, ok bool) string {
	if githubURL.MatchString(stamped) {
		return stamped
	}
	if !ok || info == nil {
		return ""
	}
	path := info.Main.Path
	if rest, found := strings.CutPrefix(path, "github.com/"); found {
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return "https://github.com/" + parts[0] + "/" + parts[1]
		}
	}
	return ""
}
