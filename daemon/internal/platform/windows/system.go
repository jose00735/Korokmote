//go:build windows

package winplat

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/jose00735/korokmote/daemon/internal/platform"
	"github.com/jose00735/korokmote/daemon/internal/proto"
)

var (
	procLockWorkStation            = user32.NewProc("LockWorkStation")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procEnumWindows                = user32.NewProc("EnumWindows")
	procGetWindowTextW             = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW       = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procIsWindow                   = user32.NewProc("IsWindow")
	procGetWindow                  = user32.NewProc("GetWindow")
	procGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop           = user32.NewProc("BringWindowToTop")
	procIsIconic                   = user32.NewProc("IsIconic")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procAttachThreadInput          = user32.NewProc("AttachThreadInput")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procOpenInputDesktop           = user32.NewProc("OpenInputDesktop")
	procCloseDesktop               = user32.NewProc("CloseDesktop")
	procGetUserObjectInformationW  = user32.NewProc("GetUserObjectInformationW")

	procGetCurrentThreadId         = kernel32.NewProc("GetCurrentThreadId")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGetLogicalDrives           = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW              = kernel32.NewProc("GetDriveTypeW")

	procLookupPrivilegeValueW     = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges     = advapi32.NewProc("AdjustTokenPrivileges")
	procInitiateSystemShutdownExW = advapi32.NewProc("InitiateSystemShutdownExW")
	procAbortSystemShutdownW      = advapi32.NewProc("AbortSystemShutdownW")

	procSetSuspendState       = powrprof.NewProc("SetSuspendState")
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
	procShellExecuteW         = shell32.NewProc("ShellExecuteW")
)

// ---- Power ----

type power struct{}

type luid struct {
	low  uint32
	high int32
}

type tokenPrivileges struct {
	count uint32
	luid  luid
	attrs uint32
}

func enableShutdownPrivilege() error {
	proc, _ := syscall.GetCurrentProcess()
	var tok syscall.Token
	if err := syscall.OpenProcessToken(proc, 0x0020|0x0008, &tok); err != nil {
		return fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer tok.Close()
	var l luid
	r, _, err := procLookupPrivilegeValueW.Call(0, u16p("SeShutdownPrivilege"), uintptr(unsafe.Pointer(&l)))
	if e := callErr(r, err, "LookupPrivilegeValue"); e != nil {
		return e
	}
	tp := tokenPrivileges{count: 1, luid: l, attrs: 0x2}
	r, _, err = procAdjustTokenPrivileges.Call(uintptr(tok), 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0)
	return callErr(r, err, "AdjustTokenPrivileges")
}

func (power) Lock() error {
	r, _, err := procLockWorkStation.Call()
	return callErr(r, err, "LockWorkStation")
}

func suspend(hibernate bool) error {
	if err := enableShutdownPrivilege(); err != nil {
		return err
	}
	// Run asynchronously: the call blocks until the machine resumes.
	go procSetSuspendState.Call(boolArg(hibernate), 0, 0)
	return nil
}

func (power) Sleep() error     { return suspend(false) }
func (power) Hibernate() error { return suspend(true) }

func shutdown(reboot bool, delay time.Duration) error {
	if err := enableShutdownPrivilege(); err != nil {
		return err
	}
	msg := "Korokmote: shutdown requested from a paired phone."
	if reboot {
		msg = "Korokmote: restart requested from a paired phone."
	}
	secs := uintptr(delay / time.Second)
	r, _, err := procInitiateSystemShutdownExW.Call(0, u16p(msg), secs, 0, boolArg(reboot), 0x80000000)
	return callErr(r, err, "InitiateSystemShutdownEx")
}

func (power) Restart(d time.Duration) error  { return shutdown(true, d) }
func (power) Shutdown(d time.Duration) error { return shutdown(false, d) }

func (power) Abort() error {
	if err := enableShutdownPrivilege(); err != nil {
		return err
	}
	r, _, err := procAbortSystemShutdownW.Call(0)
	return callErr(r, err, "AbortSystemShutdown")
}

func (power) MonitorOff() error {
	r, _, err := procPostMessageW.Call(0xFFFF, 0x0112, 0xF170, 2)
	return callErr(r, err, "PostMessage(SC_MONITORPOWER)")
}

// ---- Windows ----

type windows struct{}

var (
	enumMu    sync.Mutex
	enumFound []uintptr
	enumCB    = syscall.NewCallback(func(h, _ uintptr) uintptr {
		enumFound = append(enumFound, h)
		return 1
	})
)

func windowTitle(h uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(h)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func windowExe(h uintptr) string {
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	ph, err := syscall.OpenProcess(0x1000, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(ph)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(ph), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(syscall.UTF16ToString(buf[:size])))
}

func isAppWindow(h uintptr) bool {
	if v, _, _ := procIsWindowVisible.Call(h); v == 0 {
		return false
	}
	if owner, _, _ := procGetWindow.Call(h, 4); owner != 0 {
		return false
	}
	ex, _, _ := procGetWindowLongPtrW.Call(h, ^uintptr(19)) // GWL_EXSTYLE = -20
	if ex&0x00000080 != 0 {                                 // WS_EX_TOOLWINDOW
		return false
	}
	var cloaked uint32
	procDwmGetWindowAttribute.Call(h, 14, uintptr(unsafe.Pointer(&cloaked)), 4)
	return cloaked == 0
}

func (windows) List() ([]proto.WindowInfo, error) {
	enumMu.Lock()
	enumFound = enumFound[:0]
	procEnumWindows.Call(enumCB, 0)
	handles := append([]uintptr(nil), enumFound...)
	enumMu.Unlock()
	var out []proto.WindowInfo
	for _, h := range handles {
		if !isAppWindow(h) {
			continue
		}
		title := windowTitle(h)
		if title == "" || title == "Program Manager" {
			continue
		}
		out = append(out, proto.WindowInfo{H: uint64(h), Title: title, Exe: windowExe(h)})
	}
	return out, nil
}

func (windows) Focus(h uint64) error {
	hw := uintptr(h)
	if ok, _, _ := procIsWindow.Call(hw); ok == 0 {
		return proto.Errorf(proto.ErrNotFound, "window not found")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ic, _, _ := procIsIconic.Call(hw); ic != 0 {
		procShowWindow.Call(hw, 9) // SW_RESTORE
	}
	fg, _, _ := procGetForegroundWindow.Call()
	fgThread, _, _ := procGetWindowThreadProcessId.Call(fg, 0)
	cur, _, _ := procGetCurrentThreadId.Call()
	if fgThread != 0 && fgThread != cur {
		procAttachThreadInput.Call(cur, fgThread, 1)
		defer procAttachThreadInput.Call(cur, fgThread, 0)
	}
	procBringWindowToTop.Call(hw)
	r, _, _ := procSetForegroundWindow.Call(hw)
	if r == 0 {
		// Fallback: a synthetic Alt tap lets this process take the foreground.
		_ = sendKeys([]inputK{keyEvent(0x12, true), keyEvent(0x12, false)})
		procSetForegroundWindow.Call(hw)
	}
	return nil
}

func (windows) Close(h uint64) error {
	r, _, err := procPostMessageW.Call(uintptr(h), 0x0010, 0, 0)
	return callErr(r, err, "PostMessage(WM_CLOSE)")
}

func (windows) Foreground() (proto.WindowInfo, error) {
	h, _, _ := procGetForegroundWindow.Call()
	if h == 0 {
		return proto.WindowInfo{}, nil
	}
	return proto.WindowInfo{H: uint64(h), Title: windowTitle(h), Exe: windowExe(h)}, nil
}

// ---- Shell ----

type shell struct {
	mu      sync.Mutex
	cache   []platform.App
	cacheAt time.Time
}

// ShellOpen opens a file, folder or URL with its default handler.
func ShellOpen(target string) error {
	r, _, err := procShellExecuteW.Call(0, u16p("open"), u16p(target), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("ShellExecute(%d): %v", r, err)
	}
	return nil
}

func (s *shell) Open(target string) error { return ShellOpen(target) }

var skipAppWords = []string{"uninstall", "readme", "release notes", "help", "documentation", "website", "license", "manual"}

func (s *shell) Apps() ([]platform.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache != nil && time.Since(s.cacheAt) < time.Minute {
		return s.cache, nil
	}
	roots := []string{
		filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
	}
	seen := map[string]bool{}
	var apps []platform.App
	for ri, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".lnk" && ext != ".url" {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			low := strings.ToLower(name)
			for _, w := range skipAppWords {
				if strings.Contains(low, w) {
					return nil
				}
			}
			if seen[low] {
				return nil
			}
			seen[low] = true
			rel, _ := filepath.Rel(root, p)
			apps = append(apps, platform.App{ID: fmt.Sprintf("%d:%s", ri, strings.ToLower(rel)), Name: name, Path: p})
			return nil
		})
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })
	s.cache, s.cacheAt = apps, time.Now()
	return apps, nil
}

