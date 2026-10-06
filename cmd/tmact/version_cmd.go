package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// version is the tmact build version. It defaults to "dev" and can be
// overridden at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

// buildTime is the moment the binary was built (RFC 3339, UTC). Go's VCS
// metadata only records the commit time, which goes stale for dirty builds, so
// install scripts set it with -ldflags "-X main.buildTime=...".
var buildTime = ""

type versionInfo struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	// Time is the VCS commit time, not the build time.
	Time      string `json:"time,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	Modified  bool   `json:"modified,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

func buildVersionInfo() versionInfo {
	info := versionInfo{Version: version, BuildTime: buildTime}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	info.GoVersion = bi.GoVersion
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

// BuiltAt returns the stamped build time, falling back to the commit time for
// binaries built without the install-script ldflags.
func (v versionInfo) BuiltAt() string {
	if v.BuildTime != "" {
		return v.BuildTime
	}
	return v.Time
}

func (v versionInfo) String() string {
	var b strings.Builder
	b.WriteString("tmact ")
	b.WriteString(v.Version)
	if v.Revision != "" {
		rev := v.Revision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		b.WriteString(" (")
		b.WriteString(rev)
		if v.Modified {
			b.WriteString("-dirty")
		}
		b.WriteString(")")
	}
	if v.BuildTime != "" {
		b.WriteString(" built ")
		b.WriteString(v.BuildTime)
		if v.Time != "" {
			b.WriteString(" (commit ")
			b.WriteString(v.Time)
			b.WriteString(")")
		}
	} else if v.Time != "" {
		b.WriteString(" built ")
		b.WriteString(v.Time)
	}
	if v.GoVersion != "" {
		b.WriteString(" with ")
		b.WriteString(v.GoVersion)
	}
	return b.String()
}

func runVersion(args []string) error {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "-h", "--help", "help":
			fmt.Print(`Usage:
  tmact version [--json]
  tmact -v | --version

Print the tmact build version. When the binary was built from a Git
checkout, the VCS revision, commit time, and dirty flag are included.
Binaries installed by scripts/install.sh also carry the actual build time.
`)
			return nil
		default:
			return fmt.Errorf("unknown version flag %q", arg)
		}
	}
	info := buildVersionInfo()
	if jsonOutput {
		return printJSON(info)
	}
	fmt.Println(info.String())
	return nil
}
