package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
	"github.com/mayahiro/nexus/internal/session"
)

type Server struct {
	sessions sessionManager
	stop     context.CancelFunc
	verbose  bool
}

type sessionManager interface {
	Attach(ctx context.Context, req api.AttachSessionRequest) (api.Session, error)
	List() []api.Session
	Detach(ctx context.Context, sessionID string) (api.Session, error)
	Observe(ctx context.Context, sessionID string, opts api.ObserveOptions) (api.Observation, error)
	Act(ctx context.Context, sessionID string, action api.Action) (api.ActionResult, error)
	Shutdown(ctx context.Context) error
}

const shutdownTimeout = 5 * time.Second
const ProcessLogBaseEnv = "NEXUS_DAEMON_LOG_BASE"

var screenshotObserveTimeout = 30 * time.Second

type RunOptions struct {
	IdleTimeout time.Duration
	LogPath     string
	Verbose     bool
}

func Run(ctx context.Context, paths config.Paths, opts RunOptions) (runErr error) {
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		return err
	}

	processLock, err := acquireProcessLock(paths.PID)
	if err != nil {
		return err
	}
	defer releaseProcessLock(processLock)

	log.Printf(
		"nexus daemon event=start pid=%d socket=%q log=%q verbose=%t",
		os.Getpid(),
		paths.Socket,
		opts.LogPath,
		opts.Verbose,
	)
	defer func() {
		log.Printf(
			"nexus daemon event=stop pid=%d error=%q",
			os.Getpid(),
			errorMessage(runErr),
		)
	}()

	if err := prepareSocket(paths.Socket); err != nil {
		return err
	}

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		return err
	}
	defer os.Remove(paths.Socket)
	defer listener.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	server := NewServer(cancel)
	server.verbose = opts.Verbose
	serveOpts := rpc.ServeOptions{}
	if opts.IdleTimeout > 0 {
		activity := make(chan struct{}, 1)
		serveOpts.OnActivity = func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		}
		go watchIdle(runCtx, opts.IdleTimeout, activity, cancel)
	}

	err = rpc.Serve(runCtx, listener, server, serveOpts)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil && err == nil {
		err = shutdownErr
	}

	return err
}

func ProcessLogPath(base string, pid int) string {
	extension := filepath.Ext(base)
	name := strings.TrimSuffix(filepath.Base(base), extension)
	return filepath.Join(
		filepath.Dir(base),
		fmt.Sprintf("%s.%d%s", name, pid, extension),
	)
}

func acquireProcessLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		existingPID, _ := os.ReadFile(path)
		lock.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("lock daemon pid file %s: %w", path, err)
		}
		pid := strings.TrimSpace(string(existingPID))
		if pid != "" {
			return nil, fmt.Errorf("daemon already running with pid %s: %s", pid, path)
		}
		return nil, fmt.Errorf("daemon already running: %s: %w", path, err)
	}

	if err := lock.Truncate(0); err != nil {
		releaseProcessLock(lock)
		return nil, err
	}
	if _, err := lock.Seek(0, 0); err != nil {
		releaseProcessLock(lock)
		return nil, err
	}
	if _, err := fmt.Fprintln(lock, strconv.Itoa(os.Getpid())); err != nil {
		releaseProcessLock(lock)
		return nil, err
	}
	if err := lock.Sync(); err != nil {
		releaseProcessLock(lock)
		return nil, err
	}

	return lock, nil
}

func releaseProcessLock(lock *os.File) {
	if lock == nil {
		return
	}
	lock.Truncate(0)
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lock.Close()
}

func NewServer(stop context.CancelFunc) Server {
	return Server{
		sessions: session.NewManager(),
		stop:     stop,
	}
}

func (s Server) Ping(_ context.Context, _ api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   api.DaemonVersion,
	}, nil
}

func (s Server) AttachSession(ctx context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	startedAt := time.Now()
	s.logVerbose("request=attach event=start session=%q", req.SessionID)
	session, err := s.sessions.Attach(ctx, req)
	s.logVerbose(
		"request=attach event=finish session=%q duration_ms=%d error=%q",
		req.SessionID,
		time.Since(startedAt).Milliseconds(),
		errorMessage(err),
	)
	if err != nil {
		return api.AttachSessionResponse{}, err
	}

	return api.AttachSessionResponse{Session: session}, nil
}

