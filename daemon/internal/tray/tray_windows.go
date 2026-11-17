//go:build windows

package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

func sysDLL(name string) *syscall.LazyDLL {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return syscall.NewLazyDLL(filepath.Join(root, "System32", name))
}

var (
	user32  = sysDLL("user32.dll")
	shell32 = sysDLL("shell32.dll")
	kernel  = sysDLL("kernel32.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pRegisterWindowMessageW   = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	pAppendMenuW              = user32.NewProc("AppendMenuW")
	pTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	pDestroyMenu              = user32.NewProc("DestroyMenu")
	pGetCursorPos             = user32.NewProc("GetCursorPos")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	pGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	pShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	pGetModuleHandleW         = kernel.NewProc("GetModuleHandleW")
)

const (
	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmNull       = 0x0000
	wmApp        = 0x8000
	wmTray       = wmApp + 1
	wmLButtonUp  = 0x0202
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2
	nifMsg    = 0x1
	nifIcon   = 0x2
	nifTip    = 0x4
	nifInfo   = 0x10
	niifInfo  = 0x1

	mfString    = 0x0
	mfGrayed    = 0x1
	mfChecked   = 0x8
	mfSeparator = 0x800
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	_       uint32
}

// Tray is the running icon.
type Tray struct {
	cfg       Config
	hwnd      uintptr
	icon      uintptr
	mu        sync.Mutex
	nid       notifyIconData
	taskbarRe uintptr
}

var current *Tray

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func copyU16(dst []uint16, s string) {
	src, _ := syscall.UTF16FromString(s)
	n := copy(dst[:len(dst)-1], src)
	dst[n] = 0
}

// Run creates the icon and runs the message loop until Quit. It must be called
// from the main goroutine; it locks the OS thread.
func Run(cfg Config) error {
	runtime.LockOSThread()
	t := &Tray{cfg: cfg}
	current = t
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cls := u16("KorokmoteTray")
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(wndProc), hInstance: hInst, lpszClassName: cls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %v", err)
	}
	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16("Korokmote"))), 0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %v", err)
	}
	t.hwnd = hwnd
	t.taskbarRe, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	if len(cfg.IconPNG) > 0 {
		cx, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
		cy, _, _ := pGetSystemMetrics.Call(50)
		t.icon, _, _ = pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&cfg.IconPNG[0])), uintptr(len(cfg.IconPNG)), 1, 0x00030000, cx, cy, 0)
	}
	t.nid.cbSize = uint32(unsafe.Sizeof(t.nid))
	t.nid.hWnd = hwnd
	t.nid.uID = 1
	t.nid.uFlags = nifMsg | nifIcon | nifTip
	t.nid.uCallbackMessage = wmTray
	t.nid.hIcon = t.icon
	copyU16(t.nid.szTip[:], cfg.Tooltip)
	t.add()
	if cfg.OnReady != nil {
		go cfg.OnReady(t)
	}
	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	t.mu.Lock()
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
	t.mu.Unlock()
	return nil
}

func (t *Tray) add() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nid.uFlags = nifMsg | nifIcon | nifTip
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
}

// SetTooltip updates the hover text.
func (t *Tray) SetTooltip(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	copyU16(t.nid.szTip[:], s)
	t.nid.uFlags = nifTip
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
}

// Notify shows a balloon / toast notification.
func (t *Tray) Notify(title, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	copyU16(t.nid.szInfoTitle[:], title)
	copyU16(t.nid.szInfo[:], text)
	t.nid.dwInfoFlags = niifInfo
	t.nid.uFlags = nifInfo
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
	t.nid.szInfo[0] = 0
}

// Quit removes the icon and ends Run.
func (t *Tray) Quit() { pPostMessageW.Call(t.hwnd, wmClose, 0, 0) }

func (t *Tray) showMenu() {
	if t.cfg.Menu == nil {
		return
	}
	items := t.cfg.Menu()
	h, _, _ := pCreatePopupMenu.Call()
	for i, it := range items {
		if it.Title == "" && it.OnClick == nil {
			pAppendMenuW.Call(h, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if it.Disabled {
			flags |= mfGrayed
		}
		if it.Checked {
			flags |= mfChecked
		}
		pAppendMenuW.Call(h, flags, uintptr(i+1), uintptr(unsafe.Pointer(u16(it.Title))))
	}
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(t.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(h, 0x0100|0x0002|0x0080, uintptr(pt.x), uintptr(pt.y), 0, t.hwnd, 0)
	pPostMessageW.Call(t.hwnd, wmNull, 0, 0)
	pDestroyMenu.Call(h)
	if cmd > 0 && int(cmd) <= len(items) {
		if f := items[cmd-1].OnClick; f != nil && !items[cmd-1].Disabled {
			go f()
		}
	}
}

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	t := current
	switch {
	case m == wmTray && t != nil:
		switch lp & 0xFFFF {
		case wmRButtonUp:
			t.showMenu()
		case wmLButtonUp, wmLButtonDbl:
			if t.cfg.OnLeft != nil {
				go t.cfg.OnLeft()
			}
		}
		return 0
	case t != nil && t.taskbarRe != 0 && m == t.taskbarRe:
		t.add() // Explorer restarted
		return 0
	case m == wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case m == wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}
