// Package logring keeps recent log lines in memory for the admin page.
package logring

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Ring is a slog.Handler that forwards to next and keeps the last N lines.
type Ring struct {
	next  slog.Handler
	attrs []slog.Attr
	buf   *buffer
}

type buffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

// New wraps next.
func New(next slog.Handler, max int) *Ring {
	return &Ring{next: next, buf: &buffer{max: max}}
}

func (r *Ring) Enabled(ctx context.Context, l slog.Level) bool { return r.next.Enabled(ctx, l) }

func (r *Ring) Handle(ctx context.Context, rec slog.Record) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %-5s %s", rec.Time.Format(time.TimeOnly), rec.Level.String(), rec.Message)
	for _, a := range r.attrs {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
	}
	rec.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	r.buf.mu.Lock()
	r.buf.lines = append(r.buf.lines, b.String())
	if len(r.buf.lines) > r.buf.max {
		r.buf.lines = r.buf.lines[len(r.buf.lines)-r.buf.max:]
	}
	r.buf.mu.Unlock()
	return r.next.Handle(ctx, rec)
}

func (r *Ring) WithAttrs(as []slog.Attr) slog.Handler {
	return &Ring{next: r.next.WithAttrs(as), attrs: append(append([]slog.Attr{}, r.attrs...), as...), buf: r.buf}
}

func (r *Ring) WithGroup(name string) slog.Handler {
	return &Ring{next: r.next.WithGroup(name), attrs: r.attrs, buf: r.buf}
}

// Lines returns the most recent lines, oldest first.
func (r *Ring) Lines() []string {
	r.buf.mu.Lock()
	defer r.buf.mu.Unlock()
	return append([]string(nil), r.buf.lines...)
}
