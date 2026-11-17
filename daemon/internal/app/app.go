// Package app wires the daemon together.
package app

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/adminui"
	"github.com/jose00735/korokmote/daemon/internal/config"
	"github.com/jose00735/korokmote/daemon/internal/discovery"
	"github.com/jose00735/korokmote/daemon/internal/logring"
	"github.com/jose00735/korokmote/daemon/internal/netinfo"
	"github.com/jose00735/korokmote/daemon/internal/pairing"
	"github.com/jose00735/korokmote/daemon/internal/platform"
	"github.com/jose00735/korokmote/daemon/internal/proto"
	"github.com/jose00735/korokmote/daemon/internal/remotes"
	"github.com/jose00735/korokmote/daemon/internal/router"
	"github.com/jose00735/korokmote/daemon/internal/server"
	"github.com/jose00735/korokmote/daemon/internal/tray"
)

//go:embed icon.png
var iconPNG []byte

// Options are the command-line options.
type Options struct {
	ConfigDir string
	Console   bool
	Fake      bool
	Verbose   bool
	Pair      bool // open the pairing page on start
	Version   string
}

// Run starts the daemon and blocks until it quits.
func Run(opts Options) error {
	paths, err := config.ResolvePaths(opts.ConfigDir)
	if err != nil {
		return err
	}
	ring, closeLog := setupLogging(paths.LogDir, opts)
	defer closeLog()
	slog.Info("korokmote starting", "version", opts.Version, "config", paths.Dir)

	cfg, err := config.Load(paths.Config)
	if err != nil {
		slog.Error("config error, using defaults", "err", err)
		cfg = config.Default()
	}
	id, err := pairing.LoadIdentity(paths.Dir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	store, err := pairing.OpenStore(filepath.Join(paths.Dir, "devices.json"))
	if err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	pairer := pairing.NewPairer(store)

	var p *platform.Platform
	if opts.Fake {
		p, _ = platform.NewFake(opts.Verbose)
	} else {
		p = newPlatform(opts.Verbose)
	}
	files := platform.NewFiles(cfg.FileRoots, p.Shell)
	set := remotes.NewSet(paths.Remotes)
	rt := router.New(p, files, set, cfg.Launch)

	srv := &server.Server{
		Identity: id, Name: cfg.Name, Version: opts.Version, AllowPublic: cfg.AllowPublic,
		Store: store, Pairer: pairer, Router: rt, Platform: p,
	}
	ln, err := srv.Listen(cfg.Port)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := srv.Serve(ctx, ln); err != nil {
			slog.Error("server stopped", "err", err)
		}
	}()
	slog.Info("listening", "port", srv.Port(), "hosts", strings.Join(netinfo.Hosts(), ","), "fingerprint", id.Fingerprint[:16])

	adv := discovery.Start(discovery.Info{
		ID: id.ID, Name: cfg.Name, Port: srv.Port(), FPPrefix: id.Fingerprint[:8],
		Pairing: func() bool { return pairer.State().Open },
	})
	defer adv.Stop()

	stop := make(chan struct{})
	defer close(stop)
	go set.Watch(stop)
	set.OnReload(func() { srv.Broadcast("ev.remotes", struct{}{}, nil) })

	var trayRef *tray.Tray
	var trayMu sync.Mutex
	notify := func(title, text string) {
		trayMu.Lock()
		t := trayRef
		trayMu.Unlock()
		if t != nil {
			t.Notify(title, text)
		}
	}
	updateTip := func() {
		trayMu.Lock()
		t := trayRef
		trayMu.Unlock()
		if t == nil {
			return
		}
		n := len(srv.Sessions())
		tip := fmt.Sprintf("Korokmote :: %s :: %d connected", cfg.Name, n)
		if rt.Paused.Load() {
			tip += " :: PAUSED"
		}
		t.SetTooltip(tip)
	}
	srv.OnChange = updateTip
	lastNotice := map[string]string{}
	var noticeMu sync.Mutex
	srv.OnConnect = func(devID, name string) {
		day := time.Now().Format("2006-01-02")
		noticeMu.Lock()
		first := lastNotice[devID] != day
		lastNotice[devID] = day
		noticeMu.Unlock()
		if first {
			notify("Phone connected", name+" is controlling this PC.")
		}
	}
	pairer.OnChange(func(bool) { adv.Announce() })
	pairer.OnPaired(func(d pairing.Device) {
		noticeMu.Lock()
		lastNotice[d.ID] = time.Now().Format("2006-01-02")
		noticeMu.Unlock()
		notify("Device paired", fmt.Sprintf("%s :: %s", d.Name, proto.Phrase(id.Fingerprint, d.ID)))
	})

	exe, _ := os.Executable()
	ui, err := adminui.Start(adminui.Backend{
		Name: cfg.Name, Version: opts.Version, Identity: id, Port: srv.Port, Store: store,
		Pairer: pairer, PairDuration: cfg.PairingDuration(),
		Online: func() map[string]string {
			m := map[string]string{}
			for _, s := range srv.Sessions() {
				m[s.DeviceID] = s.Addr
			}
			return m
		},
		Revoke: func(devID string) error {
			srv.Disconnect(devID, "revoked")
			return store.Revoke(devID)
		},
		SetPerms: func(devID string, perms []string) error {
			if err := store.SetPerms(devID, perms); err != nil {
				return err
			}
			if d, ok := store.Get(devID); ok {
				srv.UpdatePerms(devID, d.Perms)
			}
			return nil
		},
		Paused:       rt.Paused.Load,
		SetPaused:    func(v bool) { rt.Paused.Store(v); updateTip() },
		Autostart:    autostartEnabled,
		SetAutostart: func(v bool) error { return setAutostart(v, exe) },
		Logs:         ring.Lines,
	})
	if err != nil {
		return err
	}
	openPage := func(view string) {
		if view == "pair" {
			pairer.Open(cfg.PairingDuration())
		}
		if err := openURL(ui.URL(view)); err != nil {
			slog.Error("open admin page", "err", err)
		}
	}
	if opts.Console {
		slog.Info("admin page", "url", ui.URL(""))
	}

	go watch(ctx, p, rt, srv, set)

	// On the first run for this user, honour the installer's choice (or config) for autostart.
	if !opts.Fake && exe != "" && firstRun(paths.Dir) {
		want := cfg.Autostart == "user"
		if choice := installerAutostart(); choice != "" {
			want = choice == "user"
		}
		if want && !autostartEnabled() {
			_ = setAutostart(true, exe)
		}
	}

	return tray.Run(tray.Config{
		Tooltip: "Korokmote :: " + cfg.Name,
		IconPNG: iconPNG,
		OnLeft:  func() { openPage("") },
		OnReady: func(t *tray.Tray) {
			trayMu.Lock()
			trayRef = t
			trayMu.Unlock()
			updateTip()
			if opts.Pair {
				openPage("pair")
			}
		},
		Menu: func() []tray.Item {
			n := len(srv.Sessions())
			return []tray.Item{
				{Title: fmt.Sprintf("%s :: %d connected :: port %d", cfg.Name, n, srv.Port()), Disabled: true, OnClick: func() {}},
				tray.Separator,
				{Title: "Pair a device…", OnClick: func() { openPage("pair") }},
				{Title: "Devices…", OnClick: func() { openPage("") }},
				{Title: "Open remotes folder", OnClick: func() { _ = openURL(paths.Remotes) }},
				tray.Separator,
				{Title: "Start with Windows", Checked: autostartEnabled(), OnClick: func() { _ = setAutostart(!autostartEnabled(), exe) }},
				{Title: "Pause input", Checked: rt.Paused.Load(), OnClick: func() { rt.Paused.Store(!rt.Paused.Load()); updateTip() }},
				tray.Separator,
				{Title: "Quit", OnClick: func() {
					trayMu.Lock()
					t := trayRef
					trayMu.Unlock()
					cancel()
					if t != nil {
						t.Quit()
					}
				}},
			}
		},
	})
}

