# Remotes

A remote is a YAML file that describes a screen layout and what each control does. The daemon loads remotes from two places:

1. Built-ins in `daemon/internal/remotes/builtin/`, embedded in the binary.
2. `%APPDATA%\Korokmote\remotes\`, which is watched for changes. A user file with the same `id` replaces the built-in.

The app asks for the list, fetches a layout, renders it with its own widgets and sends `remotes.act` when the user touches a control. The app never sees how an action is implemented.

## Example

```yaml
id: vlc
name: VLC
icon: play            # name from the app's icon set
match: [vlc.exe]      # foreground executables that make the app suggest this remote
version: 1

actions:
  toggle:   { key: SPACE }
  stop:     { key: S }
  back10:   { combo: [ALT, LEFT] }
  fwd10:    { combo: [ALT, RIGHT] }
  vol_up:   { combo: [CTRL, UP] }
  vol_down: { combo: [CTRL, DOWN] }
  full:     { key: F }
  subs:     { key: V }
  open:
    - { launch: "vlc" }
    - { wait: 800ms }
    - { focus: { exe: vlc.exe } }

layout:
  - row:
      - button: { label: "<<10", action: back10 }
      - button: { icon: play_pause, action: toggle, size: 2, style: primary }
      - button: { label: "10>>", action: fwd10 }
  - row:
      - button: { icon: stop, action: stop }
      - button: { label: "FULL", action: full }
      - button: { label: "SUBS", action: subs }
  - slider: { label: "VOLUME", up: vol_up, down: vol_down, steps: 20 }
  - touchpad: { height: 3 }
```

## Action primitives

An action is one primitive or a list of them, run in order. Actions targeting a specific app can set `focus` first.

| Primitive | Example | Effect |
|---|---|---|
| `key` | `{ key: SPACE }` | Tap a key (names in `PROTOCOL.md`, section 8) |
| `combo` | `{ combo: [CTRL, SHIFT, T] }` | Key combination |
| `text` | `{ text: "hello" }` | Type Unicode text |
| `click` | `{ click: left }` | Mouse click |
| `media` | `{ media: next }` | Media key |
| `volume` | `{ volume: "+0.05" }` or `{ volume: 0.3 }` | System volume |
| `mute` | `{ mute: toggle }` | System mute |
| `power` | `{ power: sleep }` | Power action (needs the `power` permission) |
| `launch` | `{ launch: "vlc" }` | Start an app by launcher id or a path from the allow-list |
| `open` | `{ open: "https://youtube.com" }` | Open a URL or file with the default handler |
| `focus` | `{ focus: { exe: vlc.exe } }` | Bring a window to the front |
| `wait` | `{ wait: 300ms }` | Pause (max 5 s per step, 15 s per action) |
| `arg` | `{ text: "${args.query}" }` | Substitute a value sent by the app (from an `input` widget) |

There is no shell or script primitive in v1. Anything that would run an arbitrary command must be added as an allow-listed entry under `launch` in `config.yaml` by the PC's owner.

## Layout widgets

| Widget | Fields | Notes |
|---|---|---|
| `row` | list of widgets | Children share the width; `size` sets relative weight |
| `grid` | `columns`, `items` | Even grid of buttons |
| `button` | `label` or `icon`, `action`, `hold` (action on long-press), `repeat` (bool), `style` (`default`, `primary`, `danger`), `size` | `repeat` re-fires while held |
| `toggle` | `label`, `on`, `off` | Two-state button |
| `slider` | `label`, `up`, `down`, `steps` or `set` | Relative (`up`/`down`) or absolute (`set` with `${args.value}`) |
| `dpad` | `up`, `down`, `left`, `right`, `center` | Directional pad |
| `touchpad` | `height` | Embedded pointer surface |
| `input` | `label`, `action`, `placeholder` | Text field; sends `args.query` |
| `label` | `text` | Static text, terminal style |
| `spacer` | `size` | Empty space |

The app must ignore widgets and fields it does not know and still render the rest.

## Built-in remotes planned for v1

`presentation` (PowerPoint, Google Slides, PDF viewers), `browser` (tabs, back/forward, zoom, address input), `vlc`, `mpv`, `spotify`, `youtube` (browser shortcuts), `netflix` (browser shortcuts), `window` (snap, switch, close, virtual desktops), `numpad`.

## Validation

`korokctl remotes lint <file>` checks a remote against the schema in `protocol/schema/remote.schema.json`, verifies that every referenced action exists, and reports unknown key names. The daemon logs and skips invalid remotes instead of failing to start.
