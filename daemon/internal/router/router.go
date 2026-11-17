// Package router dispatches authenticated protocol messages to the platform.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/pairing"
	"github.com/jose00735/korokmote/daemon/internal/platform"
	"github.com/jose00735/korokmote/daemon/internal/proto"
	"github.com/jose00735/korokmote/daemon/internal/remotes"
)

// Session is the per-connection state the router needs.
type Session struct {
	ID       uint64
	DeviceID string
	perms    atomic.Pointer[[]string]
	ClipSub  atomic.Bool

	heldMu  sync.Mutex
	keys    map[uint16]bool
	buttons map[platform.Button]bool
	actMu   sync.Mutex
}

// NewSession creates session state for an authenticated device.
func NewSession(id uint64, dev *pairing.Device) *Session {
	s := &Session{ID: id, DeviceID: dev.ID, keys: map[uint16]bool{}, buttons: map[platform.Button]bool{}}
	s.SetPerms(dev.Perms)
	return s
}

// SetPerms replaces the session's permissions.
func (s *Session) SetPerms(p []string) {
	cp := slices.Clone(p)
	s.perms.Store(&cp)
}

// Perms returns the current permissions.
func (s *Session) Perms() []string { return slices.Clone(*s.perms.Load()) }

// Has reports whether the session holds perm.
func (s *Session) Has(perm string) bool { return slices.Contains(*s.perms.Load(), perm) }

// Router handles messages.
type Router struct {
	P       *platform.Platform
	Worker  *Worker
	Files   *platform.Files
	Remotes *remotes.Set
	Launch  map[string]string // allow-listed launch targets from config
	Paused  atomic.Bool

	handlers map[string]handler
}

type handler struct {
	perm string
	fn   func(ctx context.Context, s *Session, d json.RawMessage) (any, error)
}

// New builds a router.
func New(p *platform.Platform, files *platform.Files, set *remotes.Set, launch map[string]string) *Router {
	r := &Router{P: p, Worker: NewWorker(p.Input), Files: files, Remotes: set, Launch: launch}
	r.handlers = map[string]handler{
		"key":          {pairing.PermInput, r.key},
		"combo":        {pairing.PermInput, r.combo},
		"text":         {pairing.PermInput, r.text},
		"click":        {pairing.PermInput, r.click},
		"media":        {pairing.PermMedia, r.media},
		"audio.list":   {pairing.PermAudio, r.audioList},
		"audio.set":    {pairing.PermAudio, r.audioSet},
		"audio.mute":   {pairing.PermAudio, r.audioMute},
		"power":        {pairing.PermPower, r.power},
		"win.list":     {pairing.PermLauncher, r.winList},
		"win.focus":    {pairing.PermLauncher, r.winFocus},
		"win.close":    {pairing.PermLauncher, r.winClose},
		"apps.list":    {pairing.PermLauncher, r.appsList},
		"apps.launch":  {pairing.PermLauncher, r.appsLaunch},
		"fs.roots":     {pairing.PermFiles, r.fsRoots},
		"fs.list":      {pairing.PermFiles, r.fsList},
		"fs.open":      {pairing.PermFiles, r.fsOpen},
		"clip.get":     {pairing.PermClipboard, r.clipGet},
		"clip.set":     {pairing.PermClipboard, r.clipSet},
		"clip.sub":     {pairing.PermClipboard, r.clipSub},
		"remotes.list": {"", r.remotesList},
		"remotes.get":  {"", r.remotesGet},
		"remotes.act":  {"", r.remotesAct},
	}
	return r
}

// Known reports whether t is a handled message type.
func (r *Router) Known(t string) bool { _, ok := r.handlers[t]; return ok }

// Handle executes one control message.
func (r *Router) Handle(ctx context.Context, s *Session, t string, d json.RawMessage) (any, error) {
	h, ok := r.handlers[t]
	if !ok {
		return nil, proto.Errorf(proto.ErrUnsupported, "unknown message %s", t)
	}
	if h.perm != "" && !s.Has(h.perm) {
		return nil, proto.Errorf(proto.ErrForbidden, "this device lacks the %s permission", h.perm)
	}
	if r.Paused.Load() && (h.perm == pairing.PermInput || h.perm == pairing.PermMedia || h.perm == pairing.PermPower || t == "remotes.act") {
		return nil, proto.Errorf(proto.ErrForbidden, "input is paused on the PC")
	}
	return h.fn(ctx, s, d)
}

