package chromium

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitTimeoutBoundsIndividualCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	err := pollBrowserWait(ctx, 25*time.Millisecond, func(checkCtx context.Context) (bool, error) {
		<-checkCtx.Done()
		return false, checkCtx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "wait timed out after 25ms") {
		t.Fatalf("unexpected wait error: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("check exceeded the wait deadline")
	}
}

func TestWaitUsesEarlierCallerDeadlineAndReturnsEvaluationError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pollBrowserWait(ctx, time.Second, func(context.Context) (bool, error) { t.Fatal("check called after cancellation"); return false, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	evalErr := errors.New("invalid expression")
	if err := pollBrowserWait(context.Background(), time.Second, func(context.Context) (bool, error) { return false, evalErr }); !errors.Is(err, evalErr) {
		t.Fatalf("evaluation error lost: %v", err)
	}
	if err := pollBrowserWait(context.Background(), time.Second, func(context.Context) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestDetachRetriesFailedProfileRemoval(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.Mkdir(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "data"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(profile, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(profile, 0o700) })
	b := New()
	b.userDataDir = profile
	if err := b.Detach(context.Background()); err == nil {
		t.Fatal("expected profile removal failure")
	}
	if b.userDataDir != profile {
		t.Fatal("failed profile cleanup forgotten")
	}
	if err := os.Chmod(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.Detach(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("profile remains: %v", err)
	}
}
