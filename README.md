# Korokmote

Open-source remote control for your PC, from your phone, over your own network. No account, no cloud, no subscription.

- **Daemon** (`daemon/`): a single Go binary for Windows that lives in the system tray and injects input, controls media, volume, power, files and apps.
- **Controller** (`android/`): a Kotlin + Jetpack Compose Android app with a Matrix-inspired interface.
- **Protocol** (`protocol/`): a small, documented, versioned protocol over TLS-pinned WebSocket, so other clients (iOS, web, CLI) can be written later.

> Status: design phase. Nothing is implemented yet. The documents below are the source of truth for the build.

## Documents

| Document | What it covers |
|---|---|
| [docs/PLAN.md](docs/PLAN.md) | Scope, milestones, order of work, definition of done |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System overview, repo layout, key decisions and their reasons |
| [docs/DAEMON.md](docs/DAEMON.md) | Go daemon for Windows: packages, Win32 APIs, config, packaging |
| [docs/ANDROID.md](docs/ANDROID.md) | Android app: modules, screens, state, networking |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | Discovery, pairing, wire format, message catalogue |
| [docs/REMOTES.md](docs/REMOTES.md) | Declarative remote definitions (custom remotes without recompiling) |
| [docs/DESIGN.md](docs/DESIGN.md) | Matrix visual language: colours, type, components, animations |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat model, pairing security, what the daemon refuses to do |
| [CLAUDE.md](CLAUDE.md) | Working instructions for AI-assisted development in this repo |

## Feature set (v1.0 target)

- Touchpad mouse (move, click, right-click, scroll, drag, pinch-zoom) and full keyboard with text entry
- Media keys and per-device volume, with controls in the Android notification and on the hardware volume buttons
- Power: lock, sleep, hibernate, restart, shut down, plus Wake-on-LAN from the app
- App launcher, window switcher, file browser (open files on the PC)
- Presentation remote, browser remote, and custom remotes defined in YAML
- Clipboard send (phone to PC and back)
- Automatic discovery on the LAN, QR-code pairing, multiple PCs

## Licence

MIT. See [LICENSE](LICENSE).
