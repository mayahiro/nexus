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
}

const maxBinaryResponseSize int64 = 512 << 20
const connectionShutdownGrace = 100 * time.Millisecond

type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	dial    func(context.Context) (net.Conn, error)
	closed  bool
	callMu  sync.Mutex
	stateMu sync.Mutex
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

	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		dial:   dial,
	}, nil
}

func (c *Client) Close() error {
	c.stateMu.Lock()

	c.closed = true
	if c.conn == nil {
		c.stateMu.Unlock()
		return nil
	}
	conn := c.conn
	c.clearConnectionLocked(conn)
	c.stateMu.Unlock()
	return conn.Close()
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
	c.callMu.Lock()
	defer c.callMu.Unlock()

	conn, reader, writer, err := c.connection(ctx)
	if err != nil {
		return err
	}
	healthy := true
	defer func() {
		if !healthy {
			c.discardConnection(conn)
		}
	}()

	if err := setDeadline(ctx, conn); err != nil {
		healthy = false
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
			healthy = false
		}
		clearDeadline(conn)
	}()

	if err := writeJSONLine(writer, request{
		ProtocolVersion: api.ProtocolVersion,
		Method:          method,
		Params:          params,
	}); err != nil {
		healthy = false
		return requestError(ctx, err)
	}

	var res response
	if err := readJSONLine(reader, &res); err != nil {
		healthy = false
		return requestError(ctx, err)
	}
	if res.Error != "" {
		healthy = false
		return errors.New(res.Error)
	}
	if result != nil {
		if err := json.Unmarshal(res.Result, result); err != nil {
			healthy = false
			return err
		}
	}
	if res.BinarySize < 0 {
		healthy = false
		return fmt.Errorf("invalid RPC binary response size: %d", res.BinarySize)
	}
	if res.BinarySize == 0 {
		return nil
	}
	if res.BinarySize > maxBinaryResponseSize {
		healthy = false
		return fmt.Errorf("RPC binary response is too large: %d bytes", res.BinarySize)
	}
	if result == nil {
		healthy = false
		return errors.New("unexpected binary RPC response")
	}

	data := make([]byte, res.BinarySize)
	if _, err := io.ReadFull(reader, data); err != nil {
		healthy = false
		return requestError(ctx, err)
	}
	switch value := result.(type) {
	case *api.ObserveSessionResponse:
		value.Observation.ScreenshotData = data
	default:
		healthy = false
		return fmt.Errorf("binary RPC response is unsupported for %T", result)
	}
	return nil
}

func (c *Client) connection(ctx context.Context) (net.Conn, *bufio.Reader, *bufio.Writer, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.closed {
		return nil, nil, nil, net.ErrClosed
	}
	if c.conn != nil {
		return c.conn, c.reader, c.writer, nil
	}
	if c.dial == nil {
		return nil, nil, nil, net.ErrClosed
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	return c.conn, c.reader, c.writer, nil
}

func (c *Client) discardConnection(conn net.Conn) {
	c.stateMu.Lock()
	c.clearConnectionLocked(conn)
	c.stateMu.Unlock()
	conn.Close()
}

func (c *Client) clearConnectionLocked(conn net.Conn) {
	if c.conn != conn {
		return
	}
	c.conn = nil
	c.reader = nil
	c.writer = nil
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

	for {
		if err := setDeadline(ctx, conn); err != nil {
			writeError(writer, err)
			return
		}

		var req request
		if err := readJSONLine(reader, &req); err != nil {
			return
		}
		if req.ProtocolVersion != api.ProtocolVersion {
			writeError(writer, fmt.Errorf(
				"RPC protocol mismatch: client=%q daemon=%q",
				req.ProtocolVersion,
				api.ProtocolVersion,
			))
			return
		}

		if opts.OnActivity != nil {
			opts.OnActivity()
		}

		requestCtx, finishRequest := contextForRequest(ctx, conn)
		var responseErr error
		switch req.Method {
		case "ping":
			params, err := decodeParams[api.PingRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.Ping(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		case "attach_session":
			params, err := decodeParams[api.AttachSessionRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.AttachSession(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		case "list_sessions":
			params, err := decodeParams[api.ListSessionsRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.ListSessions(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		case "detach_session":
			params, err := decodeParams[api.DetachSessionRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.DetachSession(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		case "stop_daemon":
			params, err := decodeParams[api.StopDaemonRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.StopDaemon(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		case "observe_session":
			params, err := decodeParams[api.ObserveSessionRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.ObserveSession(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			screenshot := res.Observation.ScreenshotData
			res.Observation.ScreenshotData = nil
			if len(screenshot) > 0 {
				res.Observation.Screenshot = ""
			}
			responseErr = writeResult(writer, res, screenshot)
		case "act_session":
			params, err := decodeParams[api.ActSessionRequest](req.Params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			res, err := handler.ActSession(requestCtx, params)
			if err != nil {
				writeError(writer, err)
				finishRequest()
				return
			}

			responseErr = writeResult(writer, res, nil)
		default:
			writeError(writer, fmt.Errorf("unknown method: %s", req.Method))
			finishRequest()
			return
		}
		finishRequest()
		if responseErr != nil {
			return
		}
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

func writeError(writer *bufio.Writer, err error) {
	writeJSONLine(writer, struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}

func writeResult(writer *bufio.Writer, result interface{}, binary []byte) error {
	if int64(len(binary)) > maxBinaryResponseSize {
		return fmt.Errorf("RPC binary response is too large: %d bytes", len(binary))
	}
	if err := writeJSONLine(writer, struct {
		Result     interface{} `json:"result"`
		BinarySize int         `json:"binary_size,omitempty"`
	}{Result: result, BinarySize: len(binary)}); err != nil {
		return err
	}
	if len(binary) == 0 {
		return nil
	}
	if _, err := writer.Write(binary); err != nil {
		return err
	}
	return writer.Flush()
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

func contextForRequest(parent context.Context, conn net.Conn) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if connectionClosed(conn) {
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
