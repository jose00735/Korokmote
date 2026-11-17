// Package remotes loads declarative remote definitions (YAML) and executes their actions.
// See docs/REMOTES.md.
package remotes

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// Step is one action primitive.
type Step struct {
	Kind  string   // key, combo, text, click, media, volume, mute, power, launch, open, focus, wait
	Str   string   // single-string argument
	List  []string // combo keys
	Exe   string   // focus target
	Title string   // focus target
	Wait  time.Duration
}

// Remote is a parsed remote definition.
type Remote struct {
	ID      string
	Name    string
	Icon    string
	Match   []string
	Version int
	Actions map[string][]Step
	Layout  any // passed to clients as-is
	Source  string
}

type rawRemote struct {
	ID      string         `yaml:"id"`
	Name    string         `yaml:"name"`
	Icon    string         `yaml:"icon"`
	Match   []string       `yaml:"match"`
	Version int            `yaml:"version"`
	Actions map[string]any `yaml:"actions"`
	Layout  any            `yaml:"layout"`
}

// Parse parses and validates one remote document.
func Parse(data []byte, source string) (*Remote, error) {
	var raw rawRemote
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("%s: missing id", source)
	}
	if raw.Name == "" {
		raw.Name = raw.ID
	}
	r := &Remote{
		ID: raw.ID, Name: raw.Name, Icon: raw.Icon, Version: raw.Version,
		Actions: map[string][]Step{}, Layout: normalize(raw.Layout), Source: source,
	}
	for _, m := range raw.Match {
		r.Match = append(r.Match, strings.ToLower(m))
	}
	for name, v := range raw.Actions {
		steps, err := parseAction(v)
		if err != nil {
			return nil, fmt.Errorf("%s: action %q: %w", source, name, err)
		}
		r.Actions[name] = steps
	}
	if err := checkLayoutRefs(r.Layout, r.Actions); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return r, nil
}

// normalize converts YAML maps into JSON-encodable maps.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = normalize(vv)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[fmt.Sprint(k)] = normalize(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = normalize(vv)
		}
		return out
	default:
		return v
	}
}

var actionFields = map[string]bool{"action": true, "hold": true, "up": true, "down": true, "left": true, "right": true, "center": true, "on": true, "off": true, "set": true}

