package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Config string
	Socket string
	PID    string
	Log    string
	Data   string
	Cache  string
}

func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	configHome := xdgDir("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	stateHome := xdgDir("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	runtimeHome := xdgDir("XDG_RUNTIME_DIR", stateHome)
	dataHome := xdgDir("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	cacheHome := xdgDir("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configDir := filepath.Join(configHome, "nexus")
	stateDir := filepath.Join(stateHome, "nexus")
	runtimeDir := filepath.Join(runtimeHome, "nexus")
	dataDir := filepath.Join(dataHome, "nexus")
	cacheDir := filepath.Join(cacheHome, "nexus")

	return Paths{
		Config: filepath.Join(configDir, "config.yaml"),
		Socket: filepath.Join(runtimeDir, "nxd.sock"),
		PID:    filepath.Join(stateDir, "nxd.pid"),
		Log:    filepath.Join(stateDir, "nxd.log"),
		Data:   dataDir,
		Cache:  cacheDir,
	}, nil
}

func xdgDir(key string, fallback string) string {
	value := os.Getenv(key)
	if filepath.IsAbs(value) {
		return value
	}
	return fallback
}
