package session

import (
	"context"
	"testing"
)

func TestShutdownWithNoSessions(t *testing.T) {
	manager := NewManager()

	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
