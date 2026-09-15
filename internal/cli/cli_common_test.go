package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
)

func TestEnsureDaemonCanceledContextDoesNotStart(t *testing.T) {
	root := t.TempDir()
	paths := config.Paths{
		Socket: filepath.Join(root, "nxd.sock"),
		PID:    filepath.Join(root, "nxd.pid"),
	}

	original := startDaemonProcess
	defer func() {
		startDaemonProcess = original
	}()
	started := false
	startDaemonProcess = func(config.Paths) error {
		started = true
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, daemonStarted, err := ensureDaemon(ctx, paths)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil || daemonStarted || started {
		t.Fatalf("canceled ensure started daemon: client=%v daemon_started=%t start_called=%t", client, daemonStarted, started)
	}
}

func TestDaemonProcessLockReleasedTracksLifetimeLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nxd.pid")
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	released, err := daemonProcessLockReleased(path)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("daemon process lock was reported as released while held")
	}

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	released, err = daemonProcessLockReleased(path)
	if err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("daemon process lock was not reported as released after unlock")
	}
}

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
