package api

import (
	"runtime/debug"
	"strings"
)

const (
	ProtocolVersion  = "v1"
	daemonBuildEpoch = "2026.08.02.1"
)

var DaemonVersion = currentDaemonVersion()

func currentDaemonVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return daemonBuildEpoch
	}

	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" && strings.TrimSpace(info.Main.Version) != "" && info.Main.Version != "(devel)" {
		revision = strings.TrimSpace(info.Main.Version)
	}
	if revision == "" {
		return daemonBuildEpoch
	}
	if !strings.HasPrefix(revision, "v") && len(revision) > 12 {
		revision = revision[:12]
	}
	version := daemonBuildEpoch + "+" + revision
	if modified {
		version += ".dirty"
	}
	return version
}

type PingRequest struct {
	ProtocolVersion string `json:"protocol_version"`
}

type PingResponse struct {
	ProtocolVersion string `json:"protocol_version"`
	DaemonVersion   string `json:"daemon_version"`
}
