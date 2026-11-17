package proto

import "strings"

// Keys maps protocol key names (PROTOCOL.md section 8) to Windows virtual-key codes.
// The codes are used as the platform-neutral key identity inside the daemon.
var Keys = map[string]uint16{
	"BACKSPACE": 0x08, "TAB": 0x09, "ENTER": 0x0D, "SHIFT": 0x10, "CTRL": 0x11, "ALT": 0x12,
	"PAUSE": 0x13, "CAPS": 0x14, "ESC": 0x1B, "SPACE": 0x20, "PGUP": 0x21, "PGDN": 0x22,
	"END": 0x23, "HOME": 0x24, "LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28,
	"PRTSC": 0x2C, "INSERT": 0x2D, "DELETE": 0x2E, "WIN": 0x5B, "MENU": 0x5D,
	"NUM0": 0x60, "NUM1": 0x61, "NUM2": 0x62, "NUM3": 0x63, "NUM4": 0x64,
	"NUM5": 0x65, "NUM6": 0x66, "NUM7": 0x67, "NUM8": 0x68, "NUM9": 0x69,
	"NUM_MUL": 0x6A, "NUM_ADD": 0x6B, "NUM_SUB": 0x6D, "NUM_DOT": 0x6E, "NUM_DIV": 0x6F,
	"NUMLOCK": 0x90, "SCROLLLOCK": 0x91,
	"BROWSER_BACK": 0xA6, "BROWSER_FWD": 0xA7, "BROWSER_REFRESH": 0xA8,
	"VOL_MUTE": 0xAD, "VOL_DOWN": 0xAE, "VOL_UP": 0xAF,
	"MEDIA_NEXT": 0xB0, "MEDIA_PREV": 0xB1, "MEDIA_STOP": 0xB2, "MEDIA_PLAY": 0xB3,
	"SEMICOLON": 0xBA, "EQUAL": 0xBB, "COMMA": 0xBC, "MINUS": 0xBD, "PERIOD": 0xBE,
	"SLASH": 0xBF, "GRAVE": 0xC0, "LBRACKET": 0xDB, "BACKSLASH": 0xDC, "RBRACKET": 0xDD,
	"QUOTE": 0xDE,
}

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		Keys[string(c)] = uint16(c)
	}
	for c := '0'; c <= '9'; c++ {
		Keys[string(c)] = uint16(c)
	}
	for i := 1; i <= 24; i++ {
		Keys["F"+itoa(i)] = uint16(0x70 + i - 1)
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// KeyCode resolves a key name, case-insensitively.
func KeyCode(name string) (uint16, bool) {
	vk, ok := Keys[strings.ToUpper(strings.TrimSpace(name))]
	return vk, ok
}

// MediaKeys maps media actions to key names.
var MediaKeys = map[string]string{
	"playpause": "MEDIA_PLAY",
	"next":      "MEDIA_NEXT",
	"prev":      "MEDIA_PREV",
	"stop":      "MEDIA_STOP",
}
