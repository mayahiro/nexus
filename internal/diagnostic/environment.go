package diagnostic

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	osVersionOnce sync.Once
	osVersion     string
)

// RuntimeEnvironment returns stable Go and operating system diagnostic fields
func RuntimeEnvironment() []Field {
	return []Field{
		Value("go_version", runtime.Version()),
		Value("os", runtime.GOOS),
		Value("os_version", currentOSVersion()),
		Value("arch", runtime.GOARCH),
	}
}

func currentOSVersion() string {
	osVersionOnce.Do(func() {
		osVersion = "unknown"
		if runtime.GOOS != "darwin" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion").Output()
		if err != nil {
			return
		}
		if version := strings.TrimSpace(string(output)); version != "" {
			osVersion = version
		}
	})
	return osVersion
}
