package cmd

import (
	"runtime/debug"

	"golang.org/x/mod/semver"
)

// releaseVersion はリリースビルド時に -ldflags -X で設定する。
var releaseVersion string

func version() string {
	info, _ := debug.ReadBuildInfo()
	return formatVersion(releaseVersion, info)
}

func formatVersion(release string, info *debug.BuildInfo) string {
	var revision, moduleVersion string
	var dirty bool
	if info != nil {
		moduleVersion = info.Main.Version
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
	}
	if revision != "" {
		revision = revision[:min(len(revision), 12)]
		if dirty {
			revision += "-dirty"
		}
	}
	if release != "" {
		if revision == "" {
			revision = "revision: unknown"
		}
		return release + " (" + revision + ")"
	}
	if revision != "" {
		return revision
	}
	if semver.IsValid(moduleVersion) {
		return moduleVersion
	}
	return "unknown"
}
