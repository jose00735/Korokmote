package remotes

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Executor performs primitives on behalf of a session (with its permissions).
type Executor interface {
	Key(name string) error
	Combo(keys []string) error
	Text(s string) error
	Click(button int) error
	Media(action string) error
	Volume(value float64, relative bool) error
	Mute(mode string) error
	Power(action string) error
	Launch(id string) error
	Open(target string) error
	Focus(exe, title string) error
}

// MaxActionTime bounds a whole action.
const MaxActionTime = 15 * time.Second

// Run executes a named action.
func (r *Remote) Run(ctx context.Context, name string, args map[string]string, ex Executor) error {
	steps, ok := r.Actions[name]
	if !ok {
		return proto.Errorf(proto.ErrNotFound, "remote %s has no action %s", r.ID, name)
	}
	ctx, cancel := context.WithTimeout(ctx, MaxActionTime)
	defer cancel()
	for i, st := range steps {
		if err := ctx.Err(); err != nil {
			return proto.Errorf(proto.ErrInternal, "action timed out")
		}
		if err := runStep(ctx, st, args, ex); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, st.Kind, err)
		}
	}
	return nil
}

func runStep(ctx context.Context, st Step, args map[string]string, ex Executor) error {
	switch st.Kind {
	case "key":
		return ex.Key(st.Str)
	case "combo":
		return ex.Combo(st.List)
	case "text":
		return ex.Text(Substitute(st.Str, args))
	case "click":
		return ex.Click(clickButtons[st.Str])
	case "media":
		return ex.Media(st.Str)
	case "volume":
		v, rel, err := parseVolume(Substitute(st.Str, args))
		if err != nil {
			return proto.Errorf(proto.ErrBadRequest, "%v", err)
		}
		return ex.Volume(v, rel)
	case "mute":
		return ex.Mute(st.Str)
	case "power":
		return ex.Power(st.Str)
	case "launch":
		return ex.Launch(Substitute(st.Str, args))
	case "open":
		if strings.HasPrefix(st.Str, "http://") || strings.HasPrefix(st.Str, "https://") {
			esc := make(map[string]string, len(args))
			for k, v := range args {
				esc[k] = url.QueryEscape(v)
			}
			return ex.Open(Substitute(st.Str, esc))
		}
		return ex.Open(Substitute(st.Str, args))
	case "focus":
		return ex.Focus(st.Exe, st.Title)
	case "wait":
		select {
		case <-time.After(st.Wait):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return proto.Errorf(proto.ErrUnsupported, "unknown primitive %s", st.Kind)
}
