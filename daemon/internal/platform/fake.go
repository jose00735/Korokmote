package platform

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Fake is an in-memory platform that records every call. It is used for tests
// and for running the daemon on non-Windows machines during development.
type Fake struct {
	mu      sync.Mutex
	Log     []string
	level   float64
	muted   bool
	clip    string
	seq     uint32
	locked  bool
	fg      proto.WindowInfo
	Verbose bool
}

// NewFake returns a platform backed by a Fake recorder.
func NewFake(verbose bool) (*Platform, *Fake) {
	f := &Fake{level: 0.5, fg: proto.WindowInfo{H: 1, Title: "Desktop", Exe: "explorer.exe"}, Verbose: verbose}
	return &Platform{
		OS: "fake", Input: f, Audio: f, Power: f, Windows: f, Shell: f, Clipboard: f, Session: f,
	}, f
}

func (f *Fake) rec(format string, args ...any) error {
	s := fmt.Sprintf(format, args...)
	f.mu.Lock()
	f.Log = append(f.Log, s)
	if len(f.Log) > 10000 {
		f.Log = f.Log[5000:]
	}
	f.mu.Unlock()
	if f.Verbose {
		slog.Info("fake platform", "call", s)
	}
	return nil
}

// Calls returns a snapshot of recorded calls.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Log...)
}

// Reset clears recorded calls.
func (f *Fake) Reset() {
	f.mu.Lock()
	f.Log = nil
	f.mu.Unlock()
}

// Find returns recorded calls containing substr.
func (f *Fake) Find(substr string) []string {
	var out []string
	for _, c := range f.Calls() {
		if strings.Contains(c, substr) {
			out = append(out, c)
		}
	}
	return out
}

// SetLocked simulates session lock.
func (f *Fake) SetLocked(v bool) { f.mu.Lock(); f.locked = v; f.mu.Unlock() }

// SetForeground simulates a foreground window change.
func (f *Fake) SetForeground(w proto.WindowInfo) { f.mu.Lock(); f.fg = w; f.mu.Unlock() }

func (f *Fake) MoveRel(dx, dy int32) error     { return f.rec("move %d %d", dx, dy) }
func (f *Fake) MoveAbs(x, y uint16) error      { return f.rec("abs %d %d", x, y) }
func (f *Fake) Button(b Button, d bool) error  { return f.rec("button %d %v", b, d) }
func (f *Fake) Scroll(dx, dy int32) error      { return f.rec("scroll %d %d", dx, dy) }
func (f *Fake) Key(vk uint16, down bool) error { return f.rec("key 0x%02X %v", vk, down) }
func (f *Fake) Text(s string) error            { return f.rec("text %d runes", len([]rune(s))) }

func (f *Fake) Devices() ([]proto.AudioDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []proto.AudioDevice{{ID: "default", Name: "Fake Speakers", Default: true, Level: f.level, Muted: f.muted}}, nil
}
func (f *Fake) Volume(string) (float64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.level, f.muted, nil
}
func (f *Fake) SetVolume(_ string, l float64) error {
	f.mu.Lock()
	f.level = l
	f.mu.Unlock()
	return f.rec("volume %.2f", l)
}
func (f *Fake) SetMute(_ string, m bool) error {
	f.mu.Lock()
	f.muted = m
	f.mu.Unlock()
	return f.rec("mute %v", m)
}

func (f *Fake) Lock() error                    { return f.rec("power lock") }
func (f *Fake) Sleep() error                   { return f.rec("power sleep") }
func (f *Fake) Hibernate() error               { return f.rec("power hibernate") }
func (f *Fake) Restart(d time.Duration) error  { return f.rec("power restart %s", d) }
func (f *Fake) Shutdown(d time.Duration) error { return f.rec("power shutdown %s", d) }
func (f *Fake) Abort() error                   { return f.rec("power abort") }
func (f *Fake) MonitorOff() error              { return f.rec("power monitor_off") }
func (f *Fake) List() ([]proto.WindowInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []proto.WindowInfo{f.fg, {H: 2, Title: "Notes", Exe: "notepad.exe"}}, nil
}
func (f *Fake) Focus(h uint64) error { return f.rec("focus %d", h) }
func (f *Fake) Close(h uint64) error { return f.rec("close %d", h) }
func (f *Fake) Foreground() (proto.WindowInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fg, nil
}

func (f *Fake) Open(t string) error { return f.rec("open %s", t) }
func (f *Fake) Apps() ([]App, error) {
	return []App{{ID: "vlc", Name: "VLC media player", Path: "/fake/vlc"}, {ID: "notepad", Name: "Notepad", Path: "/fake/notepad"}}, nil
}
func (f *Fake) Drives() []string { return []string{"/"} }

func (f *Fake) Get() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clip, nil
}
func (f *Fake) Set(s string) error {
	f.mu.Lock()
	f.clip = s
	f.seq++
	f.mu.Unlock()
	return f.rec("clipboard set")
}
func (f *Fake) Sequence() uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq
}

func (f *Fake) Locked() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.locked
}
func (f *Fake) Screen() proto.Screen { return proto.Screen{W: 1920, H: 1080} }
