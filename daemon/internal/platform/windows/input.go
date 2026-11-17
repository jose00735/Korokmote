//go:build windows

package winplat

import (
	"fmt"
	"unicode/utf16"
	"unsafe"

	"github.com/jose00735/korokmote/daemon/internal/platform"
)

var (
	procSendInput        = user32.NewProc("SendInput")
	procMapVirtualKeyW   = user32.NewProc("MapVirtualKeyW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseMove       = 0x0001
	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseXDown      = 0x0080
	mouseXUp        = 0x0100
	mouseWheel      = 0x0800
	mouseHWheel     = 0x1000
	mouseVirtual    = 0x4000
	mouseAbsolute   = 0x8000

	keyExtended = 0x0001
	keyUp       = 0x0002
	keyUnicode  = 0x0004
)

type mouseInput struct {
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	extra     uintptr
}

type keybdInput struct {
	vk, scan uint16
	flags    uint32
	time     uint32
	extra    uintptr
}

// The Win32 INPUT struct is 40 bytes on 64-bit Windows.
type inputM struct {
	typ uint32
	mi  mouseInput
}

type inputK struct {
	typ uint32
	ki  keybdInput
	_   [8]byte
}

// Compile-time size checks (64-bit only).
var (
	_ [40]byte = [unsafe.Sizeof(inputM{})]byte{}
	_ [40]byte = [unsafe.Sizeof(inputK{})]byte{}
)

var extendedVK = map[uint16]bool{
	0x21: true, 0x22: true, 0x23: true, 0x24: true, 0x25: true, 0x26: true, 0x27: true, 0x28: true,
	0x2C: true, 0x2D: true, 0x2E: true, 0x5B: true, 0x5C: true, 0x5D: true, 0x6F: true, 0x90: true,
	0xA3: true, 0xA5: true,
}

type input struct{}

func sendMouse(in []inputM) error {
	if len(in) == 0 {
		return nil
	}
	r, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(r) != len(in) {
		return fmt.Errorf("SendInput mouse: %v", err)
	}
	return nil
}

func sendKeys(in []inputK) error {
	for len(in) > 0 {
		n := min(len(in), 64)
		r, _, err := procSendInput.Call(uintptr(n), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
		if int(r) != n {
			return fmt.Errorf("SendInput keyboard: %v", err)
		}
		in = in[n:]
	}
	return nil
}

func (input) MoveRel(dx, dy int32) error {
	return sendMouse([]inputM{{typ: inputMouse, mi: mouseInput{dx: dx, dy: dy, flags: mouseMove}}})
}

func (input) MoveAbs(x, y uint16) error {
	return sendMouse([]inputM{{typ: inputMouse, mi: mouseInput{dx: int32(x), dy: int32(y), flags: mouseMove | mouseAbsolute | mouseVirtual}}})
}

func (input) Button(b platform.Button, down bool) error {
	var flags, data uint32
	switch b {
	case platform.ButtonLeft:
		flags = pick(down, mouseLeftDown, mouseLeftUp)
	case platform.ButtonRight:
		flags = pick(down, mouseRightDown, mouseRightUp)
	case platform.ButtonMiddle:
		flags = pick(down, mouseMiddleDown, mouseMiddleUp)
	case platform.ButtonBack:
		flags, data = pick(down, mouseXDown, mouseXUp), 1
	case platform.ButtonForward:
		flags, data = pick(down, mouseXDown, mouseXUp), 2
	default:
		return fmt.Errorf("unknown button %d", b)
	}
	return sendMouse([]inputM{{typ: inputMouse, mi: mouseInput{flags: flags, mouseData: data}}})
}

func pick(c bool, a, b uint32) uint32 {
	if c {
		return a
	}
	return b
}

func (input) Scroll(dx, dy int32) error {
	var in []inputM
	if dy != 0 {
		in = append(in, inputM{typ: inputMouse, mi: mouseInput{flags: mouseWheel, mouseData: uint32(dy)}})
	}
	if dx != 0 {
		in = append(in, inputM{typ: inputMouse, mi: mouseInput{flags: mouseHWheel, mouseData: uint32(dx)}})
	}
	return sendMouse(in)
}

func keyEvent(vk uint16, down bool) inputK {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0)
	var flags uint32
	if extendedVK[vk] {
		flags |= keyExtended
	}
	if !down {
		flags |= keyUp
	}
	return inputK{typ: inputKeyboard, ki: keybdInput{vk: vk, scan: uint16(scan), flags: flags}}
}

func (input) Key(vk uint16, down bool) error {
	return sendKeys([]inputK{keyEvent(vk, down)})
}

func (input) Text(s string) error {
	var in []inputK
	for _, r := range s {
		switch r {
		case '\n':
			in = append(in, keyEvent(0x0D, true), keyEvent(0x0D, false))
			continue
		case '\r':
			continue
		case '\t':
			in = append(in, keyEvent(0x09, true), keyEvent(0x09, false))
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			in = append(in,
				inputK{typ: inputKeyboard, ki: keybdInput{scan: unit, flags: keyUnicode}},
				inputK{typ: inputKeyboard, ki: keybdInput{scan: unit, flags: keyUnicode | keyUp}},
			)
		}
	}
	return sendKeys(in)
}
