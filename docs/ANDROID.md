# Android controller

Kotlin, Jetpack Compose, single activity. `minSdk 29` (Android 10, the first release with TLS 1.3 on by default, which the daemon requires), `targetSdk 35`. Package: `dev.korokmote.app`.

All dependencies must be open source so the app can ship on F-Droid: no Google Play Services, no ML Kit, no Firebase.

## Stack

| Concern | Choice |
|---|---|
| UI | Jetpack Compose + Material 3 (heavily themed, see `DESIGN.md`) |
| Navigation | Small in-house back stack (`ui/Nav.kt`); route changes trigger the glitch transition |
| State | ViewModel + `StateFlow`, unidirectional data flow |
| DI | Manual graph in `KorokApp` (no Hilt) |
| Networking | OkHttp WebSocket with a custom `X509TrustManager` that pins the daemon certificate |
| Serialisation | kotlinx.serialization (JSON), hand-written binary codec for pointer frames |
| Discovery | `NsdManager` (`_korokmote._tcp`), UDP broadcast fallback |
| Storage | SharedPreferences (hosts as JSON, settings), Android Keystore AES-256-GCM key that encrypts device tokens |
| QR scanning | CameraX + ZXing core |
| Background | Foreground service of type `connectedDevice` with a media-style notification |
| Tests | JUnit, Turbine, Compose UI tests, screenshot tests for `core:design` |

## Structure

For v1, everything lives in one `:app` module, split into packages that follow the module plan. That keeps the build fast and simple, and the packages can become modules later without any code changes.

```
protocol/   Message models, binary frames, verification phrase, key names (pure Kotlin, JVM-tested)
net/        KorokConnection, ConnectionManager (reconnect, events), Pinning, Pairing, Discovery, WakeOnLan
data/       HostStore (Keystore-encrypted tokens), SettingsStore
service/    ConnectionService (foreground, media-style notification), PlayPauseTile
design/     Theme tokens, DigitalRain, DecodeText, glitch/sweep/CRT effects, components, haptics
ui/         AppRoot, Nav, screens, TouchpadEngine, remote renderer, QR scanner
```

`design/` knows nothing about features; `ui/` screens depend on `net/`, `data/` and `design/`.

## Connection layer

```kotlin
interface KorokConnection {
    val state: StateFlow<ConnectionState>      // Idle, Connecting, Online(caps), Reconnecting, Locked, Failed(reason)
    val events: SharedFlow<ServerEvent>        // volume changed, foreground app, remotes changed, clipboard
    fun pointer(dx: Int, dy: Int)              // binary frame, non-suspending, coalesced
    fun scroll(dx: Int, dy: Int)
    suspend fun send(msg: ClientMessage)       // JSON control message
    suspend fun <R> request(msg: ClientRequest<R>): R   // correlated by id, with timeout
}
```

- `ConnectionService` owns the single `KorokConnection` for the active host; screens bind to it through a repository. It survives configuration changes and backgrounding.
- **Pointer coalescing**: gesture callbacks add deltas to an accumulator; a sender flushes on every display frame (`Choreographer`) so at most one pointer frame is sent per frame, regardless of touch sample rate.
- **Reconnect**: backoff 250 ms -> 8 s; network-change callbacks (`ConnectivityManager`) trigger an immediate retry; the address is refreshed from discovery by daemon ID.
- **Pinning**: the trust manager accepts exactly one certificate, compared by SHA-256 of the DER encoding. Hostname verification is disabled because the identity *is* the pinned certificate.
- Service stops itself after 10 minutes with the app in the background and no notification interaction (configurable).

## Screens

| Screen | Purpose | Notes |
|---|---|---|
| Boot | First-frame splash | Rain + decode of the app name, max 900 ms, skipped under Reduced motion |
| Hosts | Paired PCs and discovered daemons | Status dot (online / asleep / unknown), long-press for Wake, Rename, Forget |
| Pair | QR scanner with manual entry fallback | Shows the verification phrase after scan |
| Touchpad | Default screen once connected | Full-bleed surface, slim key row (Esc, Tab, Win, arrows), keyboard toggle |
| Keyboard | Text entry and special keys | Live typing or "compose then send" mode; modifier latches |
| Media | Play/pause, next, previous, seek keys, volume ring | Output device picker |
| Power | Lock, sleep, hibernate, restart, shut down, monitor off | Hold-to-confirm with a filling ring |
| Remotes | Grid of available remotes | Auto-suggests the remote matching the PC's foreground app |
| Remote | Rendered layout | Widgets defined in `REMOTES.md` |
| Files | Browse and open files; app launcher tab | Breadcrumb path in terminal style |
| Settings | Preferences | See below |

Navigation: a bottom "command bar" with Touchpad, Keyboard, Media, Remotes, More. The connection state lives in a thin status line at the top of every screen (`> DESKTOP-JOSE :: online :: 3 ms`).

## Touchpad gesture engine

Implemented with `pointerInput` and raw pointer events, not the high-level gesture detectors, so several gestures can be recognised together.

| Gesture | Action |
|---|---|
| One-finger move | Pointer move (with acceleration curve) |
| Tap | Left click |
| Two-finger tap | Right click |
| Three-finger tap | Middle click |
| Two-finger drag | Scroll (vertical and horizontal, with inertia) |
| Double-tap and hold, then move | Drag (left button held) |
| Pinch | Ctrl + wheel (zoom) |
| Three-finger swipe left/right | Alt+Tab / Alt+Shift+Tab |
| Three-finger swipe up | Win+Tab |

Acceleration: `out = delta * sensitivity * (1 + accel * speed)`, with `speed` in dp/ms, clamped. Sub-pixel remainders are carried to the next frame so slow movement stays precise. Sensitivity and acceleration are settings.

Hardware volume keys are intercepted in the activity while connected and mapped to PC volume (toggle in settings).

## Notification and system integration

- Media-style foreground notification: previous, play/pause, next, volume down/up, disconnect.
- Quick Settings tile: connect to the last host / toggle play-pause.
- App shortcuts (long-press launcher icon): one per paired host.

## Settings

Pointer speed, acceleration, scroll direction and speed, tap-to-click, haptics strength, animation intensity (Full / Reduced / Off), rain density, glyph set (katakana / binary / hex), accent colour (green / amber / cyan / red), keep screen on, volume keys control PC, background timeout.

## Permissions

`INTERNET`, `ACCESS_NETWORK_STATE`, `ACCESS_WIFI_STATE`, `CHANGE_WIFI_MULTICAST_STATE` (mDNS), `CAMERA` (QR, requested only on the pair screen), `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_CONNECTED_DEVICE`, `POST_NOTIFICATIONS`, `VIBRATE`, and `NEARBY_WIFI_DEVICES` on Android 13+ where discovery requires it.

## Build and release

- `./gradlew :app:assembleDebug`, `./gradlew test`, `./gradlew :core:design:verifyScreenshots`.
- Release APKs are signed in CI from a keystore held in GitHub secrets and attached to GitHub Releases. Reproducible build settings are kept so F-Droid can build from source.
