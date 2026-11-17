# Manual test checklist (real hardware)

Automated coverage: Go unit and end-to-end tests, Kotlin tests on the shared vectors, the app's networking code against a live daemon, and the Windows binary and installer under Wine. Nothing below has been run on real Windows or a physical phone yet. Work through it before tagging a release, and record your results in the release notes.

## Install

- [ ] Installer runs on Windows 10 and 11 x64; SmartScreen warning is the only prompt besides UAC
- [ ] Finish page starts the daemon un-elevated (Task Manager → Details → Elevated = No) and opens the pairing page
- [ ] Firewall rule `Korokmote` exists for Private/Domain
- [ ] Reboot: daemon starts at logon (tray icon present)
- [ ] Uninstall removes files, shortcuts, firewall rule, Run entry

## Pairing and connection

- [ ] Phone lists the PC under ON THIS NETWORK (mDNS); with mDNS blocked, the UDP beacon still finds it
- [ ] QR pairing works; the three words match the tray notification
- [ ] Manual pairing (address + short code) works and asks for word confirmation
- [ ] Toggle Wi-Fi off/on on the phone: reconnects by itself
- [ ] Change the PC's IP (renew DHCP): phone finds it again
- [ ] Revoke on the devices page: phone shows "removed on the PC" and offers to pair again
- [ ] Pause input from the tray: input requests fail with "input is paused"

## Control

- [ ] Pointer feels immediate (`korokctl bench` p50 under 10 ms on Wi-Fi)
- [ ] Tap, two-finger tap, three-finger tap, double-tap-drag, two-finger scroll + inertia, pinch zoom, three-finger swipes
- [ ] Live typing incl. accents/emoji, backspace, autocorrect replacement; modifier latch and lock
- [ ] Media keys control Spotify/YouTube; volume ring tracks changes made on the PC; phone volume keys
- [ ] Output device list shows names; per-device volume works
- [ ] Lock, screen off, sleep, hibernate, restart, timed shutdown + abort
- [ ] Wake-on-LAN wakes the PC (BIOS/NIC setting enabled)
- [ ] Apps list from Start Menu launches; windows list focuses/closes; files open
- [ ] Clipboard both directions; follow mode
- [ ] Remotes: VLC suggested when VLC is in front; custom YAML in the remotes folder appears within 2 s
- [ ] Elevated windows (Task Manager) ignore input unless the daemon runs elevated (documented limit)

## App look and behaviour

- [ ] Rain stays smooth; touchpad holds 60 fps at Full on a mid-range phone
- [ ] Reduced/Off intensity and system "Remove animations" disable motion
- [ ] Notification controls work with the app in the background; disconnects after the background timeout
- [ ] Quick Settings tile toggles play/pause
- [ ] TalkBack reads buttons and the status line once
