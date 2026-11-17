//go:build windows

package app

import (
	"github.com/jose00735/korokmote/daemon/internal/platform"
	winplat "github.com/jose00735/korokmote/daemon/internal/platform/windows"
)

func newPlatform(bool) *platform.Platform {
	winplat.SetDPIAware()
	return winplat.New()
}

func openURL(u string) error                 { return winplat.ShellOpen(u) }
func autostartEnabled() bool                 { return winplat.Autostart() }
func setAutostart(on bool, exe string) error { return winplat.SetAutostart(on, exe) }

// SingleInstance reports whether this is the only daemon in the user session.
func SingleInstance() bool { return winplat.SingleInstance(`Local\Korokmote`) }

// Fatal shows an error dialog.
func Fatal(msg string) { winplat.MessageBox("Korokmote", msg, true) }

func installerAutostart() string { return winplat.InstallerAutostart() }