// Binary handles a binary input frame.
func (r *Router) Binary(s *Session, f proto.Frame) {
	if !s.Has(pairing.PermInput) || r.Paused.Load() {
		return
	}
	switch f.Op {
	case proto.OpMove:
		r.Worker.Move(f.X, f.Y)
	case proto.OpScroll:
		r.Worker.Scroll(f.X, f.Y)
	case proto.OpAbs:
		x, y := uint16(f.X), uint16(f.Y)
		r.Worker.Do(func() error { return r.P.Input.MoveAbs(x, y) })
	case proto.OpButton:
		r.button(s, platform.Button(f.Button), f.Down)
	}
}

// Release frees every key and button the session still holds.
func (r *Router) Release(s *Session) {
	s.heldMu.Lock()
	keys := make([]uint16, 0, len(s.keys))
	for k := range s.keys {
		keys = append(keys, k)
	}
	buttons := make([]platform.Button, 0, len(s.buttons))
	for b := range s.buttons {
		buttons = append(buttons, b)
	}
	s.keys = map[uint16]bool{}
	s.buttons = map[platform.Button]bool{}
	s.heldMu.Unlock()
	for _, b := range buttons {
		b := b
		r.Worker.Do(func() error { return r.P.Input.Button(b, false) })
	}
	for _, k := range keys {
		k := k
		r.Worker.Do(func() error { return r.P.Input.Key(k, false) })
	}
}

func (r *Router) button(s *Session, b platform.Button, down bool) {
	if b < platform.ButtonLeft || b > platform.ButtonForward {
		return
	}
	s.heldMu.Lock()
	if down {
		s.buttons[b] = true
	} else {
		delete(s.buttons, b)
	}
	s.heldMu.Unlock()
	r.Worker.Do(func() error { return r.P.Input.Button(b, down) })
}

func (r *Router) keyEvent(s *Session, vk uint16, down bool) {
	s.heldMu.Lock()
	if down {
		s.keys[vk] = true
	} else {
		delete(s.keys, vk)
	}
	s.heldMu.Unlock()
	r.Worker.Do(func() error { return r.P.Input.Key(vk, down) })
}

func decode[T any](d json.RawMessage) (T, error) {
	var v T
	if len(d) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(d, &v); err != nil {
		return v, proto.Errorf(proto.ErrBadRequest, "bad payload: %v", err)
	}
	return v, nil
}

// ---- input ----

func (r *Router) tapKey(s *Session, name string) error {
	vk, ok := proto.KeyCode(name)
	if !ok {
		return proto.Errorf(proto.ErrBadRequest, "unknown key %q", name)
	}
	r.keyEvent(s, vk, true)
	r.keyEvent(s, vk, false)
	return nil
}

func (r *Router) key(_ context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Key](d)
	if err != nil {
		return nil, err
	}
	vk, ok := proto.KeyCode(m.K)
	if !ok {
		return nil, proto.Errorf(proto.ErrBadRequest, "unknown key %q", m.K)
	}
	switch m.A {
	case "down":
		r.keyEvent(s, vk, true)
	case "up":
		r.keyEvent(s, vk, false)
	case "", "tap":
		r.keyEvent(s, vk, true)
		r.keyEvent(s, vk, false)
	default:
		return nil, proto.Errorf(proto.ErrBadRequest, "bad key action %q", m.A)
	}
	return nil, nil
}

func (r *Router) pressCombo(s *Session, names []string) error {
	if len(names) == 0 || len(names) > 6 {
		return proto.Errorf(proto.ErrBadRequest, "combo needs 1-6 keys")
	}
	vks := make([]uint16, len(names))
	for i, n := range names {
		vk, ok := proto.KeyCode(n)
		if !ok {
			return proto.Errorf(proto.ErrBadRequest, "unknown key %q", n)
		}
		vks[i] = vk
	}
	for _, vk := range vks {
		r.keyEvent(s, vk, true)
	}
	for i := len(vks) - 1; i >= 0; i-- {
		r.keyEvent(s, vks[i], false)
	}
	return nil
}