func checkLayoutRefs(v any, actions map[string][]Step) error {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if s, ok := vv.(string); ok && actionFields[k] {
				if _, exists := actions[s]; !exists {
					return fmt.Errorf("layout references unknown action %q", s)
				}
				continue
			}
			if err := checkLayoutRefs(vv, actions); err != nil {
				return err
			}
		}
	case []any:
		for _, vv := range t {
			if err := checkLayoutRefs(vv, actions); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseAction(v any) ([]Step, error) {
	switch t := normalize(v).(type) {
	case []any:
		var out []Step
		for _, item := range t {
			s, err := parseStep(item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("empty action")
		}
		return out, nil
	case map[string]any:
		s, err := parseStep(t)
		if err != nil {
			return nil, err
		}
		return []Step{s}, nil
	default:
		return nil, fmt.Errorf("action must be a map or a list")
	}
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func parseStep(v any) (Step, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return Step{}, fmt.Errorf("each step must be a map with one key")
	}
	for k, val := range m {
		s := Step{Kind: k}
		switch k {
		case "key":
			s.Str = scalar(val)
			if _, ok := proto.KeyCode(s.Str); !ok {
				return s, fmt.Errorf("unknown key %q", s.Str)
			}
		case "combo":
			list, ok := val.([]any)
			if !ok || len(list) == 0 {
				return s, fmt.Errorf("combo needs a list of keys")
			}
			for _, item := range list {
				name := scalar(item)
				if _, ok := proto.KeyCode(name); !ok {
					return s, fmt.Errorf("unknown key %q", name)
				}
				s.List = append(s.List, name)
			}
		case "text", "launch", "open":
			s.Str = scalar(val)
			if s.Str == "" {
				return s, fmt.Errorf("%s needs a value", k)
			}
		case "click":
			s.Str = strings.ToLower(scalar(val))
			if s.Str == "" {
				s.Str = "left"
			}
			if _, ok := clickButtons[s.Str]; !ok {
				return s, fmt.Errorf("unknown click %q", s.Str)
			}
		case "media":
			s.Str = strings.ToLower(scalar(val))
			if _, ok := proto.MediaKeys[s.Str]; !ok {
				return s, fmt.Errorf("unknown media action %q", s.Str)
			}
		case "volume":
			s.Str = scalar(val)
			if _, _, err := parseVolume(s.Str); err != nil && !strings.Contains(s.Str, "${") {
				return s, err
			}
		case "mute":
			s.Str = strings.ToLower(scalar(val))
			if s.Str == "" {
				s.Str = "toggle"
			}
		case "power":
			s.Str = strings.ToLower(scalar(val))
			if !validPower[s.Str] {
				return s, fmt.Errorf("unknown power action %q", s.Str)
			}
		case "focus":
			fm, ok := val.(map[string]any)
			if !ok {
				return s, fmt.Errorf("focus needs exe or title")
			}
			s.Exe = strings.ToLower(scalar(fm["exe"]))
			s.Title = scalar(fm["title"])
			if s.Exe == "" && s.Title == "" {
				return s, fmt.Errorf("focus needs exe or title")
			}
		case "wait":
			d, err := time.ParseDuration(scalar(val))
			if err != nil {
				return s, fmt.Errorf("bad wait: %w", err)
			}
			s.Wait = min(d, 5*time.Second)
		default:
			return s, fmt.Errorf("unknown primitive %q", k)
		}
		return s, nil
	}
	return Step{}, fmt.Errorf("unreachable")
}

var clickButtons = map[string]int{"left": 1, "right": 2, "middle": 3, "back": 4, "forward": 5}

var validPower = map[string]bool{"lock": true, "sleep": true, "hibernate": true, "restart": true, "shutdown": true, "monitor_off": true, "abort": true}

// parseVolume returns (value, relative, err). "+0.05" / "-0.05" are relative.
func parseVolume(s string) (float64, bool, error) {
	s = strings.TrimSpace(s)
	rel := strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, fmt.Errorf("bad volume %q", s)
	}
	return f, rel, nil
}

// Set holds the loaded remotes and reloads user files on change.
type Set struct {
	mu       sync.RWMutex
	remotes  map[string]*Remote
	userDir  string
	stamp    string
	onReload func()
}

// NewSet loads builtins and the user directory.
func NewSet(userDir string) *Set {
	s := &Set{userDir: userDir}
	s.Reload()
	return s
}

// OnReload registers a callback fired after the user folder changed.
func (s *Set) OnReload(f func()) { s.onReload = f }

func (s *Set) dirStamp() string {
	var b strings.Builder
	entries, _ := os.ReadDir(s.userDir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

// Reload re-reads all definitions.
func (s *Set) Reload() {
	all := map[string]*Remote{}
	_ = fs.WalkDir(builtinFS, "builtin", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := builtinFS.ReadFile(p)
		r, err := Parse(b, "builtin:"+d.Name())
		if err != nil {
			slog.Error("invalid builtin remote", "err", err)
			return nil
		}
		all[r.ID] = r
		return nil
	})
	if s.userDir != "" {
		files, _ := filepath.Glob(filepath.Join(s.userDir, "*.y*ml"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			r, err := Parse(b, f)
			if err != nil {
				slog.Warn("skipping invalid remote", "err", err)
				continue
			}
			all[r.ID] = r
		}
	}
	s.mu.Lock()
	s.remotes = all
	s.stamp = s.dirStamp()
	s.mu.Unlock()
}

// Watch polls the user folder until stop is closed.
func (s *Set) Watch(stop <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.RLock()
			old := s.stamp
			s.mu.RUnlock()
			if s.dirStamp() != old {
				s.Reload()
				slog.Info("remotes reloaded")
				if s.onReload != nil {
					s.onReload()
				}
			}
		}
	}
}

// List returns summaries sorted by name.
func (s *Set) List() []proto.RemoteSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]proto.RemoteSummary, 0, len(s.remotes))
	for _, r := range s.remotes {
		out = append(out, proto.RemoteSummary{ID: r.ID, Name: r.Name, Icon: r.Icon, Match: r.Match})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a remote.
func (s *Set) Get(id string) (*Remote, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.remotes[id]
	return r, ok
}

// MatchExe returns the id of the first remote whose match list includes exe.
func (s *Set) MatchExe(exe string) string {
	exe = strings.ToLower(exe)
	if exe == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.remotes))
	for id := range s.remotes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, m := range s.remotes[id].Match {
			if m == exe {
				return id
			}
		}
	}
	return ""
}

// Document returns the layout document served to clients.
func (r *Remote) Document() map[string]any {
	return map[string]any{
		"id": r.ID, "name": r.Name, "icon": r.Icon, "match": r.Match, "version": r.Version,
		"actions": r.actionNames(), "layout": r.Layout,
	}
}

func (r *Remote) actionNames() []string {
	out := make([]string, 0, len(r.Actions))
	for k := range r.Actions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Substitute replaces ${args.name} placeholders.
func Substitute(s string, args map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	for k, v := range args {
		s = strings.ReplaceAll(s, "${args."+k+"}", v)
	}
	return s
}
