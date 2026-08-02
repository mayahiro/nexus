package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/diagnostic"
)

type Handler interface {
	Ping(ctx context.Context, req api.PingRequest) (api.PingResponse, error)
	AttachSession(ctx context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error)
	ListSessions(ctx context.Context, req api.ListSessionsRequest) (api.ListSessionsResponse, error)
	DetachSession(ctx context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error)
	StopDaemon(ctx context.Context, req api.StopDaemonRequest) (api.StopDaemonResponse, error)
	ObserveSession(ctx context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error)
	ActSession(ctx context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error)
}

type ServeOptions struct {
	OnActivity func()
	NewTrace   func(method string) *diagnostic.Trace
}

const maxBinaryResponseSize int64 = 512 << 20
const connectionShutdownGrace = 100 * time.Millisecond

type Client struct {
	mu     sync.Mutex
	dial   func(context.Context) (net.Conn, error)
	active map[net.Conn]struct{}
	closed bool
}

type request struct {
	ProtocolVersion string      `json:"protocol_version,omitempty"`
	Method          string      `json:"method"`
	Params          interface{} `json:"params,omitempty"`
}

type response struct {
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	BinarySize int64           `json:"binary_size,omitempty"`
}

func Dial(ctx context.Context, path string) (*Client, error) {
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}
	conn, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Close(); err != nil {
		return nil, err
	}

	return newClient(dial), nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	connections := make([]net.Conn, 0, len(c.active))
	for conn := range c.active {
		connections = append(connections, conn)
	}
	c.active = nil
	c.mu.Unlock()

	var closeErr error
	for _, conn := range connections {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

func (c *Client) Ping(ctx context.Context) (api.PingResponse, error) {
	var res api.PingResponse
	err := c.call(ctx, "ping", api.PingRequest{ProtocolVersion: api.ProtocolVersion}, &res)
	return res, err
}

func (c *Client) AttachSession(ctx context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	var res api.AttachSessionResponse
	err := c.call(ctx, "attach_session", req, &res)
	return res, err
}

func (c *Client) ListSessions(ctx context.Context) (api.ListSessionsResponse, error) {
	var res api.ListSessionsResponse
	err := c.call(ctx, "list_sessions", api.ListSessionsRequest{}, &res)
	return res, err
}

func (c *Client) DetachSession(ctx context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	var res api.DetachSessionResponse
	err := c.call(ctx, "detach_session", req, &res)
	return res, err
}

func (c *Client) StopDaemon(ctx context.Context) (api.StopDaemonResponse, error) {
	var res api.StopDaemonResponse
	err := c.call(ctx, "stop_daemon", api.StopDaemonRequest{}, &res)
	return res, err
}

func (c *Client) ObserveSession(ctx context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	var res api.ObserveSessionResponse
	err := c.call(ctx, "observe_session", req, &res)
	return res, err
}

func (c *Client) ActSession(ctx context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	var res api.ActSessionResponse
	err := c.call(ctx, "act_session", req, &res)
	return res, err
}

func (c *Client) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	conn, err := c.openConnection(ctx)
	if err != nil {
		return err
	}
	defer c.releaseConnection(conn)
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := setDeadline(ctx, conn); err != nil {
		return err
	}
	cancelFinished := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		conn.Close()
		close(cancelFinished)
	})
	defer func() {
		if !stopCancel() {
			<-cancelFinished
		}
		clearDeadline(conn)
	}()

	if err := writeJSONLine(writer, request{
		ProtocolVersion: api.ProtocolVersion,
		Method:          method,
		Params:          params,
	}); err != nil {
		return requestError(ctx, err)
	}

	var res response
	if err := readJSONLine(reader, &res); err != nil {
		return requestError(ctx, err)
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	if result != nil {
		if err := json.Unmarshal(res.Result, result); err != nil {
			return err
		}
	}
	if res.BinarySize < 0 {
		return fmt.Errorf("invalid RPC binary response size: %d", res.BinarySize)
	}
	if res.BinarySize == 0 {
		return nil
	}
	if res.BinarySize > maxBinaryResponseSize {
		return fmt.Errorf("RPC binary response is too large: %d bytes", res.BinarySize)
	}
	if result == nil {
		return errors.New("unexpected binary RPC response")
	}

	data := make([]byte, res.BinarySize)
	if _, err := io.ReadFull(reader, data); err != nil {
		return requestError(ctx, err)
	}
	switch value := result.(type) {
	case *api.ObserveSessionResponse:
		value.Observation.ScreenshotData = data
	default:
		return fmt.Errorf("binary RPC response is unsupported for %T", result)
	}
	return nil
}

func newClient(dial func(context.Context) (net.Conn, error)) *Client {
	return &Client{
		dial:   dial,
		active: map[net.Conn]struct{}{},
	}
}

