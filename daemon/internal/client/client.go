// Package client is a Go implementation of the controller side of the protocol,
// used by korokctl and by end-to-end tests.
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// PinnedTLS returns a TLS config that accepts exactly the certificate with
// fingerprint fp (hex SHA-256 of DER). If fp is empty, any certificate is
// accepted and its fingerprint is written to *seen (trust on first use).
func PinnedTLS(fp string, seen *string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // replaced by the pin check below
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("no certificate")
			}
			sum := sha256.Sum256(raw[0])
			got := hex.EncodeToString(sum[:])
			if seen != nil {
				*seen = got
			}
			if fp != "" && !strings.EqualFold(fp, got) {
				return fmt.Errorf("certificate fingerprint mismatch")
			}
			return nil
		},
	}
}

func httpClient(fp string, seen *string) *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: PinnedTLS(fp, seen), Proxy: nil},
	}
}

// Pair performs POST /v1/pair. It returns the response and the server fingerprint.
func Pair(ctx context.Context, host string, port int, fp, code string, dev proto.DeviceInfo) (*proto.PairResponse, string, error) {
	var seen string
	body, _ := json.Marshal(proto.PairRequest{Code: code, Device: dev})
	url := "https://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/v1/pair"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient(fp, &seen).Do(req)
	if err != nil {
		return nil, seen, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		var pe proto.Error
		_ = json.Unmarshal(b, &pe)
		return nil, seen, fmt.Errorf("pairing failed (%d): %s", resp.StatusCode, pe.Msg)
	}
	var pr proto.PairResponse
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil, seen, err
	}
	return &pr, seen, nil
}

// Client is an authenticated session.
type Client struct {
	ws      *websocket.Conn
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan proto.Envelope
	Events  chan proto.Envelope
	Hello   proto.HelloReply
	done    chan struct{}
	err     error
}

// Dial connects and authenticates.
func Dial(ctx context.Context, host string, port int, fp, deviceID, token string) (*Client, error) {
	url := "wss://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/v1/ws"
	ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPClient: httpClient(fp, nil), CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(1 << 20)
	c := &Client{ws: ws, pending: map[int64]chan proto.Envelope{}, Events: make(chan proto.Envelope, 64), done: make(chan struct{})}
	go c.readLoop()
	raw, err := c.Request(ctx, "hello", proto.Hello{V: proto.Version, Device: deviceID, Token: token, App: "korokctl"})
	if err != nil {
		c.Close()
		return nil, err
	}
	if err := json.Unmarshal(raw, &c.Hello); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) readLoop() {
	defer close(c.done)
	for {
		typ, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.err = err
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var env proto.Envelope
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.Re != nil {
			c.mu.Lock()
			ch := c.pending[*env.Re]
			delete(c.pending, *env.Re)
			c.mu.Unlock()
			if ch != nil {
				ch <- env
			}
			continue
		}
		select {
		case c.Events <- env:
		default:
		}
	}
}

// Request sends a message and waits for its reply.
func (c *Client) Request(ctx context.Context, t string, d any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan proto.Envelope, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	b, err := proto.Encode(t, &id, d)
	if err != nil {
		return nil, err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		return nil, err
	}
	select {
	case env := <-ch:
		if env.T == "err" {
			var pe proto.Error
			_ = json.Unmarshal(env.D, &pe)
			return nil, &pe
		}
		return env.D, nil
	case <-c.done:
		return nil, fmt.Errorf("connection closed: %v", c.err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Send sends a message without waiting.
func (c *Client) Send(ctx context.Context, t string, d any) error {
	b, err := proto.Encode(t, nil, d)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// Binary sends a binary frame.
func (c *Client) Binary(ctx context.Context, b []byte) error {
	return c.ws.Write(ctx, websocket.MessageBinary, b)
}

// Ping measures one round trip using ping/pong.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	if err := c.Send(ctx, "ping", proto.Ping{TS: start.UnixMilli()}); err != nil {
		return 0, err
	}
	for {
		select {
		case env := <-c.Events:
			if env.T == "pong" {
				return time.Since(start), nil
			}
		case <-c.done:
			return 0, fmt.Errorf("connection closed")
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// Close ends the session.
func (c *Client) Close() { _ = c.ws.Close(websocket.StatusNormalClosure, "") }
