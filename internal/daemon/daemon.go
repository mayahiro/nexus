package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
	"github.com/mayahiro/nexus/internal/session"
)

type Server struct {
	sessions *session.Manager
	stop     context.CancelFunc
}

const shutdownTimeout = 5 * time.Second

type RunOptions struct {
	IdleTimeout time.Duration
}

func Run(ctx context.Context, paths config.Paths, opts RunOptions) error {
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		return err
	}

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
	session, err := s.sessions.Attach(ctx, req)
	if err != nil {
		return api.AttachSessionResponse{}, err
	}

	return api.AttachSessionResponse{Session: session}, nil
}

func (s Server) ListSessions(_ context.Context, _ api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{
		Sessions: s.sessions.List(),
	}, nil
}

func (s Server) DetachSession(ctx context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	session, err := s.sessions.Detach(ctx, req.SessionID)
	if err != nil {
		return api.DetachSessionResponse{}, err
	}

	if len(s.sessions.List()) == 0 && s.stop != nil {
		s.stop()
	}

	return api.DetachSessionResponse{Session: session}, nil
}

func (s Server) StopDaemon(_ context.Context, _ api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	if s.stop != nil {
		s.stop()
	}
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (s Server) ObserveSession(ctx context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	observation, err := s.sessions.Observe(ctx, req.SessionID, req.Options)
	if err != nil {
		return api.ObserveSessionResponse{}, err
	}

	return api.ObserveSessionResponse{Observation: observation}, nil
}

func (s Server) ActSession(ctx context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	result, err := s.sessions.Act(ctx, req.SessionID, req.Action)
	if err != nil {
		return api.ActSessionResponse{}, err
	}

	return api.ActSessionResponse{Result: result}, nil
}

func (s Server) Shutdown(ctx context.Context) error {
	return s.sessions.Shutdown(ctx)
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