func (s *shell) Drives() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		t, _, _ := procGetDriveTypeW.Call(u16p(root))
		if t == 2 || t == 3 || t == 4 { // removable, fixed, network
			out = append(out, root)
		}
	}
	return out
}

// ---- Clipboard ----

type clipboard struct{}

func openClipboard() error {
	for i := 0; i < 10; i++ {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("clipboard busy")
}

func (clipboard) Get() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(13) // CF_UNICODETEXT
	if h == 0 {
		return "", nil
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return "", fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)
	return readU16(p), nil
}

func (clipboard) Set(s string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	data := utf16.Encode([]rune(s + "\x00"))
	size := uintptr(len(data) * 2)
	h, _, err := procGlobalAlloc.Call(0x0002, size) // GMEM_MOVEABLE
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %v", err)
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("GlobalLock failed")
	}
	copy(unsafe.Slice((*uint16)(ptr(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if err := openClipboard(); err != nil {
		procGlobalFree.Call(h)
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if r, _, err := procSetClipboardData.Call(13, h); r == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("SetClipboardData: %v", err)
	}
	return nil
}

func (clipboard) Sequence() uint32 {
	r, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(r)
}

// ---- Session ----

type session struct{}

func (session) Locked() bool {
	h, _, _ := procOpenInputDesktop.Call(0, 0, 0x0001)
	if h == 0 {
		return true
	}
	defer procCloseDesktop.Call(h)
	buf := make([]uint16, 64)
	var needed uint32
	r, _, _ := procGetUserObjectInformationW.Call(h, 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return false
	}
	return !strings.EqualFold(syscall.UTF16ToString(buf), "Default")
}

func (session) Screen() proto.Screen {
	w, _, _ := procGetSystemMetrics.Call(78)
	h, _, _ := procGetSystemMetrics.Call(79)
	if w == 0 || h == 0 {
		w, _, _ = procGetSystemMetrics.Call(0)
		h, _, _ = procGetSystemMetrics.Call(1)
	}
	return proto.Screen{W: int(w), H: int(h)}
}

// New returns the Windows platform.
func New() *platform.Platform {
	return &platform.Platform{
		OS:        "windows",
		Input:     input{},
		Audio:     newAudio(),
		Power:     power{},
		Windows:   windows{},
		Shell:     &shell{},
		Clipboard: clipboard{},
		Session:   session{},
	}
}
