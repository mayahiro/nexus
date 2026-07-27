package api

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDaemonVersionIncludesCompatibilityEpoch(t *testing.T) {
	if !strings.HasPrefix(DaemonVersion, daemonBuildEpoch) {
		t.Fatalf("unexpected daemon version: %s", DaemonVersion)
	}
}

func TestObservationScreenshotDataJSONCompatibility(t *testing.T) {
	observation := Observation{
		SessionID:      "web1",
		ScreenshotData: []byte{0, 1, 2, 3, 255},
	}
	data, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Observation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	screenshot, err := decoded.ScreenshotBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(screenshot, observation.ScreenshotData) {
		t.Fatalf("unexpected screenshot bytes: %v", screenshot)
	}
}