func (c *Client) openConnection(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, net.ErrClosed
	}
	dial := c.dial
	c.mu.Unlock()
	if dial == nil {
		return nil, net.ErrClosed
	}

	conn, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		conn.Close()
		return nil, net.ErrClosed
	}
	if c.active == nil {
		c.active = map[net.Conn]struct{}{}
	}
	c.active[conn] = struct{}{}
	c.mu.Unlock()
	return conn, nil
}

func (c *Client) releaseConnection(conn net.Conn) {
	c.mu.Lock()
	delete(c.active, conn)
	c.mu.Unlock()
	conn.Close()
}

func requestError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func Serve(ctx context.Context, listener net.Listener, handler Handler, opts ServeOptions) error {
	var wg sync.WaitGroup
	var connectionsMu sync.Mutex
	connections := map[net.Conn]struct{}{}

	go func() {
		<-ctx.Done()
		listener.Close()
		timer := time.NewTimer(connectionShutdownGrace)
		defer timer.Stop()
		<-timer.C

		connectionsMu.Lock()
		for conn := range connections {
			conn.Close()
		}
		connectionsMu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				break
			}
			return err
		}

		if opts.OnActivity != nil {
			opts.OnActivity()
		}

		connectionsMu.Lock()
		connections[conn] = struct{}{}
		connectionsMu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				connectionsMu.Lock()
				delete(connections, conn)
				connectionsMu.Unlock()
			}()
			serveConn(ctx, conn, handler, opts)
		}()
	}

	wg.Wait()
	return nil
}

func serveConn(ctx context.Context, conn net.Conn, handler Handler, opts ServeOptions) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	defer recoverHandlerPanic(writer)

	handledRequests := 0
	for {
		if err := setDeadline(ctx, conn); err != nil {
			if handledRequests > 0 {
				return
			}
			trace := newRequestTrace(opts, "unknown")
			trace.Event("rpc_deadline", "failure", diagnostic.Value("error", err))
			writeErr := writeError(writer, err)
			trace.Finish(errors.Join(err, writeErr))
			return
		}

		var req request
		if err := readJSONLine(reader, &req); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
				trace := newRequestTrace(opts, "unknown")
				trace.Event("rpc_request", "decode_failure", diagnostic.Value("error", err))
				trace.Finish(err)
			}
			return
		}

		trace := newRequestTrace(opts, req.Method)
		trace.Event("rpc_request", "decoded",
			diagnostic.Value("protocol_version", req.ProtocolVersion),
		)

		if opts.OnActivity != nil {
			opts.OnActivity()
		}

		requestCtx, finishRequest := contextForRequest(ctx, conn, trace)
		requestCtx = diagnostic.WithTrace(requestCtx, trace)

		var result interface{}
		var binary []byte
		var requestErr error
		if req.ProtocolVersion != api.ProtocolVersion {
			requestErr = fmt.Errorf(
				"RPC protocol mismatch: client=%q daemon=%q",
				req.ProtocolVersion,
				api.ProtocolVersion,
			)
		} else {
			result, binary, requestErr = dispatchRequestSafely(requestCtx, req, handler)
		}

		if requestErr != nil {
			if ctx.Err() != nil {
				trace.Event("rpc_connection", "server_context_canceled",
					diagnostic.Value("error", ctx.Err()),
				)
			}
			trace.Event("rpc_handler", "failure", diagnostic.Value("error", requestErr))
			writeErr := writeError(writer, requestErr)
			if writeErr != nil {
				trace.Event("rpc_response", "write_failure", diagnostic.Value("error", writeErr))
			}
			finishRequest()
			trace.Finish(errors.Join(requestErr, writeErr))
			return
		}

		trace.Event("rpc_handler", "finish",
			diagnostic.Value("binary_bytes", len(binary)),
		)
		responseErr := writeResult(writer, result, binary)
		if responseErr != nil {
			trace.Event("rpc_response", "write_failure",
				diagnostic.Value("binary_bytes", len(binary)),
				diagnostic.Value("error", responseErr),
			)
		} else {
			trace.Event("rpc_response", "finish",
				diagnostic.Value("binary_bytes", len(binary)),
			)
		}
		finishRequest()
		trace.Finish(responseErr)
		if responseErr != nil {
			return
		}
		handledRequests++
	}
}

func newRequestTrace(opts ServeOptions, method string) *diagnostic.Trace {
	if opts.NewTrace != nil {
		if trace := opts.NewTrace(method); trace != nil {
			return trace
		}
	}
	return diagnostic.New(method, false, nil)
}

