package proto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func vectors(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "vectors", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestBinaryVectors(t *testing.T) {
	var v struct {
		Frames []struct {
			Name   string
			Op     byte
			X, Y   int32
			Seq    byte
			Button byte
			Down   bool
			Hex    string
		}
	}
	vectors(t, "binary.json", &v)
	for _, f := range v.Frames {
		raw, _ := hex.DecodeString(f.Hex)
		got, ok, err := DecodeFrame(raw)
		if err != nil || !ok {
			t.Fatalf("%s: decode failed: %v", f.Name, err)
		}
		var enc []byte
		switch f.Op {
		case OpMove, OpScroll:
			enc = EncodeRel(f.Op, int16(f.X), int16(f.Y), f.Seq)
			if got.X != f.X || got.Y != f.Y || got.Seq != f.Seq {
				t.Errorf("%s: decoded %+v", f.Name, got)
			}
		case OpAbs:
			enc = EncodeAbs(uint16(f.X), uint16(f.Y), f.Seq)
			if got.X != f.X || got.Y != f.Y {
				t.Errorf("%s: decoded %+v", f.Name, got)
			}
		case OpButton:
			enc = EncodeButton(f.Button, f.Down)
			if got.Button != f.Button || got.Down != f.Down {
				t.Errorf("%s: decoded %+v", f.Name, got)
			}
		}
		if hex.EncodeToString(enc) != f.Hex {
			t.Errorf("%s: encoded %x want %s", f.Name, enc, f.Hex)
		}
	}
	if _, ok, _ := DecodeFrame([]byte{0x7F, 1, 2}); ok {
		t.Error("unknown opcode must be ignored")
	}
	if _, _, err := DecodeFrame([]byte{OpMove, 1}); err == nil {
		t.Error("short frame must error")
	}
}

func TestPhraseVectors(t *testing.T) {
	if len(Wordlist) != 256 {
		t.Fatalf("wordlist has %d words", len(Wordlist))
	}
	var v struct {
		Cases []struct{ FP, Device, Phrase string }
	}
	vectors(t, "phrase.json", &v)
	for _, c := range v.Cases {
		if got := Phrase(c.FP, c.Device); got != c.Phrase {
			t.Errorf("Phrase(%s,%s)=%s want %s", c.FP[:8], c.Device, got, c.Phrase)
		}
	}
}

func TestMessageVectors(t *testing.T) {
	var v struct{ Messages []json.RawMessage }
	vectors(t, "messages.json", &v)
	for _, raw := range v.Messages {
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(env.D, &payload); err != nil {
			t.Fatalf("payload %s: %v", raw, err)
		}
		switch env.T {
		case "hello":
			var h Hello
			if json.Unmarshal(env.D, &h) != nil || h.V != 1 || h.Device == "" {
				t.Errorf("hello: %+v", h)
			}
		case "text":
			var x Text
			if json.Unmarshal(env.D, &x) != nil || x.S != "héllo 👋" {
				t.Errorf("text: %+v", x)
			}
		case "audio.set":
			var a AudioSet
			if json.Unmarshal(env.D, &a) != nil || a.Delta == nil || *a.Delta != -0.02 {
				t.Errorf("audio.set: %+v", a)
			}
		}
	}
}

func TestKeys(t *testing.T) {
	for name, want := range map[string]uint16{"a": 0x41, "F12": 0x7B, "f24": 0x87, "enter": 0x0D, "MEDIA_PLAY": 0xB3, "9": 0x39} {
		if got, ok := KeyCode(name); !ok || got != want {
			t.Errorf("KeyCode(%s)=%x,%v want %x", name, got, ok, want)
		}
	}
	if _, ok := KeyCode("NOPE"); ok {
		t.Error("unknown key accepted")
	}
}
