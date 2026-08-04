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
	"github.com/mayahiro/nexus/internal/diagnostic"
	"github.com/mayahiro/nexus/internal/rpc"
	"github.com/mayahiro/nexus/internal/session"
)

type Server struct {
	sessions sessionManager
	stop     context.CancelFunc
	verbose  bool
	logger   func(string)
}

type sessionManager interface {
	Attach(ctx context.Context, req api.AttachSessionRequest) (api.Session, error)
	List() []api.Session
	Detach(ctx context.Context, sessionID string) (api.Session, error)
	Observe(ctx context.Context, sessionID string, opts api.ObserveOptions) (api.Observation, error)
	InspectStyles(ctx context.Context, req api.InspectStylesRequest) (api.StyleInspection, error)
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
		"nexus daemon event=start pid=%d socket=%q log=%q verbose=%t daemon_version=%q protocol_version=%q go_version=%q os=%q os_version=%q arch=%q",
		os.Getpid(),
		paths.Socket,
		opts.LogPath,
		opts.Verbose,
		api.DaemonVersion,
		api.ProtocolVersion,
		runtimeEnvironmentValue("go_version"),
		runtimeEnvironmentValue("os"),
		runtimeEnvironmentValue("os_version"),
		runtimeEnvironmentValue("arch"),
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
	serveOpts := rpc.ServeOptions{
		NewTrace: func(method string) *diagnostic.Trace {
			return server.newTrace(method, server.verbose)
		},
	}
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

func (s Server) Ping(ctx context.Context, _ api.PingRequest) (response api.PingResponse, resultErr error) {
	_, finish := s.beginRequest(ctx, "ping", s.verbose)
	defer func() { finish(resultErr) }()

	return api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   api.DaemonVersion,
	}, nil
}

func (s Server) AttachSession(ctx context.Context, req api.AttachSessionRequest) (response api.AttachSessionResponse, resultErr error) {
	ctx, finish := s.beginRequest(ctx, "attach_session", s.verbose,
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("backend", req.Backend),
		diagnostic.Value("target_type", req.TargetType),
	)
	defer func() { finish(resultErr) }()

	session, err := s.sessions.Attach(ctx, req)
	if err != nil {
		return api.AttachSessionResponse{}, err
	}

	return api.AttachSessionResponse{Session: session}, nil
}

func (s Server) ListSessions(ctx context.Context, _ api.ListSessionsRequest) (response api.ListSessionsResponse, resultErr error) {
	_, finish := s.beginRequest(ctx, "list_sessions", s.verbose)
	defer func() { finish(resultErr) }()

	return api.ListSessionsResponse{
		Sessions: s.sessions.List(),
	}, nil
}

func (s Server) DetachSession(ctx context.Context, req api.DetachSessionRequest) (response api.DetachSessionResponse, resultErr error) {
	ctx, finish := s.beginRequest(ctx, "detach_session", s.verbose,
		diagnostic.Value("session", req.SessionID),
	)
	defer func() { finish(resultErr) }()

	session, err := s.sessions.Detach(ctx, req.SessionID)
	if err != nil {
		return api.DetachSessionResponse{}, err
	}

	return api.DetachSessionResponse{Session: session}, nil
}

func (s Server) StopDaemon(ctx context.Context, _ api.StopDaemonRequest) (response api.StopDaemonResponse, resultErr error) {
	_, finish := s.beginRequest(ctx, "stop_daemon", s.verbose)
	defer func() { finish(resultErr) }()

	if s.stop != nil {
		s.stop()
	}
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (s Server) ObserveSession(ctx context.Context, req api.ObserveSessionRequest) (response api.ObserveSessionResponse, resultErr error) {
	req.Options.Verbose = req.Options.Verbose || s.verbose
	ctx, finish := s.beginRequest(ctx, "observe_session", req.Options.Verbose,
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("screenshot", req.Options.WithScreenshot),
		diagnostic.Value("full", req.Options.FullScreenshot),
		diagnostic.Value("recover", req.Options.RecoverScreenshot),
		diagnostic.Value("timeout_ms", req.Options.TimeoutMS),
	)
	defer func() { finish(resultErr) }()

	observeCtx, cancel := observeSessionContext(ctx, req.Options)
	defer cancel()

	observation, err := s.sessions.Observe(observeCtx, req.SessionID, req.Options)
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

// InspectStyles returns computed styles and best-effort authored declarations
// for one recently observed node.
func (s Server) InspectStyles(ctx context.Context, req api.InspectStylesRequest) (response api.InspectStylesResponse, resultErr error) {
	ctx, finish := s.beginRequest(ctx, "inspect_styles", s.verbose,
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("node_ref", req.NodeRef),
	)
	defer func() { finish(resultErr) }()

	inspection, err := s.sessions.InspectStyles(ctx, req)
	if err != nil {
		return api.InspectStylesResponse{}, err
	}
	return api.InspectStylesResponse{Inspection: inspection}, nil
}

func (s Server) ActSession(ctx context.Context, req api.ActSessionRequest) (response api.ActSessionResponse, resultErr error) {
	ctx, finish := s.beginRequest(ctx, "act_session", s.verbose,
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("action", req.Action.Kind),
	)
	defer func() { finish(resultErr) }()

	result, err := s.sessions.Act(ctx, req.SessionID, req.Action)
	if err != nil {
		return api.ActSessionResponse{}, err
	}

	return api.ActSessionResponse{Result: result}, nil
}

func (s Server) Shutdown(ctx context.Context) (resultErr error) {
	ctx, finish := s.beginRequest(ctx, "shutdown", s.verbose)
	defer func() { finish(resultErr) }()
	return s.sessions.Shutdown(ctx)
}

func (s Server) beginRequest(ctx context.Context, request string, verbose bool, fields ...diagnostic.Field) (context.Context, func(error)) {
	trace := diagnostic.FromContext(ctx)
	owned := trace == nil
	if owned {
		trace = s.newTrace(request, verbose)
		ctx = diagnostic.WithTrace(ctx, trace)
	} else {
		trace.SetVerbose(verbose)
	}

	startedAt := time.Now()
	trace.Event("daemon_request", "start", fields...)
	return ctx, func(err error) {
		finishFields := []diagnostic.Field{
			diagnostic.Value("duration_ms", time.Since(startedAt).Milliseconds()),
		}
		if err != nil {
			finishFields = append(finishFields, diagnostic.Value("error", err))
		}
		trace.Event("daemon_request", "finish", finishFields...)
		if owned {
			trace.Finish(err)
		}
	}
}

func (s Server) newTrace(request string, verbose bool) *diagnostic.Trace {
	return diagnostic.New(request, verbose, s.emitLog, daemonEnvironment()...)
}

func (s Server) emitLog(message string) {
	if s.logger != nil {
		s.logger(message)
		return
	}
	log.Print(message)
}

func daemonEnvironment() []diagnostic.Field {
	fields := diagnostic.RuntimeEnvironment()
	return append(fields,
		diagnostic.Value("daemon_version", api.DaemonVersion),
		diagnostic.Value("protocol_version", api.ProtocolVersion),
		diagnostic.Value("daemon_pid", os.Getpid()),
	)
}

func runtimeEnvironmentValue(key string) string {
	for _, field := range diagnostic.RuntimeEnvironment() {
		if field.Key == key {
			if value, ok := field.Value.(string); ok {
				return value
			}
		}
	}
	return "unknown"
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
