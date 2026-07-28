package rpc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
)

type testHandler struct{}

type binaryTestHandler struct {
	testHandler
}

type cancelTestHandler struct {
	testHandler
	entered  chan struct{}
	canceled chan struct{}
}

type errorTestHandler struct {
	testHandler
}

type panicTestHandler struct {
	testHandler
}

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

type pipeAddress struct{}

func newPipeListener(conn net.Conn) *pipeListener {
	connections := make(chan net.Conn, 1)
	connections <- conn
	return &pipeListener{
		connections: connections,
		closed:      make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (*pipeListener) Addr() net.Addr {
	return pipeAddress{}
}

func (pipeAddress) Network() string {
	return "pipe"
}

func (pipeAddress) String() string {
	return "pipe"
}

func (binaryTestHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			SessionID:      req.SessionID,
			TargetType:     "browser",
			Title:          "example",
			ScreenshotData: []byte{0, 1, 2, 3, 255},
		},
	}, nil
}

func (h cancelTestHandler) ObserveSession(ctx context.Context, _ api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	close(h.entered)
	<-ctx.Done()
	close(h.canceled)
	return api.ObserveSessionResponse{}, ctx.Err()
}

func (errorTestHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, errors.New("expected server error")
}

func (panicTestHandler) ActSession(context.Context, api.ActSessionRequest) (api.ActSessionResponse, error) {
	panic("expected handler panic")
}

func (testHandler) Ping(_ context.Context, _ api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   api.DaemonVersion,
	}, nil
}

func (testHandler) AttachSession(_ context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{
		Session: api.Session{
			ID:         req.SessionID,
			TargetType: req.TargetType,
			Backend:    req.Backend,
		},
	}, nil
}

func (testHandler) ListSessions(_ context.Context, _ api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{
		Sessions: []api.Session{
			{ID: "web1", TargetType: "browser", Backend: "chromium"},
		},
	}, nil
}

func (testHandler) DetachSession(_ context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{
		Session: api.Session{
			ID: req.SessionID,
		},
	}, nil
}

func (testHandler) StopDaemon(_ context.Context, _ api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (testHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			SessionID:  req.SessionID,
			TargetType: "browser",
			Title:      "example",
		},
	}, nil
}

func (testHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	var value interface{}
	switch req.Action.Text {
	case "false":
		value = false
	case "0":
		value = 0
	case `""`:
		value = ""
	default:
		value = req.Action.Text
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
			Value:   value,
		},
	}, nil
}

