package proto

import (
	"encoding/binary"
	"fmt"
)

// Binary frame opcodes.
const (
	OpMove   byte = 0x01
	OpScroll byte = 0x02
	OpAbs    byte = 0x03
	OpButton byte = 0x04
)

// Frame is a decoded binary input frame.
type Frame struct {
	Op     byte
	X, Y   int32 // dx/dy for move and scroll; 0..65535 for absolute
	Seq    byte
	Button byte
	Down   bool
}

// DecodeFrame parses one binary frame. Unknown opcodes return ok=false and no error.
func DecodeFrame(b []byte) (f Frame, ok bool, err error) {
	if len(b) == 0 {
		return f, false, fmt.Errorf("empty binary frame")
	}
	f.Op = b[0]
	switch f.Op {
	case OpMove, OpScroll:
		if len(b) < 6 {
			return f, false, fmt.Errorf("short frame op=%d len=%d", f.Op, len(b))
		}
		f.X = int32(int16(binary.LittleEndian.Uint16(b[1:3])))
		f.Y = int32(int16(binary.LittleEndian.Uint16(b[3:5])))
		f.Seq = b[5]
	case OpAbs:
		if len(b) < 6 {
			return f, false, fmt.Errorf("short frame op=%d len=%d", f.Op, len(b))
		}
		f.X = int32(binary.LittleEndian.Uint16(b[1:3]))
		f.Y = int32(binary.LittleEndian.Uint16(b[3:5]))
		f.Seq = b[5]
	case OpButton:
		if len(b) < 3 {
			return f, false, fmt.Errorf("short frame op=%d len=%d", f.Op, len(b))
		}
		f.Button = b[1]
		f.Down = b[2] != 0
	default:
		return f, false, nil
	}
	return f, true, nil
}

// EncodeRel encodes a relative move or scroll frame.
func EncodeRel(op byte, dx, dy int16, seq byte) []byte {
	b := make([]byte, 6)
	b[0] = op
	binary.LittleEndian.PutUint16(b[1:3], uint16(dx))
	binary.LittleEndian.PutUint16(b[3:5], uint16(dy))
	b[5] = seq
	return b
}

// EncodeAbs encodes an absolute pointer frame.
func EncodeAbs(x, y uint16, seq byte) []byte {
	b := make([]byte, 6)
	b[0] = OpAbs
	binary.LittleEndian.PutUint16(b[1:3], x)
	binary.LittleEndian.PutUint16(b[3:5], y)
	b[5] = seq
	return b
}

// EncodeButton encodes a button frame.
func EncodeButton(button byte, down bool) []byte {
	d := byte(0)
	if down {
		d = 1
	}
	return []byte{OpButton, button, d}
}
