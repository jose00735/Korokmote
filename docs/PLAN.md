# Plan

## Goal

Replace a paid remote-control app with something we own: a Windows daemon and an Android controller that talk only over the local network, with a documented protocol and a Matrix-inspired look.

## Non-goals (for v1)

- Remote access over the internet (use a VPN such as Tailscale or WireGuard if needed; the daemon will work over it unchanged).
- Screen streaming / remote desktop. A low-rate screen *preview* is a post-v1 idea.
- macOS and Linux daemons. The Go code is structured so they can be added (see `DAEMON.md`, platform layer), but only Windows is built and tested.
- iOS client.
- Controlling the lock screen, UAC prompts, or elevated windows by default (see `SECURITY.md` and `DAEMON.md`, "Windows limits").

## Milestones

Each milestone ends with something usable. Do them in order; do not start animations polish before M2 works end to end.

### M0 — Skeleton (repo, builds, CI)

- Monorepo layout as in `ARCHITECTURE.md`.
- `daemon/`: Go module, `korokmoted` builds for `windows/amd64`, tray icon appears, `--version` works.
- `android/`: Gradle project, empty Compose app with the Matrix theme tokens and one screen.
- GitHub Actions: `go vet` + `go test` + Windows build; Android `assembleDebug` + unit tests.
- Done when: both artefacts build in CI from a clean clone.

### M1 — Connect and move the mouse

- Daemon: TLS certificate generation, WebSocket server, session handshake, `SendInput` mouse move/click.
- App: manual "IP + port" connection (no pairing yet, debug-only flag), touchpad screen.
- Protocol: `hello`, `ping`, binary pointer frames.
- Done when: finger on the phone moves the PC cursor with no visible lag on Wi-Fi.

### M2 — Pairing and discovery

- Daemon: mDNS advertisement, pairing window with QR code (local web page opened from tray), device store, token auth.
- App: discovery list, QR scan, certificate pinning, encrypted host storage, auto-reconnect.
- Done when: a fresh install pairs by scanning one QR code and reconnects by itself after Wi-Fi drops. The debug bypass from M1 is removed.

### M3 — Core remotes

- Keyboard (keys, modifiers, Unicode text), scroll, drag, media keys, volume (Core Audio), power actions, Wake-on-LAN.
- App: bottom navigation between Touchpad, Keyboard, Media, Power; foreground service; media notification; hardware volume keys.
- Done when: the app replaces daily use of the paid app for mouse, typing, media and power.

### M4 — Matrix interface

- Full design system from `DESIGN.md`: digital rain, decode text, glitch transitions, scanlines, touch trails, haptics.
- Settings: animation intensity (Full / Reduced / Off), rain density, accent colour.
- Done when: every screen follows `DESIGN.md` and the touchpad still holds 60 fps on a mid-range phone with animations at Full.

### M5 — Remotes engine

- Declarative remotes (`REMOTES.md`): daemon loads YAML from `remotes/`, serves layouts, executes actions.
- Built-ins rewritten as remote definitions where possible: Presentation, Browser, VLC/MPV, Spotify, YouTube-in-browser.
- App launcher, window switcher, file browser, clipboard.
- Done when: adding a new remote is a YAML file dropped in a folder, with no rebuild of either side.

### M6 — Release

- Signed Windows installer (or portable zip + autostart toggle), `winget` manifest.
- Android: signed APK on GitHub Releases, F-Droid metadata (all dependencies FOSS; see `ANDROID.md`).
- User docs, screenshots, protocol test vectors frozen as `v1`.

## After v1 (ideas, unordered)

- Home-screen widgets and Quick Settings tiles per action
- Macro recording, per-app automatic remote switching (daemon reports foreground app)
- Screen preview thumbnail on the touchpad
- Gyroscope "air mouse"
- Linux and macOS platform layers; Wear OS companion
- Optional helper service to reach elevated windows

## Working order inside a milestone

1. Update the protocol doc first if messages change.
2. Daemon side with a Go test client (`daemon/cmd/korokctl`) that exercises the message.
3. Android side.
4. Manual test on real hardware; note latency.

## Risks

| Risk | Mitigation |
|---|---|
| Input injection blocked by elevated windows (UIPI) | Document it; optional "run elevated" autostart via Task Scheduler |
| mDNS blocked on some routers / guest Wi-Fi | UDP broadcast fallback, manual IP entry, QR contains the address |
| Antivirus flags an unsigned binary that injects input | Sign releases; publish checksums; keep the binary free of packers |
| Animations drain battery or cause touch lag | Rain capped at 30 fps on its own layer, paused off-screen, intensity setting |
| Scope creep in the remotes engine | Actions are a fixed list of primitives in v1; scripting comes later |