func dispatchRequestSafely(ctx context.Context, req request, handler Handler) (result interface{}, binary []byte, resultErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("RPC handler panic: %v\n%s", recovered, debug.Stack())
			result = nil
			binary = nil
			resultErr = errors.New("internal daemon error")
		}
	}()
	return dispatchRequest(ctx, req, handler)
}

func dispatchRequest(ctx context.Context, req request, handler Handler) (interface{}, []byte, error) {
	switch req.Method {
	case "ping":
		params, err := decodeParams[api.PingRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.Ping(ctx, params)
		return res, nil, err
	case "attach_session":
		params, err := decodeParams[api.AttachSessionRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.AttachSession(ctx, params)
		return res, nil, err
	case "list_sessions":
		params, err := decodeParams[api.ListSessionsRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.ListSessions(ctx, params)
		return res, nil, err
	case "detach_session":
		params, err := decodeParams[api.DetachSessionRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.DetachSession(ctx, params)
		return res, nil, err
	case "stop_daemon":
		params, err := decodeParams[api.StopDaemonRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.StopDaemon(ctx, params)
		return res, nil, err
	case "observe_session":
		params, err := decodeParams[api.ObserveSessionRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.ObserveSession(ctx, params)
		if err != nil {
			return nil, nil, err
		}
		screenshot := res.Observation.ScreenshotData
		res.Observation.ScreenshotData = nil
		if len(screenshot) > 0 {
			res.Observation.Screenshot = ""
		}
		return res, screenshot, nil
	case "act_session":
		params, err := decodeParams[api.ActSessionRequest](req.Params)
		if err != nil {
			return nil, nil, err
		}
		res, err := handler.ActSession(ctx, params)
		return res, nil, err
	default:
		return nil, nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

func recoverHandlerPanic(writer *bufio.Writer) {
	recovered := recover()
	if recovered == nil {
		return
	}
	log.Printf("RPC handler panic: %v\n%s", recovered, debug.Stack())
	writeError(writer, errors.New("internal daemon error"))
}

func writeError(writer *bufio.Writer, err error) error {
	if writeErr := writeJSONLine(writer, struct {
		Error string `json:"error"`
	}{Error: err.Error()}); writeErr != nil {
		return fmt.Errorf("write RPC error response: %w", writeErr)
	}
	return nil
}

func writeResult(writer *bufio.Writer, result interface{}, binary []byte) error {
	if int64(len(binary)) > maxBinaryResponseSize {
		return fmt.Errorf("RPC binary response is too large: %d bytes", len(binary))
	}
	if err := writeJSONLine(writer, struct {
		Result     interface{} `json:"result"`
		BinarySize int         `json:"binary_size,omitempty"`
	}{Result: result, BinarySize: len(binary)}); err != nil {
		return fmt.Errorf("write RPC response header: %w", err)
	}
	if len(binary) == 0 {
		return nil
	}
	written, err := writer.Write(binary)
	if err != nil {
		return fmt.Errorf("write RPC binary response: wrote %d of %d bytes: %w", written, len(binary), err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush RPC binary response: %d bytes: %w", len(binary), err)
	}
	return nil
}

func decodeParams[T any](value interface{}) (T, error) {
	var params T

	raw, err := json.Marshal(value)
	if err != nil {
		return params, err
	}

	if err := json.Unmarshal(raw, &params); err != nil {
		return params, err
	}

	return params, nil
}

func setDeadline(ctx context.Context, conn net.Conn) error {
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	return conn.SetDeadline(time.Time{})
}

func clearDeadline(conn net.Conn) {
	conn.SetDeadline(time.Time{})
}

func writeJSONLine(writer *bufio.Writer, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	if err := writer.WriteByte('\n'); err != nil {
		return err
	}
	return writer.Flush()
}

func readJSONLine(reader *bufio.Reader, value interface{}) error {
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func contextForRequest(parent context.Context, conn net.Conn, trace *diagnostic.Trace) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if connectionClosed(conn) {
					trace.Event("rpc_connection", "client_disconnected")
					cancel()
					return
				}
			}
		}
	}()

	return ctx, func() {
		once.Do(func() {
			close(done)
			cancel()
		})
	}
}

func connectionClosed(conn net.Conn) bool {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return true
	}

	closed := false
	controlErr := rawConn.Control(func(fd uintptr) {
		buffer := []byte{0}
		count, _, recvErr := syscall.Recvfrom(int(fd), buffer, syscall.MSG_PEEK|syscall.MSG_DONTWAIT)
		switch {
		case recvErr == nil:
			closed = count == 0
		case errors.Is(recvErr, syscall.EAGAIN), errors.Is(recvErr, syscall.EWOULDBLOCK), errors.Is(recvErr, syscall.EINTR):
			closed = false
		default:
			closed = true
		}
	})
	return controlErr != nil || closed
}
