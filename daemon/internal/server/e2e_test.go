package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/client"
	"github.com/jose00735/korokmote/daemon/internal/pairing"
	"github.com/jose00735/korokmote/daemon/internal/platform"
	"github.com/jose00735/korokmote/daemon/internal/proto"
	"github.com/jose00735/korokmote/daemon/internal/remotes"
	"github.com/jose00735/korokmote/daemon/internal/router"
	"github.com/jose00735/korokmote/daemon/internal/server"
)

type env struct {
	srv    *server.Server
	fake   *platform.Fake
	pairer *pairing.Pairer
	store  *pairing.Store
	id     *pairing.Identity
	port   int
	root   string
}

func setup(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	id, err := pairing.LoadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := pairing.OpenStore(filepath.Join(dir, "devices.json"))
	pairer := pairing.NewPairer(store)
	p, fake := platform.NewFake(false)
	root := filepath.Join(dir, "files")
	_ = os.MkdirAll(filepath.Join(root, "Videos"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "movie.mkv"), []byte("x"), 0o644)
	rt := router.New(p, platform.NewFiles([]string{root}, p.Shell), remotes.NewSet(""), nil)
	srv := &server.Server{Identity: id, Name: "TEST-PC", Version: "test", Store: store, Pairer: pairer, Router: rt, Platform: p}
	ln, err := srv.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()
	return &env{srv: srv, fake: fake, pairer: pairer, store: store, id: id, port: srv.Port(), root: root}
}

func waitFor(t *testing.T, f *platform.Fake, substr string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(f.Find(substr)) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d calls containing %q, got %v", n, substr, f.Calls())
}

