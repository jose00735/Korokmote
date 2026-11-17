// Package server runs the TLS WebSocket endpoint and the pairing endpoint.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/jose00735/korokmote/daemon/internal/netinfo"
	"github.com/jose00735/korokmote/daemon/internal/pairing"
	"github.com/jose00735/korokmote/daemon/internal/platform"
	"github.com/jose00735/korokmote/daemon/internal/proto"
	"github.com/jose00735/korokmote/daemon/internal/router"
)

const (
	maxSessions     = 8
	maxMessage      = 64 << 10
	helloTimeout    = 3 * time.Second
	readTimeout     = 15 * time.Second
	writeTimeout    = 5 * time.Second
	ratePerSecond   = 300
	maxAuthFailures = 10
	blockFor        = 5 * time.Minute
)

// Caps lists what this daemon build can do.
var Caps = []string{"input", "text", "media", "audio", "power", "windows", "files", "launcher", "clipboard", "remotes"}

// slow messages run off the read loop so they never delay input.
var slow = map[string]bool{"remotes.act": true, "apps.list": true, "apps.launch": true, "fs.list": true, "fs.open": true, "fs.roots": true, "win.list": true, "win.focus": true, "audio.list": true, "clip.get": true, "clip.set": true}

// Server is the network front end.
type Server struct {
	Identity    *pairing.Identity
	Name        string
	Version     string
	AllowPublic bool
	Store       *pairing.Store
	Pairer      *pairing.Pairer
	Router      *router.Router
	Platform    *platform.Platform
	OnChange    func() // sessions changed
	OnConnect   func(deviceID, name string)

	port     int
	mu       sync.Mutex
	sessions map[uint64]*conn
	nextID   uint64
	fails    map[string]*failRec
	srv      *http.Server
}

type conn struct {
	sess   *router.Session
	ws     *websocket.Conn
	name   string
	addr   string
	since  time.Time
	cancel context.CancelFunc
}

type failRec struct {
	count   int
	start   time.Time
	blocked time.Time
}

// SessionInfo describes a live session for the UI.
type SessionInfo struct {
	DeviceID string    `json:"device_id"`
	Name     string    `json:"name"`
	Addr     string    `json:"addr"`
	Since    time.Time `json:"since"`
}

// Port returns the bound port (valid after Listen).
func (s *Server) Port() int { return s.port }

// Listen binds the TCP port, trying up to 10 consecutive ports.
func (s *Server) Listen(port int) (net.Listener, error) {
	var lastErr error
	for p := port; p < port+10; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			s.port = ln.Addr().(*net.TCPAddr).Port
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("listen: %w", lastErr)
}

