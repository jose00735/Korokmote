// Package platform defines the OS abstraction used by the daemon.
// Windows implementations live in platform/windows; Fake is used elsewhere.
package platform

import (
	"errors"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// ErrUnsupported is returned by operations the platform cannot perform.
var ErrUnsupported = errors.New("not supported on this platform")

// Button identifies a mouse button (protocol numbering).
type Button uint8

const (
	ButtonLeft    Button = 1
	ButtonRight   Button = 2
	ButtonMiddle  Button = 3
	ButtonBack    Button = 4
	ButtonForward Button = 5
)

// Input injects pointer and keyboard events.
type Input interface {
	MoveRel(dx, dy int32) error
	MoveAbs(x, y uint16) error // 0..65535 across the virtual desktop
	Button(b Button, down bool) error
	Scroll(dx, dy int32) error // wheel units, 120 = one notch
	Key(vk uint16, down bool) error
	Text(s string) error
}

// Audio controls output volume.
type Audio interface {
	Devices() ([]proto.AudioDevice, error)
	Volume(deviceID string) (level float64, muted bool, err error)
	SetVolume(deviceID string, level float64) error
	SetMute(deviceID string, muted bool) error
}

// Power controls the machine's power state.
type Power interface {
	Lock() error
	Sleep() error
	Hibernate() error
	Restart(delay time.Duration) error
	Shutdown(delay time.Duration) error
	Abort() error
	MonitorOff() error
}

// Windows lists and focuses top-level windows.
type Windows interface {
	List() ([]proto.WindowInfo, error)
	Focus(h uint64) error
	Close(h uint64) error
	Foreground() (proto.WindowInfo, error)
}

// Shell opens files/URLs and lists launchable apps.
type Shell interface {
	Open(target string) error
	Apps() ([]App, error)
	Drives() []string
}

// App is a launchable application.
type App struct {
	ID   string
	Name string
	Path string
}

// Clipboard reads and writes text.
type Clipboard interface {
	Get() (string, error)
	Set(string) error
	Sequence() uint32 // changes whenever the clipboard changes
}

// Session reports interactive session state.
type Session interface {
	Locked() bool
	Screen() proto.Screen
}

// Platform bundles every capability.
type Platform struct {
	OS        string
	Input     Input
	Audio     Audio
	Power     Power
	Windows   Windows
	Shell     Shell
	Clipboard Clipboard
	Session   Session
}
