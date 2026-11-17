package remotes

import (
	"context"
	"strings"
	"testing"
)

type recorder struct{ calls []string }

func (r *recorder) add(s string) error               { r.calls = append(r.calls, s); return nil }
func (r *recorder) Key(n string) error               { return r.add("key " + n) }
func (r *recorder) Combo(k []string) error           { return r.add("combo " + strings.Join(k, "+")) }
func (r *recorder) Text(s string) error              { return r.add("text " + s) }
func (r *recorder) Click(b int) error                { return r.add("click") }
func (r *recorder) Media(a string) error             { return r.add("media " + a) }
func (r *recorder) Volume(v float64, rel bool) error { return r.add("volume") }
func (r *recorder) Mute(m string) error              { return r.add("mute " + m) }
func (r *recorder) Power(a string) error             { return r.add("power " + a) }
func (r *recorder) Launch(id string) error           { return r.add("launch " + id) }
func (r *recorder) Open(t string) error              { return r.add("open " + t) }
func (r *recorder) Focus(e, t string) error          { return r.add("focus " + e) }

func TestBuiltinsParse(t *testing.T) {
	s := NewSet("")
	list := s.List()
	if len(list) < 9 {
		t.Fatalf("expected the built-in remotes, got %d", len(list))
	}
	if s.MatchExe("VLC.exe") != "vlc" {
		t.Error("vlc.exe should match the vlc remote")
	}
	for _, sum := range list {
		r, _ := s.Get(sum.ID)
		if r.Layout == nil {
			t.Errorf("%s has no layout", r.ID)
		}
	}
}

func TestRunAndSubstitute(t *testing.T) {
	s := NewSet("")
	yt, _ := s.Get("youtube")
	rec := &recorder{}
	if err := yt.Run(context.Background(), "search", map[string]string{"query": "lofi beats & chill"}, rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "open https://www.youtube.com/results?search_query=lofi+beats+%26+chill" {
		t.Errorf("calls: %v", rec.calls)
	}
	vlc, _ := s.Get("vlc")
	rec = &recorder{}
	if err := vlc.Run(context.Background(), "open", nil, rec); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.calls, "|") != "launch vlc|focus vlc.exe" {
		t.Errorf("calls: %v", rec.calls)
	}
}

func TestInvalid(t *testing.T) {
	cases := map[string]string{
		"unknown key":    "id: x\nactions:\n  a: { key: NOPE }\n",
		"bad primitive":  "id: x\nactions:\n  a: { shell: rm }\n",
		"missing action": "id: x\nactions:\n  a: { key: A }\nlayout:\n  - button: { action: b }\n",
		"no id":          "name: x\n",
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc), name); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
