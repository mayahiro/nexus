package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
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

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
}

type request struct {
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type response struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func Dial(ctx context.Context, path string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := setDeadline(ctx, c.conn); err != nil {
		return err
	}
	defer clearDeadline(c.conn)

	if err := c.enc.Encode(request{Method: method, Params: params}); err != nil {
		return err
	}

	var res response
	if err := c.dec.Decode(&res); err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(res.Result, result)
}

func Serve(ctx context.Context, listener net.Listener, handler Handler, opts ServeOptions) error {
	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		listener.Close()
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

		wg.Add(1)
		go func() {
			defer wg.Done()
			serveConn(ctx, conn, handler, opts)
		}()
	}

	wg.Wait()
	return nil
}

func serveConn(ctx context.Context, conn net.Conn, handler Handler, opts ServeOptions) {
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	for {
		if err := setDeadline(ctx, conn); err != nil {
			writeError(enc, err)
			return
		}

		var req request
		if err := dec.Decode(&req); err != nil {
			return
		}

		if opts.OnActivity != nil {
			opts.OnActivity()
		}

		switch req.Method {
		case "ping":
			params, err := decodeParams[api.PingRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.Ping(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "attach_session":
			params, err := decodeParams[api.AttachSessionRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.AttachSession(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "list_sessions":
			params, err := decodeParams[api.ListSessionsRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.ListSessions(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "detach_session":
			params, err := decodeParams[api.DetachSessionRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.DetachSession(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "stop_daemon":
			params, err := decodeParams[api.StopDaemonRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.StopDaemon(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "observe_session":
			params, err := decodeParams[api.ObserveSessionRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.ObserveSession(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		case "act_session":
			params, err := decodeParams[api.ActSessionRequest](req.Params)
			if err != nil {
				writeError(enc, err)
				return
			}

			res, err := handler.ActSession(ctx, params)
			if err != nil {
				writeError(enc, err)
				return
			}

			if err := writeResult(enc, res); err != nil {
				return
			}
		default:
			writeError(enc, fmt.Errorf("unknown method: %s", req.Method))
			return
		}
	}
}

func writeError(enc *json.Encoder, err error) {
	enc.Encode(struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}

func writeResult(enc *json.Encoder, result interface{}) error {
	return enc.Encode(struct {
		Result interface{} `json:"result"`
	}{Result: result})
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