func TestEndToEnd(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dev := proto.DeviceInfo{ID: "phone-1", Name: "Pixel", Platform: "android", App: "1.0"}

	// No window open.
	if _, _, err := client.Pair(ctx, "127.0.0.1", e.port, e.id.Fingerprint, "nope", dev); err == nil {
		t.Fatal("pairing without a window must fail")
	}
	// Wrong pin must fail before anything is sent.
	st := e.pairer.Open(time.Minute)
	if _, _, err := client.Pair(ctx, "127.0.0.1", e.port, strings.Repeat("0", 64), st.Code, dev); err == nil {
		t.Fatal("pin mismatch must fail")
	}
	resp, seen, err := client.Pair(ctx, "127.0.0.1", e.port, e.id.Fingerprint, st.Code, dev)
	if err != nil {
		t.Fatal(err)
	}
	if seen != e.id.Fingerprint || resp.Phrase != proto.Phrase(e.id.Fingerprint, dev.ID) {
		t.Fatal("fingerprint or phrase mismatch")
	}
	if e.pairer.State().Open {
		t.Fatal("window must close after a successful pairing")
	}
	// Code is single use.
	if _, _, err := client.Pair(ctx, "127.0.0.1", e.port, e.id.Fingerprint, st.Code, dev); err == nil {
		t.Fatal("code reuse must fail")
	}

	// Bad token.
	if _, err := client.Dial(ctx, "127.0.0.1", e.port, e.id.Fingerprint, dev.ID, "bad"); err == nil {
		t.Fatal("bad token must fail")
	}
	c, err := client.Dial(ctx, "127.0.0.1", e.port, e.id.Fingerprint, dev.ID, resp.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Hello.Daemon.Name != "TEST-PC" || len(c.Hello.Caps) == 0 || c.Hello.Screen.W != 1920 {
		t.Fatalf("hello: %+v", c.Hello)
	}

	// Binary motion.
	for i := 0; i < 3; i++ {
		_ = c.Binary(ctx, proto.EncodeRel(proto.OpMove, 10, -5, byte(i)))
	}
	_ = c.Binary(ctx, proto.EncodeButton(1, true))
	_ = c.Binary(ctx, proto.EncodeButton(1, false))
	if _, err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, e.fake, "button 1 false", 1)
	var dx int
	for _, call := range e.fake.Find("move ") {
		var x, y int
		_, _ = fmtSscanf(call, &x, &y)
		dx += x
	}
	if dx != 30 {
		t.Errorf("total dx = %d, calls %v", dx, e.fake.Calls())
	}

	must := func(t string, d any) json.RawMessage {
		res, err := c.Request(ctx, t, d)
		if err != nil {
			panic(t + ": " + err.Error())
		}
		return res
	}
	must("key", proto.Key{K: "enter", A: "tap"})
	waitFor(t, e.fake, "key 0x0D false", 1)
	must("combo", proto.Combo{Keys: []string{"CTRL", "SHIFT", "ESC"}})
	waitFor(t, e.fake, "key 0x11 false", 1)
	must("text", proto.Text{S: "héllo"})
	waitFor(t, e.fake, "text 5 runes", 1)
	must("media", proto.Media{A: "playpause"})
	waitFor(t, e.fake, "key 0xB3 true", 1)
	d := -0.1
	var a proto.AudioEvent
	_ = json.Unmarshal(must("audio.set", proto.AudioSet{Delta: &d}), &a)
	if a.Level < 0.39 || a.Level > 0.41 {
		t.Errorf("volume after delta: %+v", a)
	}
	must("power", proto.Power{A: "lock"})
	waitFor(t, e.fake, "power lock", 1)

	// Files: inside roots ok, outside forbidden.
	var fl struct{ Entries []proto.FsEntry }
	_ = json.Unmarshal(must("fs.list", proto.FsPath{Path: e.root}), &fl)
	if len(fl.Entries) != 2 || !fl.Entries[0].Dir {
		t.Errorf("fs.list: %+v", fl.Entries)
	}
	if _, err := c.Request(ctx, "fs.list", proto.FsPath{Path: "/etc"}); !isCode(err, proto.ErrForbidden) {
		t.Errorf("outside root: %v", err)
	}
	if _, err := c.Request(ctx, "fs.list", proto.FsPath{Path: filepath.Join(e.root, "..", "..")}); !isCode(err, proto.ErrForbidden) {
		t.Errorf("traversal: %v", err)
	}

	// Remotes.
	var rl struct{ Remotes []proto.RemoteSummary }
	_ = json.Unmarshal(must("remotes.list", nil), &rl)
	if len(rl.Remotes) < 9 {
		t.Errorf("remotes: %d", len(rl.Remotes))
	}
	var doc map[string]any
	_ = json.Unmarshal(must("remotes.get", proto.RemoteRef{ID: "vlc"}), &doc)
	if doc["layout"] == nil {
		t.Error("remote layout missing")
	}
	e.fake.Reset()
	must("remotes.act", proto.RemoteAct{Remote: "vlc", Action: "toggle"})
	waitFor(t, e.fake, "key 0x20 false", 1)

	// Unknown message.
	if _, err := c.Request(ctx, "nope", nil); !isCode(err, proto.ErrUnsupported) {
		t.Errorf("unknown: %v", err)
	}

	// Permissions are enforced live.
	_ = e.store.SetPerms(dev.ID, []string{pairing.PermMedia})
	e.srv.UpdatePerms(dev.ID, []string{pairing.PermMedia})
	if _, err := c.Request(ctx, "key", proto.Key{K: "A"}); !isCode(err, proto.ErrForbidden) {
		t.Errorf("perm: %v", err)
	}
	if _, err := c.Request(ctx, "remotes.act", proto.RemoteAct{Remote: "vlc", Action: "toggle"}); !isCode(err, proto.ErrForbidden) {
		t.Errorf("remote perm: %v", err)
	}
	_ = e.store.SetPerms(dev.ID, pairing.DefaultPerms)
	e.srv.UpdatePerms(dev.ID, pairing.DefaultPerms)

	// A held key is released when the session drops.
	e.fake.Reset()
	must("key", proto.Key{K: "CTRL", A: "down"})
	c.Close()
	waitFor(t, e.fake, "key 0x11 false", 1)

	// Short-code pairing for a second device, then revoke it.
	st = e.pairer.Open(time.Minute)
	dev2 := proto.DeviceInfo{ID: "phone-2", Name: "Tablet"}
	resp2, seen2, err := client.Pair(ctx, "127.0.0.1", e.port, "", strings.ToLower(pairing.FormatShort(st.ShortCode)), dev2)
	if err != nil {
		t.Fatal(err)
	}
	if proto.Phrase(seen2, dev2.ID) != resp2.Phrase {
		t.Fatal("manual pairing phrase mismatch")
	}
	c2, err := client.Dial(ctx, "127.0.0.1", e.port, seen2, dev2.ID, resp2.Token)
	if err != nil {
		t.Fatal(err)
	}
	e.srv.Disconnect(dev2.ID, "revoked")
	_ = e.store.Revoke(dev2.ID)
	select {
	case ev := <-c2.Events:
		if ev.T != "bye" {
			t.Errorf("expected bye, got %s", ev.T)
		}
	case <-time.After(2 * time.Second):
		t.Error("no bye event")
	}
	if _, err := client.Dial(ctx, "127.0.0.1", e.port, seen2, dev2.ID, resp2.Token); err == nil {
		t.Error("revoked device must not connect")
	}
}

func TestPairingLockout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.pairer.Open(time.Minute)
	for i := 0; i < pairing.MaxFailures; i++ {
		_, _, _ = client.Pair(ctx, "127.0.0.1", e.port, "", "WRONG-CODE", proto.DeviceInfo{ID: "x"})
	}
	if e.pairer.State().Open {
		t.Error("window must close after repeated failures")
	}
}

func isCode(err error, code string) bool {
	var pe *proto.Error
	return errors.As(err, &pe) && pe.Code == code
}
