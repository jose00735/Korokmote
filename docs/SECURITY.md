# Security

A daemon that injects keystrokes is, by nature, remote code execution for anyone who can talk to it. The design treats every network peer as hostile until it proves it holds a paired device's token over a connection to the pinned certificate.

## Threat model

| Attacker | Example | Defence |
|---|---|---|
| Another device on the same Wi-Fi | Guest, compromised IoT device, shared residence network | Nothing works without a device token; pairing needs physical sight of the PC screen; rate limits and lockout on auth failures |
| Network eavesdropper / man in the middle | ARP spoofing on the LAN | TLS 1.3 with the certificate pinned from the QR code; discovery data is never trusted for identity |
| Rogue daemon | Fake `_korokmote._tcp` advertisement | The app only sends the token after the pin check passes |
| Lost or stolen phone | | Token encrypted with a Keystore key; per-device revoke on the PC; optional biometric lock for power and file actions |
| Malicious web page on the PC | Browser script calling the local admin page | Admin page bound to loopback, random port, one-time URL token, `Origin` check, no CORS |
| Malicious remote definition | YAML file from the internet | No shell primitive; fixed action list; lint tool; launches restricted to an allow-list |
| Internet exposure | Port forwarded by mistake | Non-private source addresses refused unless `allow_public: true` |

Out of scope: an attacker who already runs code as the same Windows user, or who has the unlocked, paired phone in hand.

## Rules the implementation must keep

1. **No unauthenticated action.** Before a valid `hello`, the daemon accepts nothing except `hello` and, during an open pairing window, `POST /v1/pair`.
2. **Pairing requires local intent.** A pairing window opens only from the tray or admin page, lasts 120 s, accepts one device, and closes after 5 bad codes.
3. **Tokens are 256-bit random, stored hashed** (SHA-256) on the PC and encrypted on the phone. Comparison is constant-time.
4. **The private key never leaves the PC** and is readable only by the owning user.
5. **Per-device permissions**: `input`, `media`, `audio`, `power`, `files`, `launcher`, `clipboard`. New devices get all except `clipboard` subscription; the owner can reduce them per device.
6. **File access is bounded** to configured roots; paths are cleaned and checked after symlink resolution. v1 can list and open files, not read, upload or delete them.
7. **Stuck-input safety**: all held keys and buttons are released when a session ends.
8. **Pause switch**: one tray click rejects all input until resumed.
9. **Visibility**: a tray notification when a device pairs, and when a device connects for the first time in a day. Auth failures are logged with the peer address.
10. **Logs never contain** tokens, typed text, clipboard contents or key names.
11. **No telemetry, no update pings, no network traffic beyond the LAN** unless the user points the app at a non-LAN address.

## Reporting

Report vulnerabilities through GitHub's private vulnerability reporting on this repository rather than a public issue.
