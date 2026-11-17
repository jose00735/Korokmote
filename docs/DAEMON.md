# Windows daemon (`korokmoted`)

Language: Go (current stable). Target: `windows/amd64` first, `windows/arm64` when convenient. No cgo.

Build: `go build -ldflags "-H=windowsgui -s -w" ./cmd/korokmoted` (the `windowsgui` flag hides the console window).

## Process model

- One process per logged-in user, started at logon, visible as a tray icon.
- Single-instance guard: a named mutex `Local\Korokmote`.
- Autostart: `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, toggled from the tray. An optional "start elevated" mode registers a Task Scheduler logon task with highest privileges instead (see limits below).
- Goroutines: accept loop, one reader and one writer per session, one input worker that serialises all `SendInput` calls, discovery, tray message loop (locked to its OS thread).

## Windows limits to design around

| Limit | Consequence | Handling |
|---|---|---|
| Session 0 isolation | A service cannot inject input | Run in the user session (tray process) |
| UIPI | A normal process cannot send input to elevated windows (Task Manager, admin terminals) | Optional elevated autostart; documented |
| Secure desktop | UAC prompts and the lock/login screen ignore injected input | Out of scope for v1; app shows "PC locked" state |
| Fast user switching | Each session needs its own daemon | Port is per-user configurable; default port falls back to the next free one |
| Windows Firewall | First run triggers the inbound prompt | Installer adds a rule for private networks only |

## Packages and dependencies

Check for the latest maintained versions when implementing; these are the intended choices, kept few on purpose.

| Concern | Package | Notes |
|---|---|---|
| Win32 calls | `golang.org/x/sys/windows` | `user32`, `kernel32`, `powrprof`, `shell32` via lazy DLL procs |
| WebSocket | `github.com/coder/websocket` | context-aware, small API |
| TLS | standard library `crypto/tls` | TLS 1.3 only, ECDSA P-256 self-signed certificate |
| mDNS | `github.com/libp2p/zeroconf/v2` | advertises `_korokmote._tcp` |
| Tray | `fyne.io/systray` | no cgo on Windows |
| Core Audio (volume) | `github.com/go-ole/go-ole` + `github.com/moutend/go-wca` | `IAudioEndpointVolume`, device enumeration |
| QR code | `github.com/skip2/go-qrcode` | rendered into the admin page |
| YAML | `gopkg.in/yaml.v3` | remotes and config |
| Logging | standard library `log/slog` | rotating file in `%LOCALAPPDATA%` |

## Platform layer

`internal/platform` defines interfaces; `internal/platform/windows` implements them. Everything above this layer is OS-independent.

```go
type Input interface {
    MoveRel(dx, dy int32) error
    MoveAbs(x, y float32) error            // 0..1 of the virtual desktop
    Button(b Button, down bool) error
    Scroll(dx, dy int32) error             // in wheel units (120 = one notch)
    Key(vk uint16, down bool) error
    Text(s string) error                   // Unicode, layout-independent
}

type Audio interface {
    Devices() ([]AudioDevice, error)
    Volume(deviceID string) (level float32, muted bool, err error)
    SetVolume(deviceID string, level float32) error
    SetMute(deviceID string, muted bool) error
    Subscribe(func(AudioEvent)) (cancel func())
}

type Power interface {
    Lock() error
    Sleep() error
    Hibernate() error
    Restart(delay time.Duration) error
    Shutdown(delay time.Duration) error
    Abort() error
    MonitorOff() error
}

type Windows interface {
    List() ([]WindowInfo, error)
    Focus(hwnd uintptr) error
    Close(hwnd uintptr) error
    Foreground() (WindowInfo, error)
    SubscribeForeground(func(WindowInfo)) (cancel func())
}

type Files interface {
    Roots() ([]Entry, error)
    List(path string) ([]Entry, error)
    Open(path string) error                // ShellExecute
}

type Launcher interface {
    Apps() ([]AppInfo, error)              // Start Menu shortcuts
    Launch(id string) error
}

