package diagnostic

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTraceDiscardsSuccessfulBufferedEvents(t *testing.T) {
	var entries []string
	trace := New("observe_session", false, func(entry string) {
		entries = append(entries, entry)
	}, Value("os", "darwin"))

	trace.Event("daemon_request", "start")
	trace.Event("daemon_request", "finish")
	trace.Finish(nil)

	if len(entries) != 0 {
		t.Fatalf("successful non-verbose trace emitted entries: %v", entries)
	}
}

func TestTraceFlushesBufferedEventsAndEnvironmentOnFailure(t *testing.T) {
	var entries []string
	trace := New("observe_session", false, func(entry string) {
		entries = append(entries, entry)
	},
		Value("daemon_version", "test-version"),
		Value("os", "darwin"),
	)

	trace.Event("daemon_request", "start", Value("session", "web1"))
	trace.Event("browser_capture", "finish", Value("error", errors.New("capture failed")))
	trace.Finish(errors.New("capture failed"))

	joined := strings.Join(entries, "\n")
	for _, expected := range []string{
		`event="failure"`,
		`daemon_version="test-version"`,
		`os="darwin"`,
		`stage="daemon_request"`,
		`session="web1"`,
		`stage="browser_capture"`,
		`error="capture failed"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("failure trace does not contain %q:\n%s", expected, joined)
		}
	}
}

func TestVerboseTraceEmitsEveryEvent(t *testing.T) {
	var entries []string
	trace := New("act_session", true, func(entry string) {
		entries = append(entries, entry)
	}, Value("arch", "arm64"))

	trace.Event("daemon_request", "start", Value("action", "click"))
	trace.Event("daemon_request", "finish")
	trace.Finish(nil)

	if len(entries) != 4 {
		t.Fatalf("unexpected verbose entry count %d: %v", len(entries), entries)
	}
	joined := strings.Join(entries, "\n")
	for _, expected := range []string{
		`stage="environment" event="snapshot"`,
		`stage="daemon_request" event="start"`,
		`action="click"`,
		`event="complete" outcome="success"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("verbose trace does not contain %q:\n%s", expected, joined)
		}
	}
}

func TestSetVerboseFlushesPreviouslyBufferedEvents(t *testing.T) {
	var entries []string
	trace := New("observe_session", false, func(entry string) {
		entries = append(entries, entry)
	}, Value("os", "darwin"))
	trace.Event("rpc_request", "decoded")

	trace.SetVerbose(true)
	trace.Event("daemon_request", "start")
	trace.Finish(nil)

	joined := strings.Join(entries, "\n")
	if !strings.Contains(joined, `stage="rpc_request" event="decoded"`) {
		t.Fatalf("buffered event was not emitted after enabling verbose:\n%s", joined)
	}
	if !strings.Contains(joined, `stage="daemon_request" event="start"`) {
		t.Fatalf("new verbose event was not emitted:\n%s", joined)
	}
}

func TestTraceContextRoundTrip(t *testing.T) {
	trace := New("ping", false, func(string) {})
	ctx := WithTrace(context.Background(), trace)
	if FromContext(ctx) != trace {
		t.Fatal("trace was not preserved in context")
	}
	trace.Finish(nil)
}

func TestTraceFailureBufferIsBounded(t *testing.T) {
	var entries []string
	trace := New("act_session", false, func(entry string) {
		entries = append(entries, entry)
	})
	for index := 0; index < maxBufferedEvents+10; index++ {
		trace.Event("action", "step", Value("index", index))
	}
	trace.Finish(errors.New("failed"))

	if len(entries) != maxBufferedEvents+1 {
		t.Fatalf("unexpected bounded trace entry count %d", len(entries))
	}
	if !strings.Contains(entries[0], "dropped_events=10") {
		t.Fatalf("failure summary does not report dropped events: %s", entries[0])
	}
}

func TestRuntimeEnvironmentContainsIdentificationFields(t *testing.T) {
	fields := RuntimeEnvironment()
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		value, ok := field.Value.(string)
		if ok {
			values[field.Key] = value
		}
	}
	for _, key := range []string{"go_version", "os", "os_version", "arch"} {
		if strings.TrimSpace(values[key]) == "" {
			t.Fatalf("runtime environment does not contain %s: %v", key, values)
		}
	}
}