func (r *Router) combo(_ context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Combo](d)
	if err != nil {
		return nil, err
	}
	return nil, r.pressCombo(s, m.Keys)
}

func (r *Router) text(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Text](d)
	if err != nil {
		return nil, err
	}
	if len(m.S) > 8192 {
		return nil, proto.Errorf(proto.ErrBadRequest, "text too long")
	}
	s := m.S
	r.Worker.Do(func() error { return r.P.Input.Text(s) })
	return nil, nil
}

func (r *Router) click(_ context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Click](d)
	if err != nil {
		return nil, err
	}
	if m.B == 0 {
		m.B = 1
	}
	if m.N <= 0 {
		m.N = 1
	}
	if m.N > 3 {
		m.N = 3
	}
	for i := 0; i < m.N; i++ {
		r.button(s, platform.Button(m.B), true)
		r.button(s, platform.Button(m.B), false)
	}
	return nil, nil
}

func (r *Router) media(_ context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Media](d)
	if err != nil {
		return nil, err
	}
	name, ok := proto.MediaKeys[m.A]
	if !ok {
		return nil, proto.Errorf(proto.ErrBadRequest, "unknown media action %q", m.A)
	}
	return nil, r.tapKey(s, name)
}

// ---- audio ----

func (r *Router) audioList(context.Context, *Session, json.RawMessage) (any, error) {
	devs, err := r.P.Audio.Devices()
	if err != nil {
		return nil, err
	}
	if devs == nil {
		devs = []proto.AudioDevice{}
	}
	return map[string]any{"devices": devs}, nil
}

// AudioState returns the level of a device for replies and events.
func (r *Router) AudioState(dev string) proto.AudioEvent {
	if dev == "" {
		dev = "default"
	}
	l, m, _ := r.P.Audio.Volume(dev)
	return proto.AudioEvent{Device: dev, Level: l, Muted: m}
}

func (r *Router) setVolume(dev string, value float64, relative bool) error {
	if dev == "" {
		dev = "default"
	}
	if relative {
		cur, _, err := r.P.Audio.Volume(dev)
		if err != nil {
			return err
		}
		value += cur
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return r.P.Audio.SetVolume(dev, value)
}

func (r *Router) audioSet(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.AudioSet](d)
	if err != nil {
		return nil, err
	}
	switch {
	case m.Level != nil:
		err = r.setVolume(m.Device, *m.Level, false)
	case m.Delta != nil:
		err = r.setVolume(m.Device, *m.Delta, true)
	default:
		return nil, proto.Errorf(proto.ErrBadRequest, "level or delta required")
	}
	if err != nil {
		return nil, err
	}
	return r.AudioState(m.Device), nil
}

func (r *Router) audioMute(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.AudioMute](d)
	if err != nil {
		return nil, err
	}
	dev := m.Device
	if dev == "" {
		dev = "default"
	}
	muted := false
	if m.Muted != nil {
		muted = *m.Muted
	} else {
		_, cur, err := r.P.Audio.Volume(dev)
		if err != nil {
			return nil, err
		}
		muted = !cur
	}
	if err := r.P.Audio.SetMute(dev, muted); err != nil {
		return nil, err
	}
	return r.AudioState(dev), nil
}

// ---- power ----

func (r *Router) doPower(a string, delay int) error {
	if delay < 0 || delay > 3600 {
		return proto.Errorf(proto.ErrBadRequest, "delay out of range")
	}
	dd := time.Duration(delay) * time.Second
	switch a {
	case "lock":
		return r.P.Power.Lock()
	case "sleep":
		return r.P.Power.Sleep()
	case "hibernate":
		return r.P.Power.Hibernate()
	case "restart":
		return r.P.Power.Restart(dd)
	case "shutdown":
		return r.P.Power.Shutdown(dd)
	case "abort":
		return r.P.Power.Abort()
	case "monitor_off":
		return r.P.Power.MonitorOff()
	}
	return proto.Errorf(proto.ErrBadRequest, "unknown power action %q", a)
}

