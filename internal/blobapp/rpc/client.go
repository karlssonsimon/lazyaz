package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	blobcore "azure-storage/internal/blobapp/core"
	sharedrpc "azure-storage/internal/rpc"
)

type Client struct {
	inner   *sharedrpc.Client
	session string
}

type Request = sharedrpc.Request
type Response = sharedrpc.Response

type SessionCreateResult struct {
	Session string            `json:"session"`
	State   blobcore.Snapshot `json:"state"`
}

type ActionInvokeResult struct {
	Action blobcore.ActionResult `json:"action"`
	State  blobcore.Snapshot     `json:"state"`
}

func Dial(socketPath string) (*Client, error) {
	inner, err := sharedrpc.DialUnix(socketPath)
	if err != nil {
		return nil, err
	}
	return &Client{inner: inner}, nil
}

func (c *Client) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

func (c *Client) Session() string { return c.session }

func (c *Client) Call(req Request) (Response, error) {
	if req.Session == "" {
		req.Session = c.session
	}
	resp, err := c.inner.Call(sharedrpc.Request(req))
	if err != nil {
		return Response{}, err
	}
	return Response(resp), nil
}

func (c *Client) CreateSession() (blobcore.Snapshot, error) {
	resp, err := c.Call(Request{ID: fmt.Sprintf("session-create-%d", time.Now().UnixNano()), Method: "session.create"})
	if err != nil {
		return blobcore.Snapshot{}, err
	}
	if !resp.OK {
		return blobcore.Snapshot{}, errors.New(resp.Error)
	}
	var result SessionCreateResult
	if err := decodeResult(resp.Result, &result); err != nil {
		return blobcore.Snapshot{}, err
	}
	c.session = result.Session
	return result.State, nil
}

func (c *Client) CloseSession() error {
	if c.session == "" {
		return nil
	}
	resp, err := c.Call(Request{ID: fmt.Sprintf("session-close-%d", time.Now().UnixNano()), Method: "session.close"})
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	c.session = ""
	return nil
}

func (c *Client) GetState() (blobcore.Snapshot, error) {
	resp, err := c.Call(Request{ID: fmt.Sprintf("state-%d", time.Now().UnixNano()), Method: "state.get"})
	if err != nil {
		return blobcore.Snapshot{}, err
	}
	if !resp.OK {
		return blobcore.Snapshot{}, errors.New(resp.Error)
	}
	var snapshot blobcore.Snapshot
	if err := decodeResult(resp.Result, &snapshot); err != nil {
		return blobcore.Snapshot{}, err
	}
	return snapshot, nil
}

func (c *Client) InvokeAction(req blobcore.ActionRequest) (ActionInvokeResult, error) {
	params, err := json.Marshal(req)
	if err != nil {
		return ActionInvokeResult{}, fmt.Errorf("encode action request: %w", err)
	}
	resp, err := c.Call(Request{ID: fmt.Sprintf("action-%d", time.Now().UnixNano()), Method: "action.invoke", Params: params})
	if err != nil {
		return ActionInvokeResult{}, err
	}
	if !resp.OK {
		return ActionInvokeResult{}, errors.New(resp.Error)
	}
	var result ActionInvokeResult
	if err := decodeResult(resp.Result, &result); err != nil {
		return ActionInvokeResult{}, err
	}
	return result, nil
}

func decodeResult(result interface{}, target interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal rpc result: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode rpc result: %w", err)
	}
	return nil
}