type Clipboard interface {
    Get() (string, error)
    Set(string) error
}
```

### Win32 mapping

| Feature | API |
|---|---|
| Mouse move, buttons, wheel | `SendInput` with `MOUSEINPUT` (`MOUSEEVENTF_MOVE`, `..._ABSOLUTE \| VIRTUALDESK`, `..._WHEEL`, `..._HWHEEL`) |
| Keys | `SendInput` with `KEYBDINPUT` using scan codes (`MapVirtualKeyW`) plus `KEYEVENTF_EXTENDEDKEY` where required |
| Text | `SendInput` with `KEYEVENTF_UNICODE`, one down/up pair per UTF-16 unit; surrogate pairs sent in order |
| Media keys | `VK_MEDIA_PLAY_PAUSE`, `VK_MEDIA_NEXT_TRACK`, `VK_MEDIA_PREV_TRACK`, `VK_MEDIA_STOP` |
| Volume | Core Audio `IMMDeviceEnumerator` -> `IAudioEndpointVolume`; change notifications pushed to clients |
| Lock | `LockWorkStation` |
| Sleep / hibernate | `SetSuspendState` (`powrprof.dll`) |
| Shutdown / restart | `InitiateSystemShutdownExW` after enabling `SeShutdownPrivilege`; `AbortSystemShutdownW` to cancel |
| Monitor off | `SendMessage(HWND_BROADCAST, WM_SYSCOMMAND, SC_MONITORPOWER, 2)` |
| Window list / focus | `EnumWindows`, `GetWindowTextW`, `IsWindowVisible`, `SetForegroundWindow` (with `AttachThreadInput` workaround) |
| Foreground change | `SetWinEventHook(EVENT_SYSTEM_FOREGROUND)` |
| Open file / launch | `ShellExecuteW` |
| Clipboard | `OpenClipboard`, `GetClipboardData(CF_UNICODETEXT)`, `SetClipboardData` |
| Session locked state | `WTSRegisterSessionNotification` -> `WM_WTSSESSION_CHANGE` |

### Input worker

All input goes through one goroutine reading from a buffered channel:

- Pointer deltas that arrive while the worker is busy are **coalesced** (summed) so a slow moment never builds a backlog; the cursor always catches up in one call.
- Key and button events are never coalesced or dropped, and keep their order relative to motion.
- On session close, the worker releases every key and button that session still holds down, so a dropped connection can never leave Ctrl or a mouse button stuck.
- Pointer acceleration is applied on the phone; the daemon moves exactly what it is told.

## Server

- Listens on `0.0.0.0:<port>` (default **47810**, configurable). Only RFC 1918 / link-local / loopback peers are accepted unless `allow_public: true`.
- TLS 1.3 only; HTTP/1.1 upgrade to WebSocket at `/v1/ws`. Any other path returns 404, except `/v1/pair` (see `PROTOCOL.md`).
- A session is unauthenticated until a valid `hello` arrives; it is closed if that does not happen within 3 s.
- Limits: max 8 sessions, 64 KiB max message, 300 control messages per second per session, 10 failed auth attempts per minute per IP then a 5-minute block.
- Read deadline 15 s, refreshed by any frame; heartbeats come from the client every 5 s.

## Admin page and tray

Tray menu: status line (connected devices), **Pair a device…**, **Devices…**, **Open remotes folder**, **Start with Windows**, **Pause** (reject all input), **Quit**.

"Pair a device" and "Devices" open the admin page in the default browser: an embedded (`embed.FS`) single page served on `127.0.0.1` only, on a random port, guarded by a one-time URL token. It shows the pairing QR code with a countdown, the list of paired devices with revoke buttons, per-device permissions, and recent log lines. The page follows the same Matrix style as the app (`DESIGN.md`).

## Files on disk

```
%APPDATA%\Korokmote\
├── config.yaml         port, name, allow_public, autostart mode
├── identity.json       daemon ID (UUID)
├── cert.pem / key.pem  TLS identity (key ACL: current user only)
├── devices.json        paired devices: id, name, token hash, permissions, last seen
└── remotes\            user remotes (override built-ins by id)
%LOCALAPPDATA%\Korokmote\logs\korokmoted.log
```

`config.yaml` example:

```yaml
name: "DESKTOP-JOSE"
port: 47810
allow_public: false
autostart: user          # user | elevated | off
pairing_window: 120s
file_roots:              # what the file browser may show; empty = user profile + drives
  - "C:\\Users\\jose\\Videos"
```

## Packaging

- Release artefacts: `korokmoted.exe` (portable) and an installer that adds the firewall rule (private profile), the autostart entry and a Start Menu shortcut.
- Versioning: `git describe` injected with `-ldflags -X`.
- Releases are code-signed when a certificate is available; SHA-256 checksums are always published.

## `korokctl`

A Go CLI that speaks the same protocol, used for development and CI:

```
korokctl discover
korokctl pair --qr-payload "<string>"
korokctl send key --vk 0x41
korokctl send text "hello"
korokctl bench --seconds 10      # round-trip latency percentiles
```