// Serve runs the HTTPS server until ctx is done.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.sessions = map[uint64]*conn{}
	s.fails = map[string]*failRec{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", s.handleWS)
	mux.HandleFunc("/v1/pair", s.handlePair)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	s.srv = &http.Server{
		Handler:           mux,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{s.Identity.Certificate}, MinVersion: tls.VersionTLS13},
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          nil,
	}
	go func() {
		<-ctx.Done()
		s.CloseAll("shutdown")
		shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shCtx)
	}()
	err := s.srv.ServeTLS(ln, "", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func peerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

func (s *Server) peerAllowed(r *http.Request) bool {
	return s.AllowPublic || netinfo.IsLocalPeer(peerIP(r))
}

func (s *Server) blocked(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fails[ip]
	return f != nil && time.Now().Before(f.blocked)
}

func (s *Server) fail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fails[ip]
	now := time.Now()
	if f == nil || now.Sub(f.start) > time.Minute {
		f = &failRec{start: now}
		s.fails[ip] = f
	}
	f.count++
	if f.count >= maxAuthFailures {
		f.blocked = now.Add(blockFor)
		slog.Warn("blocking peer after repeated auth failures", "peer", ip)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	ip := peerIP(r).String()
	if !s.peerAllowed(r) || s.blocked(ip) {
		writeJSON(w, http.StatusForbidden, proto.Errorf(proto.ErrForbidden, "not allowed"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, proto.Errorf(proto.ErrBadRequest, "POST only"))
		return
	}
	var req proto.PairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, proto.Errorf(proto.ErrBadRequest, "bad body"))
		return
	}
	if req.Device.ID == "" || len(req.Device.ID) > 64 || len(req.Device.Name) > 64 {
		writeJSON(w, http.StatusBadRequest, proto.Errorf(proto.ErrBadRequest, "bad device"))
		return
	}
	if req.Device.Name == "" {
		req.Device.Name = "Phone"
	}
	tok, dev, err := s.Pairer.TryPair(req.Code, req.Device)
	if err != nil {
		s.fail(ip)
		slog.Warn("pairing rejected", "peer", ip, "reason", err)
		writeJSON(w, http.StatusForbidden, proto.Errorf(proto.ErrForbidden, "%v", err))
		return
	}
	slog.Info("device paired", "device", dev.Name, "id", dev.ID, "peer", ip)
	writeJSON(w, http.StatusOK, proto.PairResponse{
		Token:  tok,
		Daemon: proto.DaemonInfo{ID: s.Identity.ID, Name: s.Name, OS: s.Platform.OS, Version: s.Version},
		Phrase: proto.Phrase(s.Identity.Fingerprint, dev.ID),
	})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ip := peerIP(r).String()
	if !s.peerAllowed(r) || s.blocked(ip) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	full := len(s.sessions) >= maxSessions
	s.mu.Unlock()
	if full {
		http.Error(w, "too many sessions", http.StatusServiceUnavailable)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	ws.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dev, helloID, err := s.awaitHello(ctx, ws)
	if err != nil {
		s.fail(ip)
		slog.Warn("session rejected", "peer", ip, "reason", err)
		pe := proto.AsError(err)
		if helloID != nil {
			_ = s.write(ctx, ws, proto.Reply(*helloID, nil, pe))
		}
		_ = ws.Close(websocket.StatusPolicyViolation, pe.Code)
		return
	}

	s.mu.Lock()
	s.nextID++
	c := &conn{sess: router.NewSession(s.nextID, dev), ws: ws, name: dev.Name, addr: ip, since: time.Now(), cancel: cancel}
	s.sessions[c.sess.ID] = c
	s.mu.Unlock()
	s.changed()
	slog.Info("session open", "device", dev.Name, "peer", ip)
	if s.OnConnect != nil {
		go s.OnConnect(dev.ID, dev.Name)
	}

	reply := proto.HelloReply{
		V:      proto.Version,
		Daemon: proto.DaemonInfo{ID: s.Identity.ID, Name: s.Name, OS: s.Platform.OS, Version: s.Version},
		Caps:   Caps,
		Perms:  c.sess.Perms(),
		Screen: s.Platform.Session.Screen(),
		Locked: s.Platform.Session.Locked(),
	}
	if err := s.write(ctx, ws, proto.Reply(*helloID, reply, nil)); err == nil {
		s.loop(ctx, c)
	}

	s.mu.Lock()
	delete(s.sessions, c.sess.ID)
	s.mu.Unlock()
	s.Router.Release(c.sess)
	_ = ws.Close(websocket.StatusNormalClosure, "")
	s.changed()
	slog.Info("session closed", "device", dev.Name, "peer", ip)
}

func (s *Server) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

func (s *Server) awaitHello(ctx context.Context, ws *websocket.Conn) (*pairing.Device, *int64, error) {
	hctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	typ, data, err := ws.Read(hctx)
	if err != nil {
		return nil, nil, fmt.Errorf("no hello: %w", err)
	}
	if typ != websocket.MessageText {
		return nil, nil, proto.Errorf(proto.ErrUnauthorized, "hello required first")
	}
	var env proto.Envelope
	if err := json.Unmarshal(data, &env); err != nil || env.T != "hello" || env.ID == nil {
		return nil, nil, proto.Errorf(proto.ErrUnauthorized, "hello required first")
	}
	var h proto.Hello
	if err := json.Unmarshal(env.D, &h); err != nil {
		return nil, env.ID, proto.Errorf(proto.ErrBadRequest, "bad hello")
	}
	if h.V < 1 {
		return nil, env.ID, proto.Errorf(proto.ErrUnsupported, "protocol version %d not supported", h.V)
	}
	dev, ok := s.Store.Authenticate(h.Device, h.Token)
	if !ok {
		return nil, env.ID, proto.Errorf(proto.ErrUnauthorized, "unknown device or bad token")
	}
	return dev, env.ID, nil
}

func (s *Server) write(ctx context.Context, ws *websocket.Conn, b []byte) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return ws.Write(wctx, websocket.MessageText, b)
}