func (r *Router) power(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.Power](d)
	if err != nil {
		return nil, err
	}
	return nil, r.doPower(m.A, m.Delay)
}

// ---- windows & apps ----

func (r *Router) winList(context.Context, *Session, json.RawMessage) (any, error) {
	ws, err := r.P.Windows.List()
	if err != nil {
		return nil, err
	}
	if ws == nil {
		ws = []proto.WindowInfo{}
	}
	return map[string]any{"windows": ws}, nil
}

func (r *Router) winFocus(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.WinRef](d)
	if err != nil {
		return nil, err
	}
	return nil, r.P.Windows.Focus(m.H)
}

func (r *Router) winClose(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.WinRef](d)
	if err != nil {
		return nil, err
	}
	return nil, r.P.Windows.Close(m.H)
}

func (r *Router) apps() ([]platform.App, error) {
	apps, err := r.P.Shell.Apps()
	if err != nil {
		return nil, err
	}
	for id, target := range r.Launch {
		apps = append(apps, platform.App{ID: "cfg:" + id, Name: id, Path: target})
	}
	return apps, nil
}

func (r *Router) appsList(context.Context, *Session, json.RawMessage) (any, error) {
	apps, err := r.apps()
	if err != nil {
		return nil, err
	}
	out := make([]proto.AppInfo, 0, len(apps))
	for _, a := range apps {
		out = append(out, proto.AppInfo{ID: a.ID, Name: a.Name})
	}
	return map[string]any{"apps": out}, nil
}

// launch resolves an app by id, config alias, or (case-insensitive) name.
func (r *Router) launch(id string) error {
	if target, ok := r.Launch[id]; ok {
		return r.P.Shell.Open(target)
	}
	apps, err := r.apps()
	if err != nil {
		return err
	}
	low := strings.ToLower(id)
	for _, a := range apps {
		if a.ID == id {
			return r.P.Shell.Open(a.Path)
		}
	}
	for _, a := range apps {
		if strings.ToLower(a.Name) == low {
			return r.P.Shell.Open(a.Path)
		}
	}
	for _, a := range apps {
		if strings.HasPrefix(strings.ToLower(a.Name), low) {
			return r.P.Shell.Open(a.Path)
		}
	}
	return proto.Errorf(proto.ErrNotFound, "app %q not found", id)
}

func (r *Router) appsLaunch(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.AppRef](d)
	if err != nil {
		return nil, err
	}
	return nil, r.launch(m.ID)
}

// ---- files ----

func (r *Router) fsRoots(context.Context, *Session, json.RawMessage) (any, error) {
	return map[string]any{"entries": r.Files.Roots()}, nil
}

func (r *Router) fsList(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.FsPath](d)
	if err != nil {
		return nil, err
	}
	es, err := r.Files.List(m.Path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"entries": es}, nil
}

func (r *Router) fsOpen(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.FsPath](d)
	if err != nil {
		return nil, err
	}
	return nil, r.Files.Open(r.P.Shell, m.Path)
}

// ---- clipboard ----

func (r *Router) clipGet(context.Context, *Session, json.RawMessage) (any, error) {
	t, err := r.P.Clipboard.Get()
	if err != nil {
		return nil, err
	}
	return proto.ClipText{Text: t}, nil
}

func (r *Router) clipSet(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.ClipText](d)
	if err != nil {
		return nil, err
	}
	if len(m.Text) > 1<<20 {
		return nil, proto.Errorf(proto.ErrBadRequest, "text too long")
	}
	return nil, r.P.Clipboard.Set(m.Text)
}

func (r *Router) clipSub(_ context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.ClipSub](d)
	if err != nil {
		return nil, err
	}
	s.ClipSub.Store(m.On)
	return nil, nil
}

// ---- remotes ----

func (r *Router) remotesList(context.Context, *Session, json.RawMessage) (any, error) {
	return map[string]any{"remotes": r.Remotes.List()}, nil
}

