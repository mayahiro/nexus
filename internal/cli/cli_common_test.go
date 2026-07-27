package cli

import (
	"testing"

	"github.com/mayahiro/nexus/internal/api"
)

func TestDaemonCompatibleRequiresProtocolAndBuild(t *testing.T) {
	if !daemonCompatible(api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   api.DaemonVersion,
	}) {
		t.Fatal("expected matching daemon to be compatible")
	}
	if daemonCompatible(api.PingResponse{
		ProtocolVersion: "v0",
		DaemonVersion:   api.DaemonVersion,
	}) {
		t.Fatal("expected protocol mismatch to be incompatible")
	}
	if daemonCompatible(api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   "old-build",
	}) {
		t.Fatal("expected build mismatch to be incompatible")
	}
}
