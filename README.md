# Korokmote

Open-source remote control for your Windows PC from your Android phone, over your own network. No account, no cloud, no subscription.

- **Daemon** (`daemon/`): one Go binary for Windows that lives in the notification area (the green K) and handles input, media, volume, power, apps, windows, files and the clipboard.
- **Controller** (`android/`): a Kotlin and Jetpack Compose app with a Matrix-style interface: digital rain, decode text, glitch transitions and touch trails.
- **Protocol** (`protocol/`): documented, versioned, and tested on both sides against the same golden vectors.

## Install

1. **PC:** run `korokmote-setup-<version>-x64.exe`. It installs to Program Files and adds a firewall rule for private networks. When it finishes, Korokmote starts and opens the pairing page. A portable zip is also available (x64 and ARM64) if you'd rather not install.
2. **Phone:** install `korokmote-<version>.apk`. You'll need to allow installs from your browser or file manager.
3. **Pair:** in the app, tap **[ + PAIR ]** and scan the QR code on the pairing page. You can also tap **MANUAL** and type the address and short code. Both screens show three words; check that they match.

After that, the app reconnects on its own whenever the PC is reachable. To pair another phone later, right-click the tray icon and choose **Pair a device…**.

**If the phone can't find the PC,** open Windows Settings → Network and set your Wi-Fi or Ethernet to **Private**. The firewall rule only covers private networks unless you ticked the public-network option during setup.

## What it does

| Area | Features |
|---|---|
| Touchpad | move with acceleration, tap / two-finger tap / three-finger tap for left / right / middle click, two-finger scroll with inertia, pinch to zoom, double-tap and hold to drag, three-finger swipes for Alt+Tab, Task View and the desktop |
| Keyboard | live typing from the phone keyboard (autocorrect-safe), compose-and-send, modifier latches and locks, navigation and function keys, 16 shortcuts |
| Media | play/pause/next/previous/stop, a volume ring, per-device volume and mute, phone volume keys control the PC, media controls in the notification, a Quick Settings tile |
| Power | lock, screen off, sleep, hibernate, restart, shut down (hold to confirm), timed shutdown and abort, Wake-on-LAN |
| Apps, windows, files | launch Start Menu apps, focus or close windows, browse allowed folders and open files on the PC |
| Clipboard | send phone → PC, get PC → phone, or follow the PC clipboard live |
| Remotes | Presentation, Browser, VLC, mpv, Spotify, YouTube, Netflix, Windows, Numpad. Add your own by dropping a YAML file in the remotes folder, with no rebuild (see [docs/REMOTES.md](docs/REMOTES.md)). The app suggests the right remote for whichever app is in front |
| Look | digital rain, decode text, glitch transitions, CRT scanlines, tap rings and trails, haptics. Animation can be set to Full, Reduced or Off, with accents in green, amber, cyan or red |

## Security in one paragraph

Pairing needs someone at the PC to open a two-minute window and show a single-use code. The phone pins the PC's certificate from the QR code. After that, all traffic is TLS 1.3, and the phone accepts only that certificate. The phone holds a 256-bit device token, encrypted with the Android Keystore; the PC stores only its hash. You can set each paired device's permissions, or revoke it, on the PC's devices page, and one tray click pauses all input. Nothing leaves your network. Full details are in [docs/SECURITY.md](docs/SECURITY.md).

## Build from source

```bash
# Windows daemon, installer and portable zips → dist/   (needs Go 1.24+, NSIS 3, zip)
./build.sh 1.0.0

# Android APK                                             (needs JDK 17+ and the Android SDK)
cd android && ./gradlew assembleRelease     # signed if keystore.properties or KOROKMOTE_* env vars exist

# Tests
cd daemon && go test ./...                  # includes an end-to-end test of pairing and control
cd android && ./gradlew test                # shares protocol/vectors with the Go tests
```

The daemon also runs on Linux with `--fake` (the platform calls are logged rather than executed), which is handy for working on the app. `korokctl` is a command-line client for scripting and testing: `korokctl pair`, `korokctl key ENTER`, `korokctl bench`.

Pushing a `v*` tag runs [.github/workflows/release.yml](.github/workflows/release.yml), which builds everything and publishes a GitHub release.

## Documents

| Document | What it covers |
|---|---|
| [docs/PLAN.md](docs/PLAN.md) | Milestones and their status |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System overview, repo layout, key decisions |
| [docs/DAEMON.md](docs/DAEMON.md) | The Go daemon: Win32 mapping, files on disk, packaging |
| [docs/ANDROID.md](docs/ANDROID.md) | The app: structure, connection, gestures, screens |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | Discovery, pairing, wire format, message catalogue |
| [docs/REMOTES.md](docs/REMOTES.md) | How to write your own remotes |
| [docs/DESIGN.md](docs/DESIGN.md) | The Matrix visual language and every animation |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat model and rules |

## Licence

MIT. See [LICENSE](LICENSE). Bundles JetBrains Mono (SIL Open Font License 1.1).