func (s *Server) loop(ctx context.Context, c *conn) {
	tokens := float64(ratePerSecond)
	last := time.Now()
	for {
		rctx, cancel := context.WithTimeout(ctx, readTimeout)
		typ, data, err := c.ws.Read(rctx)
		cancel()
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			f, ok, err := proto.DecodeFrame(data)
			if err == nil && ok {
				s.Router.Binary(c.sess, f)
			}
			continue
		}
		var env proto.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		if env.T == "ping" {
			_ = s.write(ctx, c.ws, proto.Event("pong", json.RawMessage(orEmpty(env.D))))
			continue
		}
		if !s.Router.Known(env.T) {
			if env.ID != nil {
				_ = s.write(ctx, c.ws, proto.Reply(*env.ID, nil, proto.Errorf(proto.ErrUnsupported, "unknown message %s", env.T)))
			}
			continue
		}
		now := time.Now()
		tokens = min(float64(ratePerSecond), tokens+now.Sub(last).Seconds()*ratePerSecond)
		last = now
		if tokens < 1 {
			if env.ID != nil {
				_ = s.write(ctx, c.ws, proto.Reply(*env.ID, nil, proto.Errorf(proto.ErrRateLimited, "slow down")))
			}
			continue
		}
		tokens--
		run := func() {
			res, err := s.Router.Handle(ctx, c.sess, env.T, env.D)
			if err != nil {
				slog.Debug("request failed", "t", env.T, "err", err)
			}
			if env.ID != nil {
				_ = s.write(ctx, c.ws, proto.Reply(*env.ID, res, err))
			}
		}
		if slow[env.T] {
			go run()
		} else {
			run()
		}
	}
}

func orEmpty(b json.RawMessage) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

// Broadcast sends an event to every session for which filter returns true (nil = all).
func (s *Server) Broadcast(t string, d any, filter func(*router.Session) bool) {
	b := proto.Event(t, d)
	s.mu.Lock()
	targets := make([]*conn, 0, len(s.sessions))
	for _, c := range s.sessions {
		if filter == nil || filter(c.sess) {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	for _, c := range targets {
		go func(c *conn) { _ = s.write(context.Background(), c.ws, b) }(c)
	}
}

// Sessions lists live sessions.
func (s *Server) Sessions() []SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionInfo, 0, len(s.sessions))
	for _, c := range s.sessions {
		out = append(out, SessionInfo{DeviceID: c.sess.DeviceID, Name: c.name, Addr: c.addr, Since: c.since})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

// Disconnect closes all sessions of a device.
func (s *Server) Disconnect(deviceID, reason string) {
	s.mu.Lock()
	var targets []*conn
	for _, c := range s.sessions {
		if c.sess.DeviceID == deviceID {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	for _, c := range targets {
		_ = s.write(context.Background(), c.ws, proto.Event("bye", proto.Bye{Reason: reason}))
		c.cancel()
	}
}

// UpdatePerms applies new permissions to live sessions of a device.
func (s *Server) UpdatePerms(deviceID string, perms []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.sessions {
		if c.sess.DeviceID == deviceID {
			c.sess.SetPerms(perms)
		}
	}
}

// CloseAll ends every session.
func (s *Server) CloseAll(reason string) {
	s.mu.Lock()
	targets := make([]*conn, 0, len(s.sessions))
	for _, c := range s.sessions {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		_ = s.write(context.Background(), c.ws, proto.Event("bye", proto.Bye{Reason: reason}))
		c.cancel()
	}
}
