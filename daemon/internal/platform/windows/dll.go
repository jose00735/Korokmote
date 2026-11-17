//go:build windows

// Package winplat implements the platform layer with the Win32 API, using only
// the standard syscall package (no cgo, no x/sys).
package winplat

import (
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// sysDLL loads a DLL from System32 by absolute path to avoid search-order hijacking.
func sysDLL(name string) *syscall.LazyDLL {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return syscall.NewLazyDLL(filepath.Join(root, "System32", name))
}

var (
	user32   = sysDLL("user32.dll")
	kernel32 = sysDLL("kernel32.dll")
	advapi32 = sysDLL("advapi32.dll")
	shell32  = sysDLL("shell32.dll")
	ole32    = sysDLL("ole32.dll")
	powrprof = sysDLL("powrprof.dll")
	dwmapi   = sysDLL("dwmapi.dll")
)

// ptr converts a uintptr returned by the OS into an unsafe.Pointer.
func ptr(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

// u16 converts a Go string into a NUL-terminated UTF-16 pointer.
func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

func u16p(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }

// readU16 reads a NUL-terminated UTF-16 string from OS memory.
func readU16(p uintptr) string {
	if p == 0 {
		return ""
	}
	base := ptr(p)
	n := 0
	for *(*uint16)(unsafe.Add(base, n*2)) != 0 {
		n++
		if n > 1<<20 {
			break
		}
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(base), n)))
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// callErr returns err only when the call reported failure (r == 0).
func callErr(r uintptr, err error, what string) error {
	if r != 0 {
		return nil
	}
	if errno, ok := err.(syscall.Errno); ok && errno != 0 {
		return &opError{what, errno}
	}
	return &opError{what, syscall.EINVAL}
}

type opError struct {
	op  string
	err error
}

func (e *opError) Error() string { return e.op + ": " + e.err.Error() }
func (e *opError) Unwrap() error { return e.err }
