//go:build !windows

package app

import (
	"log/slog"

	"github.com/jose00735/korokmote/daemon/internal/platform"
)

// Non-Windows builds run with the fake platform (development and tests).
func newPlatform(verbose bool) *platform.Platform {
	p, _ := platform.NewFake(verbose)
	return p
}

func openURL(u string) error {
	slog.Info("open", "target", u)
	return nil
}

func autostartEnabled() bool          { return false }
func setAutostart(bool, string) error { return nil }

// SingleInstance always succeeds outside Windows.
func SingleInstance() bool { return true }

// Fatal logs a fatal error.
func Fatal(msg string) { slog.Error(msg) }

func installerAutostart() string { return "" }