func TestPing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socket := filepath.Join(t.TempDir(), "nxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, testHandler{}, ServeOptions{})
	}()

	client, err := Dial(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	res, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", res.ProtocolVersion)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestSessionRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socket := filepath.Join(t.TempDir(), "nxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, testHandler{}, ServeOptions{})
	}()

	client, err := Dial(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	attached, err := client.AttachSession(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "chromium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attached.Session.ID != "web1" {
		t.Fatalf("unexpected attach result: %+v", attached)
	}

	listed, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != "web1" {
		t.Fatalf("unexpected sessions result: %+v", listed)
	}

	detached, err := client.DetachSession(context.Background(), api.DetachSessionRequest{
		SessionID: "web1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if detached.Session.ID != "web1" {
		t.Fatalf("unexpected detach result: %+v", detached)
	}

	observed, err := client.ObserveSession(context.Background(), api.ObserveSessionRequest{
		SessionID: "web1",
		Options:   api.ObserveOptions{WithText: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Observation.SessionID != "web1" {
		t.Fatalf("unexpected observe result: %+v", observed)
	}

	acted, err := client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "document.title",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acted.Result.Value != "document.title" {
		t.Fatalf("unexpected act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "false",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(bool); !ok || value {
		t.Fatalf("unexpected false act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(float64); !ok || value != 0 {
		t.Fatalf("unexpected zero act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: `""`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(string); !ok || value != "" {
		t.Fatalf("unexpected empty-string act result: %+v", acted)
	}

	stopped, err := client.StopDaemon(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !stopped.Stopped {
		t.Fatalf("unexpected stop result: %+v", stopped)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestBinaryObservationOverPipe(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		serveConn(ctx, serverConn, binaryTestHandler{}, ServeOptions{})
		close(done)
	}()

	client := &Client{
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
		writer: bufio.NewWriter(clientConn),
	}
	res, err := client.ObserveSession(context.Background(), api.ObserveSessionRequest{
		SessionID: "web1",
		Options:   api.ObserveOptions{WithScreenshot: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0, 1, 2, 3, 255}
	if !bytes.Equal(res.Observation.ScreenshotData, expected) {
		t.Fatalf("unexpected screenshot bytes: %v", res.Observation.ScreenshotData)
	}
	if res.Observation.Screenshot != "" {
		t.Fatalf("unexpected base64 screenshot on RPC wire: %q", res.Observation.Screenshot)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pipe RPC server did not stop")
	}
}

func TestServeRecoversHandlerPanic(t *testing.T) {
	firstServerConn, firstClientConn := net.Pipe()
	secondServerConn, secondClientConn := net.Pipe()
	listener := &pipeListener{
		connections: make(chan net.Conn, 2),
		closed:      make(chan struct{}),
	}
	listener.connections <- firstServerConn
	listener.connections <- secondServerConn

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- Serve(ctx, listener, panicTestHandler{}, ServeOptions{})
	}()

	firstClient := &Client{
		conn:   firstClientConn,
		reader: bufio.NewReader(firstClientConn),
		writer: bufio.NewWriter(firstClientConn),
	}
	_, err := firstClient.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action:    api.Action{Kind: "eval", Text: "1+1"},
	})
	if err == nil || err.Error() != "internal daemon error" {
		t.Fatalf("unexpected panic response: %v", err)
	}
	firstClient.Close()

	secondClient := &Client{
		conn:   secondClientConn,
		reader: bufio.NewReader(secondClientConn),
		writer: bufio.NewWriter(secondClientConn),
	}
	response, err := secondClient.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", response.ProtocolVersion)
	}
	secondClient.Close()

	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("RPC server did not stop after recovered handler panic")
	}
}

func TestServerRejectsRequestWithoutProtocolVersion(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		serveConn(ctx, serverConn, testHandler{}, ServeOptions{})
		close(done)
	}()

	reader := bufio.NewReader(clientConn)
	writer := bufio.NewWriter(clientConn)
	if err := writeJSONLine(writer, request{Method: "ping", Params: api.PingRequest{}}); err != nil {
		t.Fatal(err)
	}
	var res response
	if err := readJSONLine(reader, &res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Error, "RPC protocol mismatch") {
		t.Fatalf("unexpected protocol error: %q", res.Error)
	}
	clientConn.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RPC server did not close the incompatible connection")
	}
}

func TestClientCallStopsWhenContextWithoutDeadlineIsCanceled(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	serverRead := make(chan struct{})
	go func() {
		reader := bufio.NewReader(serverConn)
		var req request
		if readJSONLine(reader, &req) == nil {
			close(serverRead)
		}
	}()

	client := &Client{
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
		writer: bufio.NewWriter(clientConn),
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Ping(ctx)
		result <- err
	}()

	select {
	case <-serverRead:
	case <-time.After(time.Second):
		t.Fatal("server did not receive request")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled call, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled client call did not stop")
	}
}

func TestClientReconnectsAfterCanceledCall(t *testing.T) {
	firstServerConn, firstClientConn := net.Pipe()
	defer firstServerConn.Close()
	secondServerConn, secondClientConn := net.Pipe()

	firstRequest := make(chan struct{})
	go func() {
		reader := bufio.NewReader(firstServerConn)
		var req request
		if readJSONLine(reader, &req) == nil {
			close(firstRequest)
		}
	}()

	dialed := false
	client := &Client{
		conn:   firstClientConn,
		reader: bufio.NewReader(firstClientConn),
		writer: bufio.NewWriter(firstClientConn),
		dial: func(context.Context) (net.Conn, error) {
			if dialed {
				return nil, errors.New("unexpected extra dial")
			}
			dialed = true
			return secondClientConn, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	firstCall := make(chan error, 1)
	go func() {
		_, err := client.Ping(ctx)
		firstCall <- err
	}()
	select {
	case <-firstRequest:
	case <-time.After(time.Second):
		t.Fatal("server did not receive first request")
	}
	cancel()
	if err := <-firstCall; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled first call, got %v", err)
	}

	serverDone := make(chan struct{})
	go func() {
		serveConn(context.Background(), secondServerConn, testHandler{}, ServeOptions{})
		close(serverDone)
	}()
	response, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", response.ProtocolVersion)
	}
	if !dialed {
		t.Fatal("client did not reconnect")
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("reconnected server did not stop")
	}
}

func TestClientReconnectsAfterServerError(t *testing.T) {
	firstServerConn, firstClientConn := net.Pipe()
	secondServerConn, secondClientConn := net.Pipe()

	firstServerDone := make(chan struct{})
	go func() {
		serveConn(context.Background(), firstServerConn, errorTestHandler{}, ServeOptions{})
		close(firstServerDone)
	}()

	dialed := false
	client := &Client{
		conn:   firstClientConn,
		reader: bufio.NewReader(firstClientConn),
		writer: bufio.NewWriter(firstClientConn),
		dial: func(context.Context) (net.Conn, error) {
			if dialed {
				return nil, errors.New("unexpected extra dial")
			}
			dialed = true
			return secondClientConn, nil
		},
	}
	if _, err := client.Ping(context.Background()); err == nil || err.Error() != "expected server error" {
		t.Fatalf("unexpected first call error: %v", err)
	}
	select {
	case <-firstServerDone:
	case <-time.After(time.Second):
		t.Fatal("first server did not stop")
	}

	secondServerDone := make(chan struct{})
	go func() {
		serveConn(context.Background(), secondServerConn, testHandler{}, ServeOptions{})
		close(secondServerDone)
	}()
	response, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", response.ProtocolVersion)
	}
	if !dialed {
		t.Fatal("client did not reconnect")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondServerDone:
	case <-time.After(time.Second):
		t.Fatal("second server did not stop")
	}
}

func TestClientDisconnectCancelsUnixSocketHandler(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverFile := os.NewFile(uintptr(fds[0]), "rpc-server")
	clientFile := os.NewFile(uintptr(fds[1]), "rpc-client")
	serverConn, err := net.FileConn(serverFile)
	if err != nil {
		serverFile.Close()
		clientFile.Close()
		t.Fatal(err)
	}
	clientConn, err := net.FileConn(clientFile)
	serverFile.Close()
	clientFile.Close()
	if err != nil {
		serverConn.Close()
		t.Fatal(err)
	}

	handler := cancelTestHandler{
		entered:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	serverDone := make(chan struct{})
	go func() {
		serveConn(context.Background(), serverConn, handler, ServeOptions{})
		close(serverDone)
	}()

	client := &Client{
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
		writer: bufio.NewWriter(clientConn),
	}
	callDone := make(chan error, 1)
	go func() {
		_, err := client.ObserveSession(context.Background(), api.ObserveSessionRequest{SessionID: "web1"})
		callDone <- err
	}()

	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-handler.canceled:
	case <-time.After(time.Second):
		t.Fatal("handler was not canceled after client disconnect")
	}
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("client call did not stop")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("server connection did not stop")
	}
}

func TestServeCancellationClosesIdleConnections(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	listener := newPipeListener(serverConn)
	ctx, cancel := context.WithCancel(context.Background())

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- Serve(ctx, listener, testHandler{}, ServeOptions{})
	}()

	client := &Client{
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
		writer: bufio.NewWriter(clientConn),
	}
	response, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", response.ProtocolVersion)
	}

	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("RPC server did not close its idle connection")
	}
	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