func (s Server) ListSessions(_ context.Context, _ api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	s.logVerbose("request=list_sessions")
	return api.ListSessionsResponse{
		Sessions: s.sessions.List(),
	}, nil
}

func (s Server) DetachSession(ctx context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	startedAt := time.Now()
	s.logVerbose("request=detach event=start session=%q", req.SessionID)
	session, err := s.sessions.Detach(ctx, req.SessionID)
	s.logVerbose(
		"request=detach event=finish session=%q duration_ms=%d error=%q",
		req.SessionID,
		time.Since(startedAt).Milliseconds(),
		errorMessage(err),
	)
	if err != nil {
		return api.DetachSessionResponse{}, err
	}

	return api.DetachSessionResponse{Session: session}, nil
}

func (s Server) StopDaemon(_ context.Context, _ api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	s.logVerbose("request=stop_daemon")
	if s.stop != nil {
		s.stop()
	}
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (s Server) ObserveSession(ctx context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	req.Options.Verbose = req.Options.Verbose || s.verbose
	startedAt := time.Now()
	if req.Options.Verbose {
		log.Printf(
			"nexus daemon request=observe event=start session=%q screenshot=%t full=%t recover=%t timeout_ms=%d",
			req.SessionID,
			req.Options.WithScreenshot,
			req.Options.FullScreenshot,
			req.Options.RecoverScreenshot,
			req.Options.TimeoutMS,
		)
	}
	observeCtx, cancel := observeSessionContext(ctx, req.Options)
	defer cancel()

	observation, err := s.sessions.Observe(observeCtx, req.SessionID, req.Options)
	if req.Options.Verbose {
		log.Printf(
			"nexus daemon request=observe event=finish session=%q screenshot=%t full=%t recover=%t timeout_ms=%d duration_ms=%d error=%q",
			req.SessionID,
			req.Options.WithScreenshot,
			req.Options.FullScreenshot,
			req.Options.RecoverScreenshot,
			req.Options.TimeoutMS,
			time.Since(startedAt).Milliseconds(),
			errorMessage(err),
		)
	}
	if err != nil {
		return api.ObserveSessionResponse{}, err
	}

	return api.ObserveSessionResponse{Observation: observation}, nil
}

func observeSessionContext(ctx context.Context, opts api.ObserveOptions) (context.Context, context.CancelFunc) {
	if opts.WithScreenshot {
		timeout := screenshotObserveTimeout
		if opts.TimeoutMS > 0 {
			timeout = time.Duration(opts.TimeoutMS) * time.Millisecond
		}
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func (s Server) ActSession(ctx context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	startedAt := time.Now()
	s.logVerbose("request=act event=start session=%q action=%q", req.SessionID, req.Action.Kind)
	result, err := s.sessions.Act(ctx, req.SessionID, req.Action)
	s.logVerbose(
		"request=act event=finish session=%q action=%q duration_ms=%d error=%q",
		req.SessionID,
		req.Action.Kind,
		time.Since(startedAt).Milliseconds(),
		errorMessage(err),
	)
	if err != nil {
		return api.ActSessionResponse{}, err
	}

	return api.ActSessionResponse{Result: result}, nil
}

func (s Server) Shutdown(ctx context.Context) error {
	startedAt := time.Now()
	s.logVerbose("request=shutdown event=start")
	err := s.sessions.Shutdown(ctx)
	s.logVerbose(
		"request=shutdown event=finish duration_ms=%d error=%q",
		time.Since(startedAt).Milliseconds(),
		errorMessage(err),
	)
	return err
}

func (s Server) logVerbose(format string, args ...any) {
	if !s.verbose {
		return
	}
	log.Printf("nexus daemon "+format, args...)
}

func prepareSocket(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("socket already in use: %s", path)
	}

	return os.Remove(path)
}

func watchIdle(ctx context.Context, timeout time.Duration, activity <-chan struct{}, cancel context.CancelFunc) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
			cancel()
			return
		}
	}
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled.Error()
	}
	return err.Error()
}
