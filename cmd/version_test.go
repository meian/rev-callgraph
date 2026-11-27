package cmd_test

import (
	"runtime/debug"
	"testing"

	"github.com/meian/rev-callgraph/cmd"
)

func TestFormatVersion(t *testing.T) {
	const revision = "abcdef0123456789abcdef0123456789abcdef01"
	for _, tt := range []struct {
		name          string
		release       string
		moduleVersion string
		revision      string
		modified      string
		want          string
	}{
		{
			name:          "release takes precedence",
			release:       "v1.2.3",
			moduleVersion: "v1.0.0",
			revision:      revision,
			modified:      "false",
			want:          "v1.2.3 (abcdef012345)",
		},
		{
			name:     "dirty release",
			release:  "v1.2.3",
			revision: revision,
			modified: "true",
			want:     "v1.2.3 (abcdef012345-dirty)",
		},
		{
			name:          "revision takes precedence over module version",
			moduleVersion: "v1.0.1-0.20260102030405.abcdef012345",
			revision:      revision,
			want:          "abcdef012345",
		},
		{
			name:     "dirty revision",
			revision: revision,
			modified: "true",
			want:     "abcdef012345-dirty",
		},
		{
			name:     "short revision",
			revision: "abcdef0",
			want:     "abcdef0",
		},
		{
			name:          "module version without revision",
			moduleVersion: "v1.0.0",
			modified:      "true",
			want:          "v1.0.0",
		},
		{
			name:          "release without revision",
			release:       "v1.2.3",
			moduleVersion: "v1.0.0",
			want:          "v1.2.3 (revision: unknown)",
		},
		{
			name:          "development build without revision",
			moduleVersion: "(devel)",
			want:          "unknown",
		},
		{
			name:          "invalid module version",
			moduleVersion: "invalid",
			want:          "unknown",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &debug.BuildInfo{
				Main: debug.Module{Version: tt.moduleVersion},
				Settings: []debug.BuildSetting{
					{
						Key:   "vcs.revision",
						Value: tt.revision,
					},
					{
						Key:   "vcs.modified",
						Value: tt.modified,
					},
				},
			}
			if got := cmd.FormatVersion(tt.release, info); got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
			if got := info.Settings[0].Value; got != tt.revision {
				t.Errorf("BuildInfo revision = %q, want %q", got, tt.revision)
			}
		})
	}
}

func TestFormatVersionWithoutBuildInfo(t *testing.T) {
	if got := cmd.FormatVersion("", nil); got != "unknown" {
		t.Errorf("version = %q, want unknown", got)
	}
}
