# Korokmote protocol, version 1

Status: draft. Changes to this file come before changes to code. Golden test vectors live in `protocol/vectors/` and JSON Schemas in `protocol/schema/`.

## 1. Discovery

### mDNS / DNS-SD

Service type `_korokmote._tcp.local`, port = daemon port (default 47810).

TXT records:

| Key | Example | Meaning |
|---|---|---|
| `id` | `7c1e…` | Daemon ID (UUID, stable) |
| `name` | `DESKTOP-JOSE` | Display name |
| `v` | `1` | Highest protocol version supported |
| `fp` | `a1b2c3d4` | First 8 hex chars of the certificate fingerprint (hint only, never trusted) |
| `pair` | `1` | Present while a pairing window is open |

### UDP fallback

The app broadcasts the 8-byte datagram `KRKM?` + version byte + 2 reserved bytes to port 47811. The daemon answers unicast with a JSON object containing the same fields as the TXT records plus `port`. Used when mDNS yields nothing within 2 s.

Discovery data is unauthenticated. It is only ever used to find an address; identity is proven by the pinned certificate.

## 2. Pairing

1. The user chooses **Pair a device** on the PC. The daemon opens a pairing window (default 120 s) and shows a QR code containing:

   ```
   korokmote://pair?v=1&id=<daemon-id>&name=<name>&h=<ip1,ip2>&p=<port>&fp=<sha256-hex>&c=<one-time-code>&mac=<aa:bb:..>
   ```

   - `fp`: SHA-256 of the daemon certificate (DER), 64 hex chars.
   - `c`: 128-bit random one-time code, base64url. Valid for this window only, single use.
   - `mac`: MAC of the active interface, for Wake-on-LAN.

2. The app connects by TLS to one of `h`, verifies the server certificate hash equals `fp`, and sends:

   ```
   POST /v1/pair
   { "code": "<c>", "device": { "id": "<uuid>", "name": "Pixel 8", "platform": "android", "app": "1.0.0" } }
   ```

3. The daemon replies `200 { "token": "<256-bit base64url>", "daemon": { "id": "...", "name": "..." } }`, stores only `SHA-256(token)`, closes the pairing window, and shows a tray notification naming the new device.

4. Both sides show the same **verification phrase** (three words derived from `SHA-256(fp || device.id)`) as a visual confirmation.

**Manual pairing** (camera unavailable): the daemon also shows the address and a short code. The user types both; the app displays the full verification phrase and the user must confirm it matches the PC before the token is stored. In this mode the fingerprint is learned on first use, so the confirmation step is mandatory.

Failures: wrong or expired code -> `403`; after 5 failures the window closes.

## 3. Transport

- `wss://<host>:<port>/v1/ws`, TLS 1.3, server certificate must match the pinned fingerprint.
- Text frames: one JSON object each (UTF-8). Binary frames: high-rate input (section 6).
- Max frame size 64 KiB. `TCP_NODELAY` on. No WebSocket compression.

## 4. Envelope

```json
{ "t": "key", "id": 42, "d": { } }
```

| Field | Meaning |
|---|---|
| `t` | Message type |
| `id` | Optional. If present, the peer answers with `{"t":"ok","re":42,"d":{…}}` or `{"t":"err","re":42,"d":{"code":"…","msg":"…"}}` |
| `d` | Payload, type-specific |

Unknown types and unknown fields are ignored (forward compatibility). Error codes: `unauthorized`, `forbidden`, `bad_request`, `unsupported`, `not_found`, `locked`, `rate_limited`, `internal`.

## 5. Session

### hello (client -> server, must be first, within 3 s)

```json
{ "t": "hello", "id": 1, "d": { "v": 1, "device": "<device-id>", "token": "<token>", "app": "1.0.0" } }
```

Reply:

```json
{ "t": "ok", "re": 1, "d": {
    "v": 1,
    "daemon": { "id": "…", "name": "DESKTOP-JOSE", "os": "windows", "version": "1.0.0" },
    "caps": ["input", "text", "media", "audio", "power", "windows", "files", "launcher", "clipboard", "remotes"],
    "perms": ["input", "media", "audio", "power", "files"],
    "screen": { "w": 2560, "h": 1440 },
    "locked": false
} }
```

`caps` is what the daemon can do; `perms` is what this device is allowed to do. Version negotiation: the server answers with `min(client v, server v)`; if it cannot support that it replies `err unsupported` and closes.

### ping / pong

Client sends `{"t":"ping","d":{"ts":<client ms>}}` every 5 s; server echoes `{"t":"pong","d":{"ts":<same>}}`. The app shows round-trip time in the status line.

### Server events (no reply expected)

| `t` | Payload | When |
|---|---|---|
| `ev.lock` | `{ "locked": true }` | Session locked / unlocked |
| `ev.audio` | `{ "device": "…", "level": 0.42, "muted": false }` | Volume changed on the PC |
| `ev.fg` | `{ "title": "…", "exe": "vlc.exe", "remote": "vlc" }` | Foreground app changed; `remote` is a suggested remote id |
| `ev.remotes` | `{}` | Remote definitions reloaded |
| `ev.clip` | `{ "text": "…" }` | PC clipboard changed (only if the device subscribed) |
| `bye` | `{ "reason": "revoked" }` | Server is closing the session |