func (r *Router) remotesGet(_ context.Context, _ *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.RemoteRef](d)
	if err != nil {
		return nil, err
	}
	rem, ok := r.Remotes.Get(m.ID)
	if !ok {
		return nil, proto.Errorf(proto.ErrNotFound, "remote %q not found", m.ID)
	}
	return rem.Document(), nil
}

func (r *Router) remotesAct(ctx context.Context, s *Session, d json.RawMessage) (any, error) {
	m, err := decode[proto.RemoteAct](d)
	if err != nil {
		return nil, err
	}
	rem, ok := r.Remotes.Get(m.Remote)
	if !ok {
		return nil, proto.Errorf(proto.ErrNotFound, "remote %q not found", m.Remote)
	}
	s.actMu.Lock()
	defer s.actMu.Unlock()
	if err := rem.Run(ctx, m.Action, m.Args, &exec{r: r, s: s}); err != nil {
		return nil, err
	}
	return nil, nil
}

// exec implements remotes.Executor with the session's permissions.
type exec struct {
	r *Router
	s *Session
}

func (e *exec) need(p string) error {
	if !e.s.Has(p) {
		return proto.Errorf(proto.ErrForbidden, "this device lacks the %s permission", p)
	}
	return nil
}

func (e *exec) Key(name string) error {
	if err := e.need(pairing.PermInput); err != nil {
		return err
	}
	return e.r.tapKey(e.s, name)
}

func (e *exec) Combo(keys []string) error {
	if err := e.need(pairing.PermInput); err != nil {
		return err
	}
	return e.r.pressCombo(e.s, keys)
}

func (e *exec) Text(s string) error {
	if err := e.need(pairing.PermInput); err != nil {
		return err
	}
	return e.r.Worker.DoWait(func() error { return e.r.P.Input.Text(s) })
}

func (e *exec) Click(b int) error {
	if err := e.need(pairing.PermInput); err != nil {
		return err
	}
	e.r.button(e.s, platform.Button(b), true)
	e.r.button(e.s, platform.Button(b), false)
	return nil
}

func (e *exec) Media(a string) error {
	if err := e.need(pairing.PermMedia); err != nil {
		return err
	}
	return e.r.tapKey(e.s, proto.MediaKeys[a])
}

func (e *exec) Volume(v float64, rel bool) error {
	if err := e.need(pairing.PermAudio); err != nil {
		return err
	}
	return e.r.setVolume("default", v, rel)
}

func (e *exec) Mute(mode string) error {
	if err := e.need(pairing.PermAudio); err != nil {
		return err
	}
	_, cur, err := e.r.P.Audio.Volume("default")
	if err != nil {
		return err
	}
	target := !cur
	switch mode {
	case "on", "true":
		target = true
	case "off", "false":
		target = false
	}
	return e.r.P.Audio.SetMute("default", target)
}

func (e *exec) Power(a string) error {
	if err := e.need(pairing.PermPower); err != nil {
		return err
	}
	return e.r.doPower(a, 0)
}

func (e *exec) Launch(id string) error {
	if err := e.need(pairing.PermLauncher); err != nil {
		return err
	}
	return e.r.launch(id)
}

func (e *exec) Open(target string) error {
	if err := e.need(pairing.PermLauncher); err != nil {
		return err
	}
	low := strings.ToLower(target)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		// Local paths must stay inside the file roots.
		if err := e.need(pairing.PermFiles); err != nil {
			return err
		}
		return e.r.Files.Open(e.r.P.Shell, target)
	}
	return e.r.P.Shell.Open(target)
}

func (e *exec) Focus(exe, title string) error {
	if err := e.need(pairing.PermInput); err != nil {
		return err
	}
	ws, err := e.r.P.Windows.List()
	if err != nil {
		return err
	}
	for _, w := range ws {
		if (exe != "" && strings.EqualFold(w.Exe, exe)) || (title != "" && strings.Contains(strings.ToLower(w.Title), strings.ToLower(title))) {
			return e.r.P.Windows.Focus(w.H)
		}
	}
	return fmt.Errorf("no window matches %s%s", exe, title)
}
