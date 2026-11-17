//go:build !windows

package tray

import "log/slog"

// Tray is a no-op stand-in on non-Windows platforms.
type Tray struct{ quit chan struct{} }

// Run blocks until Quit.
func Run(cfg Config) error {
	t := &Tray{quit: make(chan struct{})}
	if cfg.OnReady != nil {
		go cfg.OnReady(t)
	}
	<-t.quit
	return nil
}

// SetTooltip logs the tooltip.
func (t *Tray) SetTooltip(s string) { slog.Debug("tray tooltip", "text", s) }

// Notify logs the notification.
func (t *Tray) Notify(title, text string) { slog.Info("notification", "title", title, "text", text) }

// Quit ends Run.
func (t *Tray) Quit() {
	select {
	case <-t.quit:
	default:
		close(t.quit)
	}
}
