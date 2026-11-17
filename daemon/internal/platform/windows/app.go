//go:build windows

package winplat

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procCreateMutexW                  = kernel32.NewProc("CreateMutexW")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procRegOpenKeyExW                 = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW                = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW               = advapi32.NewProc("RegDeleteValueW")
	procRegQueryValueExW              = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey                   = advapi32.NewProc("RegCloseKey")
)

// SingleInstance takes a named mutex. ok is false if another instance holds it.
func SingleInstance(name string) (ok bool) {
	h, _, err := procCreateMutexW.Call(0, 0, u16p(name))
	if h == 0 {
		return true
	}
	if errno, isErrno := err.(syscall.Errno); isErrno && errno == 183 { // ERROR_ALREADY_EXISTS
		return false
	}
	return true
}

// MessageBox shows a blocking message box.
func MessageBox(title, text string, isError bool) {
	flags := uintptr(0x40) // MB_ICONINFORMATION
	if isError {
		flags = 0x10 // MB_ICONERROR
	}
	procMessageBoxW.Call(0, u16p(text), u16p(title), flags)
}

// SetDPIAware opts into per-monitor DPI awareness so screen metrics are physical pixels.
func SetDPIAware() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		procSetProcessDpiAwarenessContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	}
}

const (
	hkeyCurrentUser = 0x80000001
	runKey          = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue        = "Korokmote"
)

func openRun(access uintptr) (uintptr, error) {
	var key uintptr
	r, _, _ := procRegOpenKeyExW.Call(hkeyCurrentUser, u16p(runKey), 0, access, uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return 0, fmt.Errorf("RegOpenKeyEx: %d", r)
	}
	return key, nil
}

// Autostart reports whether the HKCU Run entry exists.
func Autostart() bool {
	key, err := openRun(0x0001) // KEY_QUERY_VALUE
	if err != nil {
		return false
	}
	defer procRegCloseKey.Call(key)
	r, _, _ := procRegQueryValueExW.Call(key, u16p(runValue), 0, 0, 0, 0)
	return r == 0
}

// SetAutostart adds or removes the HKCU Run entry for exe.
func SetAutostart(enabled bool, exe string) error {
	key, err := openRun(0x0002 | 0x0001) // KEY_SET_VALUE | KEY_QUERY_VALUE
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(key)
	if !enabled {
		r, _, _ := procRegDeleteValueW.Call(key, u16p(runValue))
		if r != 0 && r != 2 {
			return fmt.Errorf("RegDeleteValue: %d", r)
		}
		return nil
	}
	val, _ := syscall.UTF16FromString(`"` + exe + `"`)
	r, _, _ := procRegSetValueExW.Call(key, u16p(runValue), 0, 1 /* REG_SZ */, uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueEx: %d", r)
	}
	return nil
}

// InstallerAutostart returns the installer's autostart choice ("user", "off" or "").
func InstallerAutostart() string {
	var key uintptr
	r, _, _ := procRegOpenKeyExW.Call(0x80000002 /* HKLM */, u16p(`Software\Korokmote`), 0, 0x0001|0x0100 /* QUERY | WOW64_64KEY */, uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return ""
	}
	defer procRegCloseKey.Call(key)
	buf := make([]uint16, 32)
	size := uint32(len(buf) * 2)
	var typ uint32
	r, _, _ = procRegQueryValueExW.Call(key, u16p("Autostart"), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 || typ != 1 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