## 6. Binary frames

First byte is the opcode. Integers are little-endian, signed.

| Opcode | Name | Layout (bytes) | Notes |
|---|---|---|---|
| `0x01` | pointer move (relative) | `op(1) dx(i16) dy(i16) seq(u8)` | Pixels. 6 bytes |
| `0x02` | scroll | `op(1) dx(i16) dy(i16) seq(u8)` | Wheel units; 120 = one notch |
| `0x03` | pointer move (absolute) | `op(1) x(u16) y(u16) seq(u8)` | 0..65535 across the virtual desktop |
| `0x04` | button | `op(1) button(u8) down(u8)` | 1 left, 2 right, 3 middle, 4 back, 5 forward |

Frames with an unknown opcode are ignored. Binary frames before `hello` close the connection.

## 7. Control messages (client -> server)

### Input

| `t` | Payload | Notes |
|---|---|---|
| `key` | `{ "k": "A", "a": "tap" }` | `k` is a key name (section 8); `a` is `down`, `up` or `tap` |
| `combo` | `{ "keys": ["CTRL", "SHIFT", "ESC"] }` | Pressed in order, released in reverse |
| `text` | `{ "s": "héllo 👋" }` | Unicode, layout-independent |
| `click` | `{ "b": 1, "n": 2 }` | `n` clicks of button `b` |

### Media and audio

| `t` | Payload |
|---|---|
| `media` | `{ "a": "playpause" }` with `a` in `playpause`, `next`, `prev`, `stop` |
| `audio.list` | `{}` -> `{ "devices": [ { "id", "name", "default", "level", "muted" } ] }` |
| `audio.set` | `{ "device": "default", "level": 0.5 }` or `{ "device": "default", "delta": -0.02 }` |
| `audio.mute` | `{ "device": "default", "muted": true }` (omit `muted` to toggle) |

### Power

| `t` | Payload |
|---|---|
| `power` | `{ "a": "lock" }` with `a` in `lock`, `sleep`, `hibernate`, `restart`, `shutdown`, `monitor_off`, `abort`; optional `"delay": 30` seconds for restart/shutdown |

### Windows, launcher, files, clipboard

| `t` | Payload -> reply |
|---|---|
| `win.list` | `{}` -> `{ "windows": [ { "h", "title", "exe" } ] }` |
| `win.focus` / `win.close` | `{ "h": 123456 }` |
| `apps.list` | `{}` -> `{ "apps": [ { "id", "name" } ] }` |
| `apps.launch` | `{ "id": "…" }` |
| `apps.icon` | `{ "id": "…", "size": 64 }` -> `{ "png": "<base64>" }` |
| `fs.roots` | `{}` -> `{ "entries": [ … ] }` |
| `fs.list` | `{ "path": "C:\\Users\\jose\\Videos" }` -> `{ "entries": [ { "name", "dir", "size", "mtime" } ] }` |
| `fs.open` | `{ "path": "…" }` |
| `clip.get` | `{}` -> `{ "text": "…" }` |
| `clip.set` | `{ "text": "…" }` |
| `clip.sub` | `{ "on": true }` |

### Remotes

| `t` | Payload -> reply |
|---|---|
| `remotes.list` | `{}` -> `{ "remotes": [ { "id", "name", "icon", "match": ["vlc.exe"] } ] }` |
| `remotes.get` | `{ "id": "vlc" }` -> the layout document (`REMOTES.md`) |
| `remotes.act` | `{ "remote": "vlc", "action": "vol_up", "args": { } }` |

## 8. Key names

Letters `A`–`Z`, digits `0`–`9`, `F1`–`F24`, and:
`ENTER ESC TAB SPACE BACKSPACE DELETE INSERT HOME END PGUP PGDN UP DOWN LEFT RIGHT CTRL SHIFT ALT WIN MENU CAPS PRTSC PAUSE NUMLOCK SCROLLLOCK VOL_UP VOL_DOWN VOL_MUTE MEDIA_PLAY MEDIA_NEXT MEDIA_PREV MEDIA_STOP BROWSER_BACK BROWSER_FWD BROWSER_REFRESH`, numpad `NUM0`–`NUM9 NUM_ADD NUM_SUB NUM_MUL NUM_DIV NUM_DOT`, and punctuation by name (`MINUS EQUAL LBRACKET RBRACKET SEMICOLON QUOTE COMMA PERIOD SLASH BACKSLASH GRAVE`).

Key names refer to physical keys on a US layout. For characters, use `text`.

## 9. Wake-on-LAN

Not part of the session. The app sends the standard magic packet (6 × `0xFF` then the MAC 16 times) by UDP broadcast to ports 9 and 7 on the stored subnet.

## 10. Versioning rules

- Additive changes (new types, new optional fields, new opcodes) do not bump the version.
- Changing the meaning or layout of an existing message bumps `v` and the path (`/v2/ws`). The daemon keeps serving the previous version for at least one minor release.