// firstRun returns true once per config directory.
func firstRun(dir string) bool {
	marker := filepath.Join(dir, ".initialised")
	if _, err := os.Stat(marker); err == nil {
		return false
	}
	_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o600)
	return true
}

func setupLogging(dir string, opts Options) (*logring.Ring, func()) {
	path := filepath.Join(dir, "korokmoted.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 5<<20 {
		_ = os.Rename(path, path+".1")
	}
	var w io.Writer = io.Discard
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		w = f
	}
	if opts.Console {
		w = io.MultiWriter(w, os.Stderr)
	}
	level := slog.LevelInfo
	if opts.Verbose {
		level = slog.LevelDebug
	}
	ring := logring.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}), 200)
	slog.SetDefault(slog.New(ring))
	return ring, func() {
		if f != nil {
			_ = f.Close()
		}
	}
}

// watch polls session lock, foreground window, clipboard and volume, and pushes events.
func watch(ctx context.Context, p *platform.Platform, rt *router.Router, srv *server.Server, set *remotes.Set) {
	t := time.NewTicker(700 * time.Millisecond)
	defer t.Stop()
	locked := p.Session.Locked()
	fg, _ := p.Windows.Foreground()
	clipSeq := p.Clipboard.Sequence()
	audio := rt.AudioState("default")
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		tick++
		if len(srv.Sessions()) == 0 {
			continue
		}
		if l := p.Session.Locked(); l != locked {
			locked = l
			srv.Broadcast("ev.lock", proto.LockEvent{Locked: l}, nil)
		}
		if w, err := p.Windows.Foreground(); err == nil && w.Exe != "" && (w.Exe != fg.Exe || w.Title != fg.Title) {
			fg = w
			srv.Broadcast("ev.fg", proto.ForegroundEvent{Title: w.Title, Exe: w.Exe, Remote: set.MatchExe(w.Exe)},
				func(s *router.Session) bool { return s.Has(pairing.PermLauncher) || s.Has(pairing.PermInput) })
		}
		if seq := p.Clipboard.Sequence(); seq != clipSeq {
			clipSeq = seq
			if txt, err := p.Clipboard.Get(); err == nil && txt != "" && len(txt) < 64<<10 {
				srv.Broadcast("ev.clip", proto.ClipText{Text: txt},
					func(s *router.Session) bool { return s.ClipSub.Load() && s.Has(pairing.PermClipboard) })
			}
		}
		if tick%2 == 0 {
			if a := rt.AudioState("default"); a != audio {
				audio = a
				srv.Broadcast("ev.audio", a, func(s *router.Session) bool { return s.Has(pairing.PermAudio) })
			}
		}
	}
}
