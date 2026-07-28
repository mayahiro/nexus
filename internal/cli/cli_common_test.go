package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/daemon"
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

func TestDaemonProcessEnvironmentReplacesLogBase(t *testing.T) {
	t.Setenv(daemon.ProcessLogBaseEnv, "/tmp/old.log")

	environment := daemonProcessEnvironment("/tmp/new.log")
	want := daemon.ProcessLogBaseEnv + "=/tmp/new.log"
	count := 0
	for _, value := range environment {
		if strings.HasPrefix(value, daemon.ProcessLogBaseEnv+"=") {
			count++
			if value != want {
				t.Fatalf("unexpected daemon log environment: %s", value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("unexpected daemon log environment count: %d in %v", count, os.Environ())
	}
}
