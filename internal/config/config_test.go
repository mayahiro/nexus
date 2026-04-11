package config

import "testing"

func TestDefaultPaths(t *testing.T) {
	t.Setenv("HOME", "/tmp/nexus-home")

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.Config != "/tmp/nexus-home/.config/nexus/config.yaml" {
		t.Fatalf("unexpected config path: %s", paths.Config)
	}

	if paths.Socket != "/tmp/nexus-home/.local/state/nexus/nxd.sock" {
		t.Fatalf("unexpected socket path: %s", paths.Socket)
	}

	if paths.Data != "/tmp/nexus-home/.local/share/nexus" {
		t.Fatalf("unexpected data path: %s", paths.Data)
	}

	if paths.Cache != "/tmp/nexus-home/.cache/nexus" {
		t.Fatalf("unexpected cache path: %s", paths.Cache)
	}
}

func TestDefaultPathsWithXDG(t *testing.T) {
	t.Setenv("HOME", "/tmp/nexus-home")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/xdg-runtime")

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.Config != "/tmp/xdg-config/nexus/config.yaml" {
		t.Fatalf("unexpected config path: %s", paths.Config)
	}

	if paths.Socket != "/tmp/xdg-runtime/nexus/nxd.sock" {
		t.Fatalf("unexpected socket path: %s", paths.Socket)
	}

	if paths.PID != "/tmp/xdg-state/nexus/nxd.pid" {
		t.Fatalf("unexpected pid path: %s", paths.PID)
	}

	if paths.Log != "/tmp/xdg-state/nexus/nxd.log" {
		t.Fatalf("unexpected log path: %s", paths.Log)
	}

	if paths.Data != "/tmp/nexus-home/.local/share/nexus" {
		t.Fatalf("unexpected data path: %s", paths.Data)
	}

	if paths.Cache != "/tmp/nexus-home/.cache/nexus" {
		t.Fatalf("unexpected cache path: %s", paths.Cache)
	}
}

func TestDefaultPathsWithDataAndCacheXDG(t *testing.T) {
	t.Setenv("HOME", "/tmp/nexus-home")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-cache")

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.Data != "/tmp/xdg-data/nexus" {
		t.Fatalf("unexpected data path: %s", paths.Data)
	}

	if paths.Cache != "/tmp/xdg-cache/nexus" {
		t.Fatalf("unexpected cache path: %s", paths.Cache)
	}
}

func TestDefaultPathsIgnoresRelativeXDG(t *testing.T) {
	t.Setenv("HOME", "/tmp/nexus-home")
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("XDG_STATE_HOME", "relative-state")
	t.Setenv("XDG_RUNTIME_DIR", "relative-runtime")

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.Config != "/tmp/nexus-home/.config/nexus/config.yaml" {
		t.Fatalf("unexpected config path: %s", paths.Config)
	}

	if paths.Socket != "/tmp/nexus-home/.local/state/nexus/nxd.sock" {
		t.Fatalf("unexpected socket path: %s", paths.Socket)
	}

	if paths.Data != "/tmp/nexus-home/.local/share/nexus" {
		t.Fatalf("unexpected data path: %s", paths.Data)
	}

	if paths.Cache != "/tmp/nexus-home/.cache/nexus" {
		t.Fatalf("unexpected cache path: %s", paths.Cache)
	}
}
