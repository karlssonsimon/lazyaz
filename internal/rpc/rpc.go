package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type Request struct {
	ID      string          `json:"id"`
	Session string          `json:"session,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     string      `json:"id"`
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type Handler func(Request) Response

type Server struct {
	listener net.Listener
	path     string
	handler  Handler
}

func ListenUnix(socketPath string, handler Handler) (*Server, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("handler is required")
	}
	if err := os.RemoveAll(trimmed); err != nil {
		return nil, fmt.Errorf("remove existing socket %s: %w", trimmed, err)
	}
	listener, err := net.Listen("unix", trimmed)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", trimmed, err)
	}
	return &Server{listener: listener, path: trimmed, handler: handler}, nil
}

func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(bufio.NewReader(conn))
	encoder := json.NewEncoder(conn)
	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			return
		}
		if err := encoder.Encode(s.handler(req)); err != nil {
			return
		}
	}
}

func (s *Server) Close() error {
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	if s.path != "" {
		if removeErr := os.RemoveAll(s.path); removeErr != nil && err == nil {
			err = removeErr
		}
	}
	return err
}

type Client struct {
	conn    net.Conn
	decoder *json.Decoder
	encoder *json.Encoder
}

func DialUnix(socketPath string) (*Client, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	conn, err := net.DialTimeout("unix", trimmed, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", trimmed, err)
	}
	return &Client{
		conn:    conn,
		decoder: json.NewDecoder(bufio.NewReader(conn)),
		encoder: json.NewEncoder(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Call(req Request) (Response, error) {
	if err := c.encoder.Encode(req); err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	var resp Response
	if err := c.decoder.Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}
